package repo

import (
	"context"
	"fmt"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/db/desktopbranding"
)

const desktopBrandingSingletonKey = "authyide"

// GetDesktopBranding returns the saved desktop branding, or nil until the
// admin has saved anything. Defaults come from tauri.conf.json in the usecase.
func (r *appBuilderRepo) GetDesktopBranding(ctx context.Context) (*domain.DesktopBranding, error) {
	row, err := r.db.DesktopBranding.Query().First(ctx)
	if err != nil && db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get desktop branding: %w", err)
	}
	return desktopBrandingFromRow(row), nil
}

func (r *appBuilderRepo) SaveDesktopBranding(ctx context.Context, req *domain.UpdateDesktopBrandingReq) (*domain.DesktopBranding, error) {
	create := r.db.DesktopBranding.Create().SetSingletonKey(desktopBrandingSingletonKey)
	applyDesktopBrandingCreate(req, create)
	err := create.
		OnConflictColumns(desktopbranding.FieldSingletonKey).
		Update(func(u *db.DesktopBrandingUpsert) {
			applyDesktopBrandingUpsert(req, u)
		}).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("upsert desktop branding: %w", err)
	}
	return r.GetDesktopBranding(ctx)
}

func (r *appBuilderRepo) SetDesktopIconKey(ctx context.Context, key string) (*domain.DesktopBranding, error) {
	create := r.db.DesktopBranding.Create().
		SetSingletonKey(desktopBrandingSingletonKey).
		SetIconKey(key)
	err := create.
		OnConflictColumns(desktopbranding.FieldSingletonKey).
		Update(func(u *db.DesktopBrandingUpsert) { u.SetIconKey(key) }).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("upsert desktop branding icon: %w", err)
	}
	return r.GetDesktopBranding(ctx)
}

// ClearDesktopIconKey reverts to the workspace template icon.
func (r *appBuilderRepo) ClearDesktopIconKey(ctx context.Context) (*domain.DesktopBranding, error) {
	return r.SetDesktopIconKey(ctx, "")
}

func applyDesktopBrandingCreate(req *domain.UpdateDesktopBrandingReq, c *db.DesktopBrandingCreate) {
	if req.ProductName != nil {
		c.SetProductName(*req.ProductName)
	}
	if req.BundleIdentifier != nil {
		c.SetBundleIdentifier(*req.BundleIdentifier)
	}
	if req.UpdaterEndpoint != nil {
		c.SetUpdaterEndpoint(*req.UpdaterEndpoint)
	}
}

func applyDesktopBrandingUpsert(req *domain.UpdateDesktopBrandingReq, u *db.DesktopBrandingUpsert) {
	if req.ProductName != nil {
		u.SetProductName(*req.ProductName)
	}
	if req.BundleIdentifier != nil {
		u.SetBundleIdentifier(*req.BundleIdentifier)
	}
	if req.UpdaterEndpoint != nil {
		u.SetUpdaterEndpoint(*req.UpdaterEndpoint)
	}
}

func desktopBrandingFromRow(row *db.DesktopBranding) *domain.DesktopBranding {
	return &domain.DesktopBranding{
		ID:               row.ID,
		ProductName:      row.ProductName,
		BundleIdentifier: row.BundleIdentifier,
		UpdaterEndpoint:  row.UpdaterEndpoint,
		IconKey:          row.IconKey,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
