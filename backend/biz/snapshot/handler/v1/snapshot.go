package v1

import (
	"log/slog"

	"github.com/GoYoko/web"
	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/errcode"
	"github.com/chaitin/MonkeyCode/backend/middleware"
)

// SnapshotHandler 会话源码快照处理器。
type SnapshotHandler struct {
	service domain.SnapshotService
	logger  *slog.Logger
}

// presignReq 绑定快照签发请求体。
type presignReq struct {
	TaskID   uuid.UUID   `json:"task_id" validate:"required"`
	Archive  ArchiveInfo `json:"archive" validate:"required"`
	FileName string      `json:"file_name" validate:"required"`
}

// completeReq 绑定 :id 路径参数与摘要上报体。
type completeReq struct {
	ID     uuid.UUID `param:"id" validate:"required"`
	SHA256 string    `json:"sha256" validate:"required,len=64"`
}

// failReq 绑定 :id 路径参数与失败原因上报体。原因可选：会话侧可能已经来不及
// 收集原因（例如页面被关闭）。
type failReq struct {
	ID    uuid.UUID `param:"id" validate:"required"`
	Error string    `json:"error"`
}

// currentReq 绑定按会话取当前快照的查询参数。
type currentReq struct {
	TaskID uuid.UUID `query:"task_id" validate:"required"`
}

// archiveInfo 归档元数据，独立于 req 结构以便单独校验。
type ArchiveInfo struct {
	FileCount int   `json:"file_count" validate:"required,gt=0"`
	SizeBytes int64 `json:"size_bytes" validate:"required,gt=0"`
}

// listSnapshotsReq 绑定快照列表查询参数。
type listSnapshotsReq struct {
	Limit int `query:"limit"`
}

// NewSnapshotHandler 创建处理器并注册路由。
func NewSnapshotHandler(i *do.Injector) (*SnapshotHandler, error) {
	w := do.MustInvoke[*web.Web](i)
	logger := do.MustInvoke[*slog.Logger](i)
	service := do.MustInvoke[domain.SnapshotService](i)
	auth := do.MustInvoke[*middleware.AuthMiddleware](i)
	targetActive := do.MustInvoke[*middleware.TargetActiveMiddleware](i)

	h := &SnapshotHandler{
		logger:  logger.With("component", "handler.snapshots"),
		service: service,
	}

	v1 := w.Group("/api/v1/users/snapshots")
	v1.Use(auth.Auth(), targetActive.TargetActive())
	v1.POST("/upload", web.BindHandler(h.Presign))
	v1.POST("/complete", web.BindHandler(h.Complete))
	v1.POST("/:id/fail", web.BindHandler(h.Fail))
	v1.GET("", web.BindHandler(h.List))
	v1.GET("/current", web.BindHandler(h.CurrentByTask))
	v1.GET("/:id", web.BindHandler(h.Get))

	return h, nil
}

// Presign 校验会话归属后登记快照行并签发上传凭证。
//
//	@Summary	会话源码快照签发
//	@Tags		【用户】快照
//	@Accept		json
//	@Produce	json
//	@Security	MonkeyCodeAIAuth
//	@Param		body	body		presignReq	true	"会话主键与归档元数据"
//	@Success	200	{object}	web.Resp{data=domain.PresignSnapshotResp}	"签发成功"
//	@Failure	400	{object}	web.Resp	"空工作区或超大小"
//	@Failure	401	{object}	web.Resp	"未授权"
//	@Failure	500	{object}	web.Resp	"服务器内部错误"
//	@Router		/api/v1/users/snapshots/upload [post]
func (h *SnapshotHandler) Presign(c *web.Context, req presignReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	resp, err := h.service.Presign(c.Request().Context(), user.ID, &domain.PresignSnapshotReq{
		TaskID:   req.TaskID,
		Archive:  domain.ArchiveInfo(req.Archive),
		FileName: req.FileName,
	})
	if err != nil {
		h.logger.ErrorContext(c.Request().Context(), "failed to presign snapshot", "error", err, "user_id", user.ID)
		return err
	}
	return c.Success(resp)
}

// Complete 重读对象校验摘要，一致则置为可用并取代同会话更早的快照。
//
//	@Summary	会话源码快照完成
//	@Tags		【用户】快照
//	@Accept		json
//	@Produce	json
//	@Security	MonkeyCodeAIAuth
//	@Param		id	path		uuid.UUID	true	"快照主键"
//	@Param		body	body		completeReq	true	"对象摘要"
//	@Success	200	{object}	web.Resp{data=domain.Snapshot}	"校验通过"
//	@Failure	401	{object}	web.Resp	"未授权"
//	@Failure	500	{object}	web.Resp	"摘要不一致或读取失败"
//	@Router		/api/v1/users/snapshots/complete [post]
func (h *SnapshotHandler) Complete(c *web.Context, req completeReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	snap, err := h.service.Complete(c.Request().Context(), user.ID, req.ID, &domain.CompleteSnapshotReq{
		SHA256: req.SHA256,
	})
	if err != nil {
		h.logger.ErrorContext(c.Request().Context(), "failed to complete snapshot", "error", err, "user_id", user.ID, "snapshot_id", req.ID)
		return err
	}
	return c.Success(snap)
}

// Fail 把已登记但未完成的快照置为失败。会话侧上传中断后没有它，uploading 行
// 会永远挂在数据库里。
//
//	@Summary	会话源码快照失败
//	@Tags		【用户】快照
//	@Accept		json
//	@Produce	json
//	@Security	MonkeyCodeAIAuth
//	@Param		id	path		uuid.UUID	true	"快照主键"
//	@Param		body	body		failReq	false	"失败原因，可省略"
//	@Success	200	{object}	web.Resp	"置为失败"
//	@Failure	401	{object}	web.Resp	"未授权"
//	@Failure	500	{object}	web.Resp	"服务器内部错误"
//	@Router		/api/v1/users/snapshots/{id}/fail [post]
func (h *SnapshotHandler) Fail(c *web.Context, req failReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	if err := h.service.Fail(c.Request().Context(), user.ID, req.ID, &domain.FailSnapshotReq{
		Error: req.Error,
	}); err != nil {
		h.logger.ErrorContext(c.Request().Context(), "failed to mark snapshot failed", "error", err, "user_id", user.ID, "snapshot_id", req.ID)
		return err
	}
	return c.Success(nil)
}

// List 列出当前用户的全部可用快照，按创建时间倒序。
//
//	@Summary	会话源码快照列表
//	@Tags		【用户】快照
//	@Accept		json
//	@Produce	json
//	@Security	MonkeyCodeAIAuth
//	@Param		limit	query		int			false	"返回条数上限，默认 50"
//	@Success	200	{object}	web.Resp	"快照列表"
//	@Failure	401	{object}	web.Resp	"未授权"
//	@Failure	500	{object}	web.Resp	"服务器内部错误"
//	@Router		/api/v1/users/snapshots [get]
func (h *SnapshotHandler) List(c *web.Context, req listSnapshotsReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	rows, err := h.service.ListMine(c.Request().Context(), user.ID, req.Limit)
	if err != nil {
		h.logger.ErrorContext(c.Request().Context(), "failed to list snapshots", "error", err, "user_id", user.ID)
		return err
	}
	return c.Success(rows)
}

// CurrentByTask 返回该会话最新一份可用快照，没有则 data 为空。
//
//	@Summary	会话最新源码快照
//	@Tags		【用户】快照
//	@Accept		json
//	@Produce	json
//	@Security	MonkeyCodeAIAuth
//	@Param		task_id	query	uuid.UUID	true	"会话主键"
//	@Success	200	{object}	web.Resp{data=domain.Snapshot}	"快照详情，无则为空"
//	@Failure	401	{object}	web.Resp	"未授权"
//	@Failure	500	{object}	web.Resp	"服务器内部错误"
//	@Router		/api/v1/users/snapshots/current [get]
func (h *SnapshotHandler) CurrentByTask(c *web.Context, req currentReq) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	snap, err := h.service.CurrentByTask(c.Request().Context(), user.ID, req.TaskID)
	if err != nil {
		h.logger.ErrorContext(c.Request().Context(), "failed to get current snapshot", "error", err, "user_id", user.ID, "task_id", req.TaskID)
		return err
	}
	return c.Success(snap)
}

// Get 读取单个快照，含过期标记与来源会话标题。
//
//	@Summary	会话源码快照详情
//	@Tags		【用户】快照
//	@Accept		json
//	@Produce	json
//	@Security	MonkeyCodeAIAuth
//	@Param		id	path		uuid.UUID	true	"快照主键"
//	@Success	200	{object}	web.Resp{data=domain.Snapshot}	"快照详情"
//	@Failure	401	{object}	web.Resp	"未授权"
//	@Failure	500	{object}	web.Resp	"快照不存在"
//	@Router		/api/v1/users/snapshots/{id} [get]
func (h *SnapshotHandler) Get(c *web.Context, id uuid.UUID) error {
	user := middleware.GetUser(c)
	if user == nil {
		return errcode.ErrUnauthorized
	}
	snap, err := h.service.Get(c.Request().Context(), user.ID, id)
	if err != nil {
		return err
	}
	return c.Success(snap)
}
