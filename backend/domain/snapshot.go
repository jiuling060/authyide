package domain

import (
	"context"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/db"
)

// 快照状态。uploading 表示已登记但对象尚未校验完成；ready 可被 App 绑定；
// failed 不可恢复，重试必须新建一行。
const (
	SnapshotStatusUploading = "uploading"
	SnapshotStatusReady     = "ready"
	SnapshotStatusFailed    = "failed"
)

// SnapshotService 会话源码快照业务逻辑接口。
//
// 所有方法都以归属校验为前提：uid 与快照不符统一返回 ErrSnapshotNotFound，
// 不暴露快照是否存在。
type SnapshotService interface {
	// Presign 校验会话归属后登记快照行并签发上传凭证。
	// 返回的 Snapshot 状态恒为 uploading。
	Presign(ctx context.Context, uid uuid.UUID, req *PresignSnapshotReq) (*PresignSnapshotResp, error)

	// Complete 比对上报摘要与对象实际摘要，一致则在单事务内置 ready 并把同会话
	// 更早的 ready 快照置 superseded。
	Complete(ctx context.Context, uid uuid.UUID, id uuid.UUID, req *CompleteSnapshotReq) (*Snapshot, error)

	// Fail 把 uploading 快照置为 failed 并写入错误信息。
	Fail(ctx context.Context, uid uuid.UUID, id uuid.UUID, req *FailSnapshotReq) error

	// CurrentByTask 返回该会话最新一份 ready 快照，无则返回 nil，无错误。
	CurrentByTask(ctx context.Context, uid uuid.UUID, taskID uuid.UUID) (*Snapshot, error)

	// ListMine 返回该用户名下的 ready 快照，按创建时间倒序，最多 limit 条。
	// limit 小于等于 0 时按默认值 50。
	ListMine(ctx context.Context, uid uuid.UUID, limit int) ([]*Snapshot, error)

	// Get 校验归属后返回快照。
	Get(ctx context.Context, uid uuid.UUID, id uuid.UUID) (*Snapshot, error)
}

// SnapshotRepo 快照数据访问接口。
type SnapshotRepo interface {
	// Create 落一行 uploading 快照并返回。fileCount 与 sizeBytes 由会话侧打包
	// 时上报，Presign 阶段已完成校验。
	Create(ctx context.Context, uid, taskID uuid.UUID, objectKey string, sizeBytes int64, fileCount int) (*db.AppSourceSnapshot, error)

	// SetSHA256 在 PromoteInTx 之前写入校验通过的摘要；只允许 uploading 行。
	SetSHA256(ctx context.Context, tx *db.Tx, id uuid.UUID, sha256 string) error

	// PromoteInTx 在调用方提供的 tx 内把该行置 ready 并写 completed_at；
	// 只允许 uploading 行。
	PromoteInTx(ctx context.Context, tx *db.Tx, id uuid.UUID) error

	// MarkFailed 把 uploading 行置为 failed 并写入错误原因。已终态的行静默成功，
	// 便于会话侧重试；归属以 Where 条件约束，不影响返回值。
	MarkFailed(ctx context.Context, uid, id uuid.UUID, cause string) error

	// SupersedeOthersInTx 在调用方提供的 tx 内把同会话其余 ready 快照置
	// superseded，id 是本轮刚提升的那行，需排除在外。与 PromoteInTx 共用同一 tx。
	SupersedeOthersInTx(ctx context.Context, tx *db.Tx, taskID, id uuid.UUID) error

	// InTx 在单个事务内执行 fn；fn 返回非 nil 时整体回滚。
	InTx(ctx context.Context, fn func(tx *db.Tx) error) error

	// GetByID 按 ID 查，不做归属过滤；归属校验在 service 层。
	GetByID(ctx context.Context, id uuid.UUID) (*db.AppSourceSnapshot, error)

	// CurrentByTask 返回该会话最新一份 ready 且未 superseded 的快照。
	CurrentByTask(ctx context.Context, taskID uuid.UUID) (*db.AppSourceSnapshot, error)

	// ListMine 返回该用户名下的 ready 快照，按创建时间倒序，最多 limit 条。
	ListMine(ctx context.Context, uid uuid.UUID, limit int) ([]*db.AppSourceSnapshot, error)
}

// PresignSnapshotReq 请求签发上传凭证。FileCount 与 SizeBytes 由会话侧先完成
// 打包再上报，Presign 阶段即可判定空快照与超大小。
type PresignSnapshotReq struct {
	TaskID   uuid.UUID   `json:"task_id" validate:"required"`
	Archive  ArchiveInfo `json:"archive" validate:"required"`
	FileName string      `json:"file_name" validate:"required"`
}

// ArchiveInfo 会话侧打包产物的元数据。
type ArchiveInfo struct {
	FileCount int   `json:"file_count" validate:"required,gt=0"`
	SizeBytes int64 `json:"size_bytes" validate:"required,gt=0"`
}

// PresignSnapshotResp 签发结果，含上传凭证与已登记的快照行。
type PresignSnapshotResp struct {
	Snapshot  *Snapshot `json:"snapshot"`
	UploadURL string    `json:"upload_url"`
	AccessURL string    `json:"access_url"`
}

// CompleteSnapshotReq 上报对象摘要，供后端重读对象校验传输完整性。
type CompleteSnapshotReq struct {
	SHA256 string `json:"sha256" validate:"required,len=64"`
}

// FailSnapshotReq 上报同步失败原因。
type FailSnapshotReq struct {
	Error string `json:"error"`
}

// SnapshotStore 是快照 service 与 runner 访问对象存储所需的最小接口。
// *pkg/oss.Client 不直接实现它（Presign 返回自己的类型），由 biz/snapshot
// 的 ossStore 适配；service 与 runner 的单元测试可以注入内存假实现。
type SnapshotStore interface {
	// GetObject 读取完整对象键指向的对象体。调用方负责关闭返回值。
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
	// PresignPut 对 prefix 与 filename 拼出的对象签发上传凭证。
	PresignPut(ctx context.Context, prefix, filename string, expires time.Duration) (uploadURL, accessURL string, err error)
}

// Snapshot 会话源码快照。
type Snapshot struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	TaskID      uuid.UUID  `json:"task_id"`
	ObjectKey   string     `json:"object_key"`
	SHA256      string     `json:"sha256"`
	SizeBytes   int64      `json:"size_bytes"`
	FileCount   int        `json:"file_count"`
	Status      string     `json:"status"`
	Superseded  bool       `json:"superseded"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Stale 是派生字段，不落库：会话最近活动时间晚于 completed_at 即为过期。
	Stale bool `json:"stale"`

	// TaskTitle 来自会话记录，仅用于展示，不参与任何判定。
	TaskTitle string `json:"task_title,omitempty"`
}

// From 从数据库模型转换为领域模型。Stale 与 TaskTitle 由 service 层填充。
func (s *Snapshot) From(src *db.AppSourceSnapshot) *Snapshot {
	if src == nil {
		return s
	}
	s.ID = src.ID
	s.UserID = src.UserID
	s.TaskID = src.TaskID
	s.ObjectKey = src.ObjectKey
	s.SHA256 = src.Sha256
	s.SizeBytes = src.SizeBytes
	s.FileCount = src.FileCount
	s.Status = src.Status
	s.Superseded = src.Superseded
	s.Error = src.Error
	s.CreatedAt = src.CreatedAt
	s.UpdatedAt = src.UpdatedAt
	s.CompletedAt = src.CompletedAt
	return s
}
