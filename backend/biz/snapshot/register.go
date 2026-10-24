package snapshot

import (
	"github.com/samber/do"

	v1 "github.com/chaitin/MonkeyCode/backend/biz/snapshot/handler/v1"
	"github.com/chaitin/MonkeyCode/backend/biz/snapshot/repo"
	"github.com/chaitin/MonkeyCode/backend/biz/snapshot/usecase"
)

// ProvideSnapshot 注册会话源码快照模块的仓储、对象存储适配器与处理器。
func ProvideSnapshot(i *do.Injector) {
	do.Provide(i, repo.NewSnapshotRepo)
	do.Provide(i, usecase.NewSnapshotStore)
	do.Provide(i, usecase.NewSnapshotService)
	do.Provide(i, v1.NewSnapshotHandler)
}

// InvokeSnapshot 解析处理器以触发路由注册。
func InvokeSnapshot(i *do.Injector) {
	do.MustInvoke[*v1.SnapshotHandler](i)
}
