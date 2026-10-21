// Package v1 exposes the mobile app builder API to the admin console.
package v1

import (
	"fmt"
	"io"
	"mime/multipart"

	"github.com/GoYoko/web"
	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/usecase"
	"github.com/chaitin/MonkeyCode/backend/config"
	"github.com/chaitin/MonkeyCode/backend/consts"
	"github.com/chaitin/MonkeyCode/backend/errcode"
	"github.com/chaitin/MonkeyCode/backend/middleware"
)

const (
	routePrefix  = "/api/v1/admin/app-builder"
	artifactName = "authyide.apk"
)

// BuildHandler is the HTTP entry of the app builder.
type BuildHandler struct {
	usecase usecase.Usecase
	cfg     *config.Config
}

// NewBuildHandler creates the handler and registers its routes. When the
// module is disabled no route is registered and every call is a 404.
func NewBuildHandler(i *do.Injector) (*BuildHandler, error) {
	w := do.MustInvoke[*web.Web](i)
	cfg := do.MustInvoke[*config.Config](i)

	h := &BuildHandler{
		usecase: do.MustInvoke[usecase.Usecase](i),
		cfg:     cfg,
	}
	if !cfg.MobileBuild.Enabled {
		return h, nil
	}

	auth := do.MustInvoke[*middleware.AuthMiddleware](i)
	g := w.Group(routePrefix)
	g.Use(auth.Auth(), auth.AdminAuth())
	g.GET("", web.BindHandler(h.Status))
	g.GET("/branding", web.BindHandler(h.GetBranding))
	g.PUT("/branding", web.BindHandler(h.SaveBranding))
	g.POST("/branding/assets", web.BindHandler(h.UploadAsset))
	g.DELETE("/branding/assets", web.BindHandler(h.ClearAsset))
	g.GET("/builds", web.BindHandler(h.ListJobs))
	g.POST("/builds", web.BindHandler(h.EnqueueBuild))
	g.GET("/builds/:id", web.BindHandler(h.GetJob))
	g.GET("/builds/:id/download", web.BindHandler(h.DownloadArtifact))
	g.POST("/builds/:id/publish", web.BindHandler(h.Publish))
	g.GET("/releases", web.BindHandler(h.Releases))
	g.GET("/ota/config", web.BindHandler(h.OtaStatus))
	g.POST("/ota/publish", web.BindHandler(h.PublishOta))

	return h, nil
}

type emptyReq struct{}
type idReq struct {
	ID string `param:"id" validate:"required,uuid"`
}
type slotReq struct {
	Slot string `form:"slot" validate:"required"`
}
type uploadAssetReq struct {
	File *multipart.FileHeader `form:"file" validate:"required"`
	Slot string                `form:"slot" validate:"required"`
}
type listJobsReq struct {
	Limit int `query:"limit"`
}
type statusReq struct{}

// Status reports the app builder capabilities.
//
//	@Summary		获取 APP 打包能力
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Success		200	{object}	web.Resp{data=domain.StatusResp}	"成功"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder [get]
func (h *BuildHandler) Status(c *web.Context, req statusReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	st, err := h.usecase.Status(c.Request().Context())
	if err != nil {
		return err
	}
	return c.Success(st)
}

// GetBranding returns the saved branding, or the app.json defaults.
//
//	@Summary		获取移动端品牌配置
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Success		200	{object}	web.Resp{data=domain.Branding}	"成功"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/branding [get]
func (h *BuildHandler) GetBranding(c *web.Context, req emptyReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	b, err := h.usecase.GetBranding(c.Request().Context())
	if err != nil {
		return err
	}
	return c.Success(b)
}

// SaveBranding updates the branding fields that are present in the request.
//
//	@Summary		保存移动端品牌配置
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			req	body	domain.UpdateBrandingReq	true	"品牌配置"
//	@Success		200	{object}	web.Resp{data=domain.Branding}	"成功"
//	@Failure		400	{object}	web.Resp	"配置校验失败"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/branding [put]
func (h *BuildHandler) SaveBranding(c *web.Context, req domain.UpdateBrandingReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	b, err := h.usecase.SaveBranding(c.Request().Context(), &req)
	if err != nil {
		return err
	}
	return c.Success(b)
}

// UploadAsset replaces one branding asset.
//
//	@Summary		上传品牌素材
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			file	formData	file	true	"PNG 图片"
//	@Param			slot	formData	string	true	"素材槽位"
//	@Success		200	{object}	web.Resp{data=domain.Branding}	"成功"
//	@Failure		400	{object}	web.Resp	"非 PNG 或超大小"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/branding/assets [post]
func (h *BuildHandler) UploadAsset(c *web.Context, req uploadAssetReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	data, err := h.readFile(req.File)
	if err != nil {
		return err
	}
	b, err := h.usecase.UploadAsset(c.Request().Context(), req.Slot, data)
	if err != nil {
		return err
	}
	return c.Success(b)
}

// ClearAsset removes one branding asset and falls back to the bundled default.
//
//	@Summary		删除品牌素材
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			slot	query	string	true	"素材槽位"
//	@Success		200	{object}	web.Resp{data=domain.Branding}	"成功"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/branding/assets [delete]
func (h *BuildHandler) ClearAsset(c *web.Context, req slotReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	b, err := h.usecase.ClearAsset(c.Request().Context(), req.Slot)
	if err != nil {
		return err
	}
	return c.Success(b)
}

// ListJobs returns the most recent packaging jobs.
//
//	@Summary		获取打包任务列表
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			limit	query	int	false	"返回条数，默认 20"
//	@Success		200	{object}	web.Resp{data=[]domain.BuildJob}	"成功"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/builds [get]
func (h *BuildHandler) ListJobs(c *web.Context, req listJobsReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	jobs, err := h.usecase.ListJobs(c.Request().Context(), req.Limit)
	if err != nil {
		return err
	}
	return c.Success(jobs)
}

// EnqueueBuild starts a new packaging job.
//
//	@Summary		发起打包
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			req	body	domain.EnqueueBuildReq	true	"打包参数"
//	@Success		200	{object}	web.Resp{data=domain.BuildJob}	"成功"
//	@Failure		409	{object}	web.Resp	"已有任务在跑"
//	@Failure		400	{object}	web.Resp	"版本不合法"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/builds [post]
func (h *BuildHandler) EnqueueBuild(c *web.Context, req domain.EnqueueBuildReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	user := middleware.GetUser(c)
	job, err := h.usecase.EnqueueBuild(c.Request().Context(), user.ID, &req)
	if err != nil {
		return err
	}
	return c.Success(job)
}

// GetJob returns one packaging job.
//
//	@Summary		获取打包任务详情
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			id	path	string	true	"任务 ID"
//	@Success		200	{object}	web.Resp{data=domain.BuildJob}	"成功"
//	@Failure		400	{object}	web.Resp	"任务不存在"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/builds/{id} [get]
func (h *BuildHandler) GetJob(c *web.Context, req idReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	id, err := h.parseID(req.ID)
	if err != nil {
		return err
	}
	job, err := h.usecase.GetJob(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.Success(job)
}

// DownloadArtifact streams the APK of a succeeded job.
//
//	@Summary		下载 APK
//	@Tags			【管理员】移动端 APP 打包
//	@Produce		application/vnd.android.package-archive
//	@Security		MonkeyCodeAIAuth
//	@Param			id	path	string	true	"任务 ID"
//	@Failure		400	{object}	web.Resp	"产物不可下载"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/builds/{id}/download [get]
func (h *BuildHandler) DownloadArtifact(c *web.Context, req idReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	id, err := h.parseID(req.ID)
	if err != nil {
		return err
	}
	path, err := h.usecase.DownloadArtifact(c.Request().Context(), id)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Type", "application/vnd.android.package-archive")
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", artifactName))
	return c.File(path)
}

// Publish promotes a succeeded job to the native update channel.
//
//	@Summary		发布为原生更新版本
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			id	path	string	true	"任务 ID"
//	@Success		200	{object}	web.Resp{data=domain.ReleaseStatus}	"成功"
//	@Failure		400	{object}	web.Resp	"版本过低或产物缺失"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/builds/{id}/publish [post]
func (h *BuildHandler) Publish(c *web.Context, req idReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	id, err := h.parseID(req.ID)
	if err != nil {
		return err
	}
	st, err := h.usecase.Publish(c.Request().Context(), id)
	if err != nil {
		return err
	}
	return c.Success(st)
}

// Releases reports every release channel.
//
//	@Summary		获取发布状态
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Success		200	{object}	web.Resp{data=domain.ReleasesResp}	"成功"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/releases [get]
func (h *BuildHandler) Releases(c *web.Context, req emptyReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	st, err := h.usecase.Releases(c.Request().Context())
	if err != nil {
		return err
	}
	return c.Success(st)
}

// OtaStatus reports whether OTA publishing is available.
//
//	@Summary		获取 OTA 发布状态
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Success		200	{object}	web.Resp{data=domain.OtaStatus}	"成功"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/ota/config [get]
func (h *BuildHandler) OtaStatus(c *web.Context, req emptyReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	st, err := h.usecase.OtaStatus(c.Request().Context())
	if err != nil {
		return err
	}
	return c.Success(st)
}

// PublishOta writes the OTA manifest.
//
//	@Summary		发布 OTA 版本
//	@Tags			【管理员】移动端 APP 打包
//	@Accept			json
//	@Produce		json
//	@Security		MonkeyCodeAIAuth
//	@Param			req	body	domain.PublishOtaReq	true	"OTA 版本与地址"
//	@Success		200	{object}	web.Resp{data=domain.OtaStatus}	"成功"
//	@Failure		400	{object}	web.Resp	"OTA 未启用或版本过低"
//	@Failure		401	{object}	web.Resp	"未授权"
//	@Failure		403	{object}	web.Resp	"非管理员"
//	@Router			/api/v1/admin/app-builder/ota/publish [post]
func (h *BuildHandler) PublishOta(c *web.Context, req domain.PublishOtaReq) error {
	if err := h.requireAdmin(c); err != nil {
		return err
	}
	st, err := h.usecase.PublishOta(c.Request().Context(), &req)
	if err != nil {
		return err
	}
	return c.Success(st)
}

// requireAdmin repeats the AdminAuth check inside the handler so the routes
// stay correct if the middleware chain is ever reordered.
func (h *BuildHandler) requireAdmin(c *web.Context) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	if user.Role != consts.UserRoleAdmin {
		return errcode.ErrForbidden
	}
	return nil
}

func (h *BuildHandler) parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errcode.ErrBadRequest.Wrap(fmt.Errorf("invalid id"))
	}
	return id, nil
}

// readFile loads the uploaded asset, enforcing the configured size limit.
func (h *BuildHandler) readFile(header *multipart.FileHeader) ([]byte, error) {
	if header == nil {
		return nil, errcode.ErrBadRequest.Wrap(fmt.Errorf("file is required"))
	}
	limit := h.cfg.MobileBuild.MaxAssetBytes
	if limit <= 0 {
		limit = 4 << 20
	}
	if header.Size > limit {
		return nil, errcode.ErrAssetInvalid.Wrap(fmt.Errorf("file exceeds %d bytes", limit))
	}
	file, err := header.Open()
	if err != nil {
		return nil, errcode.ErrAssetInvalid.Wrap(err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, errcode.ErrAssetInvalid.Wrap(err)
	}
	if int64(len(data)) > limit {
		return nil, errcode.ErrAssetInvalid.Wrap(fmt.Errorf("file exceeds %d bytes", limit))
	}
	return data, nil
}
