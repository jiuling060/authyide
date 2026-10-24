package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/db/appsourcesnapshot"
	"github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/pkg/entx"
)

const defaultListLimit = 50

type snapshotRepo struct {
	db *db.Client
}

func NewSnapshotRepo(i *do.Injector) (domain.SnapshotRepo, error) {
	return &snapshotRepo{db: do.MustInvoke[*db.Client](i)}, nil
}

func (r *snapshotRepo) Create(ctx context.Context, uid, taskID uuid.UUID, objectKey string, sizeBytes int64, fileCount int) (*db.AppSourceSnapshot, error) {
	row, err := r.db.AppSourceSnapshot.Create().
		SetUserID(uid).
		SetTaskID(taskID).
		SetObjectKey(objectKey).
		SetSizeBytes(sizeBytes).
		SetFileCount(fileCount).
		SetStatus(domain.SnapshotStatusUploading).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *snapshotRepo) SetSHA256(ctx context.Context, tx *db.Tx, id uuid.UUID, sha256 string) error {
	_, err := tx.AppSourceSnapshot.Update().
		Where(appsourcesnapshot.ID(id),
			appsourcesnapshot.StatusEQ(domain.SnapshotStatusUploading)).
		SetSha256(sha256).
		Save(ctx)
	return err
}

func (r *snapshotRepo) PromoteInTx(ctx context.Context, tx *db.Tx, id uuid.UUID) error {
	_, err := tx.AppSourceSnapshot.Update().
		Where(appsourcesnapshot.ID(id),
			appsourcesnapshot.StatusEQ(domain.SnapshotStatusUploading)).
		SetStatus(domain.SnapshotStatusReady).
		SetSuperseded(false).
		SetCompletedAt(time.Now()).
		Save(ctx)
	return err
}

func (r *snapshotRepo) MarkFailed(ctx context.Context, uid, id uuid.UUID, cause string) error {
	_, err := r.db.AppSourceSnapshot.Update().
		Where(appsourcesnapshot.ID(id), appsourcesnapshot.UserID(uid),
			appsourcesnapshot.StatusEQ(domain.SnapshotStatusUploading)).
		SetStatus(domain.SnapshotStatusFailed).
		SetError(cause).
		Save(ctx)
	return err
}

func (r *snapshotRepo) SupersedeOthersInTx(ctx context.Context, tx *db.Tx, taskID, id uuid.UUID) error {
	_, err := tx.AppSourceSnapshot.Update().
		Where(appsourcesnapshot.TaskID(taskID),
			appsourcesnapshot.StatusEQ(domain.SnapshotStatusReady),
			appsourcesnapshot.IDNEQ(id)).
		SetSuperseded(true).
		Save(ctx)
	return err
}

func (r *snapshotRepo) InTx(ctx context.Context, fn func(tx *db.Tx) error) error {
	return entx.WithTx2(ctx, r.db, fn)
}

func (r *snapshotRepo) GetByID(ctx context.Context, id uuid.UUID) (*db.AppSourceSnapshot, error) {
	row, err := r.db.AppSourceSnapshot.Query().
		Where(appsourcesnapshot.ID(id)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *snapshotRepo) CurrentByTask(ctx context.Context, taskID uuid.UUID) (*db.AppSourceSnapshot, error) {
	row, err := r.db.AppSourceSnapshot.Query().
		Where(appsourcesnapshot.TaskID(taskID),
			appsourcesnapshot.StatusEQ(domain.SnapshotStatusReady),
			appsourcesnapshot.Superseded(false)).
		Order(db.Desc(appsourcesnapshot.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *snapshotRepo) ListMine(ctx context.Context, uid uuid.UUID, limit int) ([]*db.AppSourceSnapshot, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	rows, err := r.db.AppSourceSnapshot.Query().
		Where(appsourcesnapshot.UserID(uid),
			appsourcesnapshot.StatusEQ(domain.SnapshotStatusReady)).
		Order(db.Desc(appsourcesnapshot.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
