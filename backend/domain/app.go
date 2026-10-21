package domain

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/db"
)

// AppUsecase App 业务逻辑接口
type AppUsecase interface {
	List(ctx context.Context, uid uuid.UUID, req *ListAppsReq) (*ListAppsResp, error)
	Get(ctx context.Context, uid uuid.UUID, id uuid.UUID) (*App, error)
	Create(ctx context.Context, uid uuid.UUID, req *CreateAppReq) (*App, error)
	Update(ctx context.Context, uid uuid.UUID, id uuid.UUID, req *UpdateAppReq) (*App, error)
	Delete(ctx context.Context, uid uuid.UUID, id uuid.UUID) error
	// AdNetworks returns the ad network options defined in packer/ad_networks.yaml.
	AdNetworks(ctx context.Context) ([]AdNetworkOption, error)
}

// AppQuotaService extends AppUsecase with the user-center packaging entry. It is
// declared separately because the interface is only implemented once the app
// builder module is registered.
type AppQuotaService interface {
	AppUsecase
	AppBuildQuotaService
}

// AppRepo App 数据访问接口
type AppRepo interface {
	List(ctx context.Context, uid uuid.UUID) ([]*db.App, error)
	Get(ctx context.Context, uid, id uuid.UUID) (*db.App, error)
	Create(ctx context.Context, uid uuid.UUID, req *CreateAppReq) (*db.App, error)
	Update(ctx context.Context, uid, id uuid.UUID, req *UpdateAppReq) (*db.App, error)
	Delete(ctx context.Context, uid, id uuid.UUID) error
	EnsureEntitlement(ctx context.Context, uid uuid.UUID) (*db.Entitlement, error)
}

// App 跨平台应用信息
type App struct {
	ID                   uuid.UUID  `json:"id"`
	Name                 string     `json:"name"`
	PackageID            string     `json:"package_id"`
	IconKey              string     `json:"icon_key,omitempty"`
	Description          string     `json:"description,omitempty"`
	AdNetwork            string     `json:"ad_network"`
	AdAppID              string     `json:"ad_app_id,omitempty"`
	EnabledPlatforms     []string   `json:"enabled_platforms"`
	BuildCount           int        `json:"build_count"`
	Locked               bool       `json:"locked"`
	Status               string     `json:"status"`
	LastBuiltAt          *time.Time `json:"last_built_at,omitempty"`
	LastBuildPlatform    string     `json:"last_build_platform,omitempty"`
	LastBuildArtifactKey string     `json:"last_build_artifact_key,omitempty"`
	Tier                 string     `json:"tier"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// AdNetworkOption 广告网络选项，来自 packer/ad_networks.yaml。
type AdNetworkOption struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	Vendor      string   `json:"vendor,omitempty"`
	Platforms   []string `json:"platforms,omitempty"`
}

// From 从数据库模型转换为领域模型
func (a *App) From(src *db.App) *App {
	if src == nil {
		return a
	}
	a.ID = src.ID
	a.Name = src.Name
	a.PackageID = src.PackageID
	a.IconKey = src.IconKey
	a.Description = src.Description
	a.AdNetwork = src.AdNetwork
	a.AdAppID = src.AdAppID
	a.EnabledPlatforms = src.EnabledPlatforms
	a.BuildCount = src.BuildCount
	a.Locked = src.Locked
	a.Status = src.Status
	a.LastBuiltAt = src.LastBuiltAt
	a.LastBuildPlatform = src.LastBuildPlatform
	a.LastBuildArtifactKey = src.LastBuildArtifactKey
	a.CreatedAt = src.CreatedAt
	a.UpdatedAt = src.UpdatedAt
	return a
}

// ListAppsReq 查询 App 列表请求
type ListAppsReq struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}

// ListAppsResp 查询 App 列表响应
type ListAppsResp struct {
	Apps []*App `json:"apps"`
	// Tier 与 HostQuota 来自用户的套餐 entitlement，前端据此展示配额。
	Tier      string `json:"tier"`
	HostQuota int    `json:"host_quota"`
}

// CreateAppReq 创建 App 请求
type CreateAppReq struct {
	Name             string   `json:"name" validate:"required"`
	PackageID        string   `json:"package_id" validate:"required"`
	Description      string   `json:"description,omitempty"`
	AdNetwork        string   `json:"ad_network,omitempty"`
	AdAppID          string   `json:"ad_app_id,omitempty"`
	EnabledPlatforms []string `json:"enabled_platforms,omitempty"`
}

// UpdateAppReq 更新 App 请求
type UpdateAppReq struct {
	ID               uuid.UUID `param:"id" validate:"required" json:"-" swaggerignore:"true"`
	Name             *string   `json:"name,omitempty"`
	Description      *string   `json:"description,omitempty"`
	IconKey          *string   `json:"icon_key,omitempty"`
	AdNetwork        *string   `json:"ad_network,omitempty"`
	AdAppID          *string   `json:"ad_app_id,omitempty"`
	EnabledPlatforms []string  `json:"enabled_platforms,omitempty"`
}

// DeleteAppReq 删除 App 请求
type DeleteAppReq struct {
	ID uuid.UUID `param:"id" validate:"required" json:"-" swaggerignore:"true"`
}

// AdNetworksReq 广告网络列表请求，无参数。
type AdNetworksReq struct{}
