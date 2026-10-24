// Package mobilebuild registers the mobile app builder module.
package mobilebuild

import (
	"fmt"
	"path/filepath"

	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/assetstore"
	mbdomain "github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	v1 "github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/handler/v1"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/repo"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/usecase"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/worker"
	"github.com/chaitin/MonkeyCode/backend/config"
	"github.com/chaitin/MonkeyCode/backend/domain"
)

func ProvideMobileBuild(i *do.Injector) {
	// The snapshot archive comes from the snapshot module's store adapter, so
	// read it back out of the container instead of importing that module. The
	// empty adapter is returned when object storage is disabled; overlay-source
	// reports a clear error in that case.
	do.Provide(i, func(i *do.Injector) (mbdomain.ObjectSource, error) {
		if store, err := do.Invoke[domain.SnapshotStore](i); err == nil {
			if src, ok := store.(mbdomain.ObjectSource); ok {
				return src, nil
			}
		}
		return nil, fmt.Errorf("no object store is registered")
	})
	do.Provide(i, func(i *do.Injector) (assetstore.Store, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return assetstore.NewFileStore(cfg.MobileBuild.AssetRoot), nil
	})
	do.Provide(i, func(i *do.Injector) (assetstore.AppStore, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return assetstore.NewAppStore(filepath.Join(cfg.MobileBuild.AssetRoot, "apps")), nil
	})
	do.Provide(i, repo.NewAppBuilderRepo)
	do.Provide(i, usecase.NewUsecase)
	do.Provide(i, usecase.NewAppBuildService)
	do.Provide(i, worker.NewWorker)
	do.Provide(i, v1.NewBuildHandler)
}

func InvokeMobileBuild(i *do.Injector) {
	do.MustInvoke[*worker.Worker](i)
	do.MustInvoke[*v1.BuildHandler](i)
}
