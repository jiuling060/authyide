package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/config"
	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/errcode"
)

const (
	// defaultMaxArchiveBytes 在 ObjectStorage.MaxSize 未配置时的兜底上限，
	// 与 overlay-source 的解压上限保持同一量级。
	defaultMaxArchiveBytes int64 = 512 << 20
	// defaultPresignExpires 在 PresignExpires 未配置或非法时的兜底有效期。
	defaultPresignExpires = 24 * time.Hour
	// maxFailureCauseBytes 限制落库的失败原因长度，避免会话侧回传超长输出。
	maxFailureCauseBytes = 512
	// snapshotDirName 是 RepoPrefix 下快照对象的子目录名。
	snapshotDirName = "snapshots"
	// archiveExt 是快照归档的固定扩展名。
	archiveExt = ".tar.gz"
	// minArchiveBytes 低于该字节数视为空工作区。空 tar.gz 本身约 20 字节，
	// 留出足够余量让"只有一个空目录"的情况也判为空。
	minArchiveBytes int64 = 256
)

type snapshotService struct {
	cfg    *config.Config
	repo   domain.SnapshotRepo
	store  domain.SnapshotStore
	tasks  domain.TaskRepo
	logger *slog.Logger
}

func NewSnapshotService(i *do.Injector) (domain.SnapshotService, error) {
	return &snapshotService{
		cfg:    do.MustInvoke[*config.Config](i),
		repo:   do.MustInvoke[domain.SnapshotRepo](i),
		store:  do.MustInvoke[domain.SnapshotStore](i),
		tasks:  do.MustInvoke[domain.TaskRepo](i),
		logger: do.MustInvoke[*slog.Logger](i).With("module", "snapshot.usecase"),
	}, nil
}

// Presign 登记 uploading 快照行并签发上传凭证。
//
// file_count 与 size_bytes 由会话侧先完成打包再上报，因此空工作区与超大小都能
// 在本步拒绝，不需要等上传结束。
func (s *snapshotService) Presign(ctx context.Context, uid uuid.UUID, req *domain.PresignSnapshotReq) (*domain.PresignSnapshotResp, error) {
	task, err := s.ownedTask(ctx, uid, req.TaskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, errcode.ErrSnapshotNotFound
	}
	if req.Archive.FileCount <= 0 {
		return nil, errcode.ErrSnapshotEmpty
	}
	if req.Archive.SizeBytes <= minArchiveBytes {
		return nil, errcode.ErrSnapshotEmpty
	}
	if limit := s.maxArchiveBytes(); limit > 0 && req.Archive.SizeBytes > limit {
		return nil, errcode.ErrSnapshotTooLarge
	}

	id := uuid.New()
	prefix, filename := s.objectPrefixAndName(uid, id)
	uploadURL, accessURL, err := s.store.PresignPut(ctx, prefix, filename, s.presignExpires())
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to presign snapshot upload", "error", err, "task_id", req.TaskID)
		return nil, errcode.ErrSnapshotSyncFailed
	}

	row, err := s.repo.Create(ctx, uid, req.TaskID, path.Join(prefix, filename), req.Archive.SizeBytes, req.Archive.FileCount)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to create snapshot row", "error", err, "task_id", req.TaskID)
		return nil, err
	}

	return &domain.PresignSnapshotResp{
		Snapshot:  s.toSnapshot(ctx, row),
		UploadURL: uploadURL,
		AccessURL: accessURL,
	}, nil
}

// Complete 重读对象校验摘要，一致则在单事务内 promote 并取代同会话更早的快照。
func (s *snapshotService) Complete(ctx context.Context, uid uuid.UUID, id uuid.UUID, req *domain.CompleteSnapshotReq) (*domain.Snapshot, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil || row == nil || row.UserID != uid || row.Status != domain.SnapshotStatusUploading {
		return nil, errcode.ErrSnapshotNotFound
	}

	got, size, err := s.digestObject(ctx, row.ObjectKey)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to read snapshot object", "error", err, "snapshot_id", id)
		_ = s.repo.MarkFailed(ctx, uid, id, fmt.Sprintf("read object: %v", err))
		return nil, errcode.ErrSnapshotSyncFailed
	}
	if got != strings.ToLower(req.SHA256) {
		_ = s.repo.MarkFailed(ctx, uid, id, fmt.Sprintf("sha256 mismatch: expected %s got %s", req.SHA256, got))
		s.logger.WarnContext(ctx, "snapshot sha256 mismatch", "snapshot_id", id, "expected", req.SHA256, "got", got)
		return nil, errcode.ErrSnapshotSyncFailed
	}
	if size != row.SizeBytes {
		_ = s.repo.MarkFailed(ctx, uid, id, fmt.Sprintf("size mismatch: expected %d got %d", row.SizeBytes, size))
		return nil, errcode.ErrSnapshotSyncFailed
	}

	if err := s.promote(ctx, uid, row, got); err != nil {
		return nil, err
	}
	fresh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toSnapshot(ctx, fresh), nil
}

// Fail 把 uploading 快照置为 failed。已终态的行静默成功，便于会话侧重试。
func (s *snapshotService) Fail(ctx context.Context, uid uuid.UUID, id uuid.UUID, req *domain.FailSnapshotReq) error {
	cause := truncateCause(req.Error)
	return s.repo.MarkFailed(ctx, uid, id, cause)
}

// CurrentByTask 返回该会话最新一份 ready 快照。会话不存在或归属不符返回空。
func (s *snapshotService) CurrentByTask(ctx context.Context, uid uuid.UUID, taskID uuid.UUID) (*domain.Snapshot, error) {
	task, err := s.ownedTask(ctx, uid, taskID)
	if err != nil || task == nil {
		return nil, nil
	}
	row, err := s.repo.CurrentByTask(ctx, taskID)
	if err != nil {
		return nil, nil
	}
	return s.toSnapshot(ctx, row), nil
}

// ListMine 返回该用户名下的 ready 快照，按创建时间倒序。
func (s *snapshotService) ListMine(ctx context.Context, uid uuid.UUID, limit int) ([]*domain.Snapshot, error) {
	rows, err := s.repo.ListMine(ctx, uid, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.Snapshot, 0, len(rows))
	cached := make(map[uuid.UUID]*db.Task, len(rows))
	for _, row := range rows {
		out = append(out, s.toSnapshotWithCache(ctx, row, cached))
	}
	return out, nil
}

// Get 校验归属后返回快照。不存在、归属不符或未就绪统一返回不存在。
func (s *snapshotService) Get(ctx context.Context, uid uuid.UUID, id uuid.UUID) (*domain.Snapshot, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil || row == nil || row.UserID != uid || row.Status != domain.SnapshotStatusReady {
		return nil, errcode.ErrSnapshotNotFound
	}
	return s.toSnapshot(ctx, row), nil
}

func (s *snapshotService) promote(ctx context.Context, uid uuid.UUID, row *db.AppSourceSnapshot, sha256 string) error {
	err := s.repo.InTx(ctx, func(tx *db.Tx) error {
		if err := s.repo.SetSHA256(ctx, tx, row.ID, sha256); err != nil {
			return err
		}
		if err := s.repo.PromoteInTx(ctx, tx, row.ID); err != nil {
			return err
		}
		return s.repo.SupersedeOthersInTx(ctx, tx, row.TaskID, row.ID)
	})
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to promote snapshot", "error", err, "snapshot_id", row.ID)
		_ = s.repo.MarkFailed(ctx, uid, row.ID, fmt.Sprintf("promote: %v", err))
		return errcode.ErrSnapshotSyncFailed
	}
	return nil
}

func (s *snapshotService) digestObject(ctx context.Context, key string) (string, int64, error) {
	rc, err := s.store.GetObject(ctx, key)
	if err != nil {
		return "", 0, err
	}
	defer rc.Close()

	hasher := sha256.New()
	n, err := io.Copy(hasher, rc)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), n, nil
}

// toSnapshot 转换并填充 Stale 与 TaskTitle。
func (s *snapshotService) toSnapshot(ctx context.Context, row *db.AppSourceSnapshot) *domain.Snapshot {
	return s.toSnapshotWithCache(ctx, row, nil)
}

func (s *snapshotService) toSnapshotWithCache(ctx context.Context, row *db.AppSourceSnapshot, cached map[uuid.UUID]*db.Task) *domain.Snapshot {
	if row == nil {
		return nil
	}
	snap := &domain.Snapshot{}
	snap.From(row)

	task, ok := cached[row.TaskID]
	if !ok {
		var err error
		task, err = s.tasks.GetByID(ctx, row.TaskID)
		if err != nil {
			task = nil
		}
		if cached != nil {
			cached[row.TaskID] = task
		}
	}
	if task == nil {
		return snap
	}
	snap.TaskTitle = task.Title
	// 会话最近活动时间晚于快照完成时间即为过期。completed_at 为空表示尚未
	// promote，此时不参与过期判定。
	if row.CompletedAt != nil && task.LastActiveAt.After(*row.CompletedAt) {
		snap.Stale = true
	}
	return snap
}

// ownedTask 返回归属 uid 的会话，不存在或归属不符返回 nil。
func (s *snapshotService) ownedTask(ctx context.Context, uid, taskID uuid.UUID) (*db.Task, error) {
	task, err := s.tasks.GetByID(ctx, taskID)
	if err != nil {
		if db.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if task.UserID != uid {
		return nil, nil
	}
	return task, nil
}

func (s *snapshotService) objectPrefixAndName(uid, id uuid.UUID) (string, string) {
	prefix := path.Join(s.cfg.ObjectStorage.RepoPrefix, snapshotDirName, uid.String())
	return strings.Trim(prefix, "/"), id.String() + archiveExt
}

func (s *snapshotService) maxArchiveBytes() int64 {
	if n := s.cfg.ObjectStorage.MaxSize; n > 0 {
		return n
	}
	return defaultMaxArchiveBytes
}

func (s *snapshotService) presignExpires() time.Duration {
	raw := strings.TrimSpace(s.cfg.ObjectStorage.PresignExpires)
	if raw == "" {
		return defaultPresignExpires
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultPresignExpires
	}
	return d
}

func truncateCause(cause string) string {
	cause = strings.TrimSpace(cause)
	if len(cause) == 0 {
		return "snapshot sync failed"
	}
	if len(cause) > maxFailureCauseBytes {
		return cause[:maxFailureCauseBytes]
	}
	return cause
}
