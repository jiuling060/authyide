package v1

import (
	"fmt"
	"log/slog"

	"github.com/GoYoko/web"
	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/errcode"
	"github.com/chaitin/MonkeyCode/backend/middleware"
)

// AppHandler App 处理器
type AppHandler struct {
	usecase domain.AppQuotaService
	logger  *slog.Logger
}

// buildIDReq 绑定构建任务路由上的 :jobID 参数。
type buildIDReq struct {
	JobID uuid.UUID `param:"jobID" validate:"required"`
}

// enqueueBuildReq 同时绑定 :id 路径参数与打包请求体。
type enqueueBuildReq struct {
	ID          uuid.UUID `param:"id" validate:"required"`
	Platform    string    `json:"platform"`
	Version     string    `json:"version" validate:"required"`
	VersionCode int       `json:"version_code"`
}

// NewAppHandler 创建 App 处理器
func NewAppHandler(i *do.Injector) (*AppHandler, error) {
	w := do.MustInvoke[*web.Web](i)
	logger := do.MustInvoke[*slog.Logger](i)
	usecase := do.MustInvoke[domain.AppQuotaService](i)
	auth := do.MustInvoke[*middleware.AuthMiddleware](i)
	targetActive := do.MustInvoke[*middleware.TargetActiveMiddleware](i)

	h := &AppHandler{
		logger:  logger.With("component", "handler.apps"),
		usecase: usecase,
	}

	v1 := w.Group("/api/v1/users/apps")
	v1.Use(auth.Auth(), targetActive.TargetActive())
	v1.GET("", web.BindHandler(h.List))
	v1.POST("", web.BindHandler(h.Create))
	v1.GET("/ad-networks", web.BindHandler(h.AdNetworks))
	v1.GET("/builds/:jobID", web.BindHandler(h.GetBuild))
	v1.GET("/builds/:jobID/download", web.BindHandler(h.DownloadBuild))
	v1.POST("/builds/:jobID/publish", web.BindHandler(h.PublishBuild))
	v1.DELETE("/builds/:jobID", web.BindHandler(h.DeleteBuild))
	v1.GET("/:id", web.BindHandler(h.Get))
	v1.POST("/:id/builds", web.BindHandler(h.EnqueueBuild))
	v1.GET("/:id/builds", web.BindHandler(h.ListBuilds))
	v1.PUT("/:id", web.BindHandler(h.Update))
	v1.DELETE("/:id", web.BindHandler(h.Delete))

	return h, nil
}

// List 获取当前用户的 App 列表
//
//	@Summary		获取当前用户的 App 列表
//	@Tags			【用户】App
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			page	query		domain.ListAppsReq	true	"分页参数"
//	@Success		200		{object}	web.Resp{data=domain.ListAppsResp}	"成功"
//	@Failure		401		{object}	web.Resp							"未授权"
//	@Failure		500		{object}	web.Resp							"服务器内部错误"
//	@Router			/api/v1/users/apps [get]
func (h *AppHandler) List(c *web.Context, req domain.ListAppsReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.usecase.List(c.Request().Context(), user.ID, &req)
	if err != nil {
		h.logger.ErrorContext(c.Request().Context(), "failed to list apps", "error", err, "user_id", user.ID)
		return err
	}
	return c.Success(resp)
}

// Get 获取单个 App
//
//	@Summary		获取单个 App
//	@Tags			【用户】App
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			id	path		uuid.UUID	true	"App ID"
//	@Success		200		{object}	web.Resp{data=domain.App}	"成功"
//	@Failure		401		{object}	web.Resp					"未授权"
//	@Failure		404		{object}	web.Resp					"App 不存在"
//	@Router			/api/v1/users/apps/{id} [get]
func (h *AppHandler) Get(c *web.Context, req domain.DeleteAppReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.usecase.Get(c.Request().Context(), user.ID, req.ID)
	if err != nil {
		return err
	}
	return c.Success(resp)
}

// Create 创建 App
//
//	@Summary		创建 App
//	@Tags			【用户】App
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			req	body		domain.CreateAppReq	true	"创建参数"
//	@Success		200		{object}	web.Resp{data=domain.App}	"成功"
//	@Failure		400		{object}	web.Resp					"请求参数错误"
//	@Failure		401		{object}	web.Resp					"未授权"
//	@Router			/api/v1/users/apps [post]
func (h *AppHandler) Create(c *web.Context, req domain.CreateAppReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.usecase.Create(c.Request().Context(), user.ID, &req)
	if err != nil {
		h.logger.ErrorContext(c.Request().Context(), "failed to create app", "error", err, "user_id", user.ID)
		return err
	}
	return c.Success(resp)
}

// Update 更新 App。额度受限时 App 被冻结，返回 err-app-locked。
//
//	@Summary		更新 App
//	@Tags			【用户】App
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			id	path		uuid.UUID				true	"App ID"
//	@Param			req	body		domain.UpdateAppReq		true	"更新参数"
//	@Success		200		{object}	web.Resp{data=domain.App}	"成功"
//	@Failure		401		{object}	web.Resp					"未授权"
//	@Failure		404		{object}	web.Resp					"App 不存在"
//	@Failure		409		{object}	web.Resp					"App 已锁定"
//	@Router			/api/v1/users/apps/{id} [put]
func (h *AppHandler) Update(c *web.Context, req domain.UpdateAppReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.usecase.Update(c.Request().Context(), user.ID, req.ID, &req)
	if err != nil {
		return err
	}
	return c.Success(resp)
}

// Delete 删除 App
//
//	@Summary		删除 App
//	@Tags			【用户】App
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			id	path		uuid.UUID	true	"App ID"
//	@Success		200		{object}	web.Resp	"成功"
//	@Failure		401		{object}	web.Resp	"未授权"
//	@Failure		404		{object}	web.Resp	"App 不存在"
//	@Router			/api/v1/users/apps/{id} [delete]
func (h *AppHandler) Delete(c *web.Context, req domain.DeleteAppReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	if err := h.usecase.Delete(c.Request().Context(), user.ID, req.ID); err != nil {
		return err
	}
	return c.Success(nil)
}

// AdNetworks 返回广告网络列表。列表由 packer/ad_networks.yaml 定义，
// 前端通过该接口渲染下拉框，不自行硬编码。
//
//	@Summary		获取广告网络列表
//	@Tags			【用户】App
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Success		200		{object}	web.Resp{data=[]domain.AdNetworkOption}	"成功"
//	@Failure		401		{object}	web.Resp							"未授权"
//	@Failure		500		{object}	web.Resp							"服务器内部错误"
//	@Router			/api/v1/users/apps/ad-networks [get]
func (h *AppHandler) AdNetworks(c *web.Context, req domain.AdNetworksReq) error {
	networks, err := h.usecase.AdNetworks(c.Request().Context())
	if err != nil {
		h.logger.ErrorContext(c.Request().Context(), "failed to load ad networks", "error", err)
		return err
	}
	return c.Success(networks)
}

// EnqueueBuild 为 App 提交一次打包任务，受套餐配额约束。
//
//	@Summary		提交 App 打包任务
//	@Tags			【用户】App 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			id	path		uuid.UUID					true	"App ID"
//	@Param			req	body		domain.EnqueueAppBuildReq	true	"打包参数"
//	@Success		201		{object}	web.Resp{data=domain.AppBuildJob}	"成功"
//	@Failure		401		{object}	web.Resp						"未授权"
//	@Failure		409		{object}	web.Resp						"额度不足或有进行中任务"
//	@Router			/api/v1/users/apps/{id}/builds [post]
func (h *AppHandler) EnqueueBuild(c *web.Context, req enqueueBuildReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.usecase.EnqueueBuild(c.Request().Context(), user.ID, req.ID, &domain.EnqueueAppBuildReq{
		Platform:    req.Platform,
		Version:     req.Version,
		VersionCode: req.VersionCode,
	})
	if err != nil {
		return err
	}
	return c.Success(resp)
}

// ListBuilds 查询某个 App 的打包历史。
//
//	@Summary		查询 App 打包历史
//	@Tags			【用户】App 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			id	path	uuid.UUID	true	"App ID"
//	@Success		200	{object}	web.Resp{data=[]domain.AppBuildJob}	"成功"
//	@Failure		401	{object}	web.Resp								"未授权"
//	@Router			/api/v1/users/apps/{id}/builds [get]
func (h *AppHandler) ListBuilds(c *web.Context, req domain.DeleteAppReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.usecase.ListBuilds(c.Request().Context(), user.ID, req.ID)
	if err != nil {
		return err
	}
	return c.Success(resp)
}

// GetBuild 查询单个打包任务。
//
//	@Summary		查询打包任务
//	@Tags			【用户】App 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			jobID	path	uuid.UUID	true	"任务 ID"
//	@Success		200	{object}	web.Resp{data=domain.AppBuildJob}	"成功"
//	@Failure		401	{object}	web.Resp							"未授权"
//	@Failure		404	{object}	web.Resp							"任务不存在"
//	@Router			/api/v1/users/apps/builds/{jobID} [get]
func (h *AppHandler) GetBuild(c *web.Context, req buildIDReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.usecase.GetBuild(c.Request().Context(), user.ID, req.JobID)
	if err != nil {
		return err
	}
	return c.Success(resp)
}

// DownloadBuild 下载打包产物。
//
//	@Summary		下载打包产物
//	@Tags			【用户】App 打包
//	@Produce		application/vnd.android.package-archive
//	@Security		MonkeyCodeAIAuth
//	@Param			jobID	path	uuid.UUID	true	"任务 ID"
//	@Failure		400	{object}	web.Resp	"产物不可下载"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Router			/api/v1/users/apps/builds/{jobID}/download [get]
func (h *AppHandler) DownloadBuild(c *web.Context, req buildIDReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	art, err := h.usecase.DownloadBuild(c.Request().Context(), user.ID, req.JobID)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Type", art.ContentType)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", art.FileName))
	return c.File(art.Path)
}

// PublishBuild 把成功的打包任务发布为原生更新版本。
//
//	@Summary		发布为原生更新版本
//	@Tags			【用户】App 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			jobID	path	uuid.UUID	true	"任务 ID"
//	@Success		200	{object}	web.Resp{data=domain.AppBuildRelease}	"成功"
//	@Failure		401	{object}	web.Resp									"未授权"
//	@Router			/api/v1/users/apps/builds/{jobID}/publish [post]
func (h *AppHandler) PublishBuild(c *web.Context, req buildIDReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.usecase.PublishBuild(c.Request().Context(), user.ID, req.JobID)
	if err != nil {
		return err
	}
	return c.Success(resp)
}

// DeleteBuild 删除打包任务。
//
//	@Summary		删除打包任务
//	@Tags			【用户】App 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			jobID	path	uuid.UUID	true	"任务 ID"
//	@Success		200	{object}	web.Resp	"成功"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Router			/api/v1/users/apps/builds/{jobID} [delete]
func (h *AppHandler) DeleteBuild(c *web.Context, req buildIDReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	if err := h.usecase.DeleteBuild(c.Request().Context(), user.ID, req.JobID); err != nil {
		return err
	}
	return c.Success(nil)
}
