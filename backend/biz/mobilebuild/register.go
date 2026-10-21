// Package mobilebuild registers the mobile app builder module.
package mobilebuild

import (
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/assetstore"
	v1 "github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/handler/v1"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/repo"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/usecase"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/worker"
	"github.com/chaitin/MonkeyCode/backend/config"
)

func ProvideMobileBuild(i *do.Injector) {
	do.Provide(i, func(i *do.Injector) (assetstore.Store, error) {
		cfg := do.MustInvoke[*config.Config](i)
		return assetstore.NewFileStore(cfg.MobileBuild.AssetRoot), nil
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
