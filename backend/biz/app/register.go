package app

import (
	"github.com/samber/do"

	v1 "github.com/chaitin/MonkeyCode/backend/biz/app/handler/v1"
	"github.com/chaitin/MonkeyCode/backend/biz/app/repo"
	"github.com/chaitin/MonkeyCode/backend/biz/app/usecase"
)

func ProvideApp(i *do.Injector) {
	do.Provide(i, repo.NewAppRepo)
	do.Provide(i, usecase.NewAppUsecase)
	do.Provide(i, v1.NewAppHandler)
}

func InvokeApp(i *do.Injector) {
	do.MustInvoke[*v1.AppHandler](i)
}
