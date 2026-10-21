# 桌面端打包后端（Desktop Builder Runner）技术设计

Feature Name: app-builder-desktop-runner
Updated: 2026-10-20
Status: draft — 设计权威定义在别处，本文件只做指针

## Description

桌面端打包的技术设计已经完整存在于 `app-builder` 规格内，本文件不重复它的内容。
这里列的是实现时必须打开的章节，避免实现时漏读。

## 设计权威章节索引

需求侧（`.monkeycode/specs/app-builder/requirements.md`）：

- Requirement 1 平台能力与前置检查（第 36 行）
- Requirement 3 桌面品牌配置读写（第 59 行）
- Requirement 6 桌面在线打包（第 97 行）
- Requirement 7 打包槽位互斥（第 111 行）
- Requirement 8 打包机资源约束（第 122 行）
- Requirement 12 桌面升级发布（第 169 行）
- Requirement 16 管理后台打包页面（第 216 行）

设计侧（`.monkeycode/specs/app-builder/design.md`）：

- Components and Interfaces 段（第 99 行）
- Runner 抽象（第 178 行）
- 桌面端流水线 `runner/desktop.go`（第 231 行）
- 迁移 000033 `app_build_artifacts`（第 397 行）
- 迁移 000034 `desktop_branding`（第 425 行）
- 状态机（第 452 行）
- Configuration 段（第 465 行）
- 实测基线段（第 502 行）
- Correctness Properties 段（第 549 行）
- Error Handling 段（第 564 行）
- Test Strategy 段（第 590 行）
- Risks and Open Questions 段（第 618 行）

## 本规格补充的部分

实施拆解、阻塞项盘点、追加的 Correctness Properties 与 Error Handling，
全部写在 `requirements.md` 内，原因是它们描述的是执行顺序而不是系统设计，
放在设计文档里会掩盖上面这些权威章节。

## 实现前必读的三条工程约定

这些约定在 `app-builder/design.md` 里没有显式写成规则，但违反会导致线上事故：

1. **ent schema 改动必须逐列核对 hand-written SQL。**
   `entx.SoftDeleteMixin2` 只贡献 `deleted_at` 一列且不带索引。
   000031 的 `app_branding` 就是这个模式，000033 与 000034 照抄。
   核对方式：`go generate` 后跑 `information_schema.columns` 对比迁移 DDL。

2. **`config.go` 第 477 至 478 行注明：未注册 `SetDefault` 的 key 在
   `Unmarshal` 时被静默忽略。** 新增的 8 项桌面配置必须在装配处补默认值，
   否则线上启动会拿到零值而不是配置值。

3. **runner 的超时必须杀进程组。** `exec.Command` 的 `Context` 只杀直接子进程，
   `npm ci` 与 `cargo` 会留孙进程占磁盘句柄。必须
   `SysProcAttr.Setpgid = true` 加 `syscall.Kill(-pid, SIGKILL)`。

## Correctness Properties

沿用 `app-builder/design.md` 第 549 行的全部条目，另加三条见
`requirements.md` 的 Correctness Properties 段（第 13 至 15 条）。

## Error Handling

沿用 `app-builder/design.md` 第 564 行的全部条目与 11420 至 11424 五项业务码，
另加两条见 `requirements.md` 的 Error Handling 段。

## References

- `.monkeycode/specs/app-builder/design.md` — 全部设计内容
- `.monkeycode/specs/app-builder/requirements.md` — 全部需求内容
- `.monkeycode/specs/app-builder-desktop-runner/requirements.md` — 本规格的拆解与补充
- `.monkeycode/MEMORY.md` — 项目踩坑记录，含构建机磁盘与内存基线
