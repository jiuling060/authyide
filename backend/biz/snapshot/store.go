// Package snapshot 实现会话源码快照：把某个会话此刻的工作区打成一份归档存入
// 对象存储，供该用户的 App 打包时叠加到模板工作区之上。
//
// 模块分三层：repo 访问 app_source_snapshots 表，usecase 负责归属校验、
// 摘要校验与状态流转，handler/v1 暴露 /api/v1/users/snapshots 路由。
// 对象存储适配器在 usecase/store.go，把 *pkg/oss.Client 适配成
// domain.SnapshotStore，避免 domain 反向依赖 pkg/oss。
package snapshot
