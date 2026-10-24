package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/db/appdesktopbranding"
)

// GetAppDesktopBranding returns one app's saved desktop identity, or nil until
// the owner has saved any. Defaults come from the app row in the usecase.
func (r *appBuilderRepo) GetAppDesktopBranding(ctx context.Context, appID uuid.UUID) (*domain.AppDesktopBranding, error) {
	row, err := r.db.AppDesktopBranding.Query().
		Where(appdesktopbranding.AppID(appID)).
		First(ctx)
	if err != nil && db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app desktop branding: %w", err)
	}
	return appDesktopBrandingFromRow(row), nil
}

func (r *appBuilderRepo) SaveAppDesktopBranding(ctx context.Context, appID uuid.UUID, req *domain.UpdateAppDesktopBrandingReq) (*domain.AppDesktopBranding, error) {
	create := r.db.AppDesktopBranding.Create().SetAppID(appID)
	applyAppDesktopBrandingCreate(req, create)
	err := create.
		OnConflictColumns(appdesktopbranding.FieldAppID).
		Update(func(u *db.AppDesktopBrandingUpsert) {
			applyAppDesktopBrandingUpsert(req, u)
		}).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("upsert app desktop branding: %w", err)
	}
	return r.GetAppDesktopBranding(ctx, appID)
}

func applyAppDesktopBrandingCreate(req *domain.UpdateAppDesktopBrandingReq, c *db.AppDesktopBrandingCreate) {
	if req == nil {
		return
	}
	if req.ProductName != nil {
		c.SetProductName(*req.ProductName)
	}
	if req.BundleIdentifier != nil {
		c.SetBundleIdentifier(*req.BundleIdentifier)
	}
}

func applyAppDesktopBrandingUpsert(req *domain.UpdateAppDesktopBrandingReq, u *db.AppDesktopBrandingUpsert) {
	if req == nil {
		return
	}
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
	}
	if req.BundleIdentifier != nil {
		u.SetBundleIdentifier(*req.BundleIdentifier)
	}
}

func appDesktopBrandingFromRow(row *db.AppDesktopBranding) *domain.AppDesktopBranding {
	return &domain.AppDesktopBranding{
		ID:               row.ID,
		AppID:            row.AppID,
		ProductName:      row.ProductName,
		BundleIdentifier: row.BundleIdentifier,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
