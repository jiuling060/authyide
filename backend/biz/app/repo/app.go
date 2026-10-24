package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/db/app"
	"github.com/chaitin/MonkeyCode/backend/db/entitlement"
	"github.com/chaitin/MonkeyCode/backend/domain"
)

type appRepo struct {
	db *db.Client
}

func NewAppRepo(i *do.Injector) (domain.AppRepo, error) {
	return &appRepo{db: do.MustInvoke[*db.Client](i)}, nil
}

func (r *appRepo) List(ctx context.Context, uid uuid.UUID) ([]*db.App, error) {
	rows, err := r.db.App.Query().
		Where(app.UserID(uid)).
		Order(db.Desc(app.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}
	return rows, nil
}

func (r *appRepo) Get(ctx context.Context, uid, id uuid.UUID) (*db.App, error) {
	row, err := r.db.App.Query().
		Where(app.ID(id), app.UserID(uid)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// GetByID resolves one app without an owner check, for the admin views.
func (r *appRepo) GetByID(ctx context.Context, id uuid.UUID) (*db.App, error) {
	row, err := r.db.App.Query().
		Where(app.ID(id)).
		Only(ctx)
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *appRepo) Create(ctx context.Context, uid uuid.UUID, req *domain.CreateAppReq) (*db.App, error) {
	row, err := r.db.App.Create().
		SetUserID(uid).
		SetName(req.Name).
		SetPackageID(req.PackageID).
		SetDescription(req.Description).
		SetAdNetwork(req.AdNetwork).
		SetAdAppID(req.AdAppID).
		SetEnabledPlatforms(req.EnabledPlatforms).
		SetNillableSourceSnapshotID(req.SourceSnapshotID).
		SetStatus("draft").
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create app: %w", err)
	}
	return row, nil
}

func (r *appRepo) Update(ctx context.Context, uid, id uuid.UUID, req *domain.UpdateAppReq) (*db.App, error) {
	update := r.db.App.UpdateOneID(id)
	if req.Name != nil {
		update = update.SetName(*req.Name)
	}
	if req.Description != nil {
		update = update.SetDescription(*req.Description)
	}
	if req.IconKey != nil {
		update = update.SetIconKey(*req.IconKey)
	}
	if req.AdNetwork != nil {
		update = update.SetAdNetwork(*req.AdNetwork)
	}
	if req.AdAppID != nil {
		update = update.SetAdAppID(*req.AdAppID)
	}
	if req.EnabledPlatforms != nil {
		update = update.SetEnabledPlatforms(req.EnabledPlatforms)
	}
	if req.SourceSnapshotID != nil {
		if inner := *req.SourceSnapshotID; inner != nil {
			update = update.SetSourceSnapshotID(*inner)
		} else {
			update = update.ClearSourceSnapshotID()
		}
	}

	_, err := update.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update app: %w", err)
	}
	// re-read so the returned row is always the current state
	return r.Get(ctx, uid, id)
}

func (r *appRepo) Delete(ctx context.Context, uid, id uuid.UUID) error {
	_, err := r.db.App.Delete().
		Where(app.UserID(uid)).
		Where(app.ID(id)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete app: %w", err)
	}
	return nil
}

func (r *appRepo) EnsureEntitlement(ctx context.Context, uid uuid.UUID) (*db.Entitlement, error) {
	row, err := r.db.Entitlement.Query().Where(entitlement.UserID(uid)).Only(ctx)
	if err != nil && !db.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get entitlement: %w", err)
	}
	if row != nil {
		return row, nil
	}
	if err != nil {
		return r.createEntitlement(ctx, uid)
	}
	return row, nil
}

func (r *appRepo) createEntitlement(ctx context.Context, uid uuid.UUID) (*db.Entitlement, error) {
	row, err := r.db.Entitlement.Create().
		SetUserID(uid).
		SetTier("free").
		SetFreeBuildsUsed(0).
		SetDesktopLicensed(false).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create entitlement: %w", err)
	}
	return row, nil
}
