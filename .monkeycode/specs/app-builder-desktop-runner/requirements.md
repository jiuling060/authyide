# 桌面端打包后端（Desktop Builder Runner）实施规格

Feature Name: app-builder-desktop-runner
Updated: 2026-10-20
Status: draft — 需求与设计已存在，本文件只承担阻塞项盘点与实施拆解

## Description

桌面端打包的需求与技术设计已经存在于 `app-builder` 规格内，不是新需求。本规格
存在的理由是：该部分完全未开工，且被一个环境级硬阻塞卡住，需要先盘清阻塞项、
排出实施顺序、给每个任务定验收线，才能让后续实现直接照着做。

需求权威来源（勿改写到本文件）：

- `.monkeycode/specs/app-builder/requirements.md`
  Requirement 1 平台能力与前置检查、Requirement 3 桌面品牌配置读写、
  Requirement 6 桌面在线打包、Requirement 7 打包槽位互斥、Requirement 8 打包机
  资源约束、Requirement 12 桌面升级发布、Requirement 16 管理后台打包页面
- `.monkeycode/specs/app-builder/design.md`
  Components and Interfaces 段的 `runner/desktop.go`、Data Models 段的
  `000033_create_app_build_artifacts.sql` 与 `000034_create_desktop_branding.sql`、
  Configuration 段、实测基线段、Correctness Properties 段、Error Handling 段

编号已被占用的资源，本规格不改：

- 迁移号 000033、000034 归桌面 runner 使用
- 业务码 11420-11424 归桌面 runner 使用
- 迁移号 000035 归 `app-self-service-packaging`，000036 归 `app-live-preview`
- 业务码 11425/11426 归 `app-self-service-packaging`，11427/11428 归 `app-live-preview`

## 现状盘点

### 后端

`backend/biz/mobilebuild/` 共 8 个文件 2713 行，`grep -rilE 'desktop|tauri|ohmyagent|sidecar'`
零命中。桌面品牌配置、桌面 runner、产物渠道概念、升级发布链路四项全部不存在。

迁移现状：`app_build_jobs` 表在 000031 重建，`app_branding` 表同批创建。
000032 补 `deleted_at`。当前线上 schema 版本 000032。

### 前端

`frontend/src/pages/console/manager/app-builder.tsx` 共 971 行。平台维度硬编码：
提交时写死 `platform: "android"`（第 392 行），读取时写死
`releases?.native?.find(item.platform === "android")`（第 189 行）。
全文件 `ios` 与 `desktop` 零命中，无平台选择器。

### 构建机工具链

服务器 `38.39.30.160` 缺 Rust 生态全部工具：`cargo`、`rustc`、`go`、`patchelf`、`rpm`
均不存在，本机 `desktop/target/release/` 下无任何 `exe`、`dmg`、`nsis` 产物。

移动端构建链路可用：JDK 17 与 Android SDK 挂载就绪，APK 产物实测 57384873 字节
可下载，`worker` 启动与轮询无异常。

### 硬阻塞：引擎源码不可得

`desktop/` 引用的 `agent` 引擎仓库在当前环境拿不到：

- SSH 方式：host key 校验失败
- HTTPS 方式：仓库不存在或无权访问
- credential helper 输出长度 383 字节，仍返回仓库不存在

该阻塞影响三件事，且无法通过换工具链绕过：

1. `make linux` 链路依赖 `check-ohmy-src`，源码缺失时必然失败
2. sidecar 占位符无法替换为真实二进制，桌面 App 无法启动 agent 功能
3. 桌面 CI 工作流无法产出真实可分发产物

### CI 工作流品牌名不一致

`.github/workflows/desktop-linux.yml` 的 `EXPECTED` 变量第 100 行写死旧品牌名，
与 `tauri.conf.json` 当前 productName 不匹配，产物校验必然红。同类问题在
`desktop-windows.yml` 第 100 行、第 105 行，`desktop-macos.yml` 第 110 行、
第 114 行、第 116 行，`gen-latest-json.py` 第 42、43、49、50、56、57 行，
`macos_release_candidate.py` 第 17 行，合计约 12 处。这些属于打包产物命名层，
与 runner 代码层无耦合，可独立修。

## 实施任务拆解

任务分四组，组内可并行，组间严格串行。

### 组 A：阻塞解除（前置，未完成不得进入组 B）

**A1 取得引擎源码**

动作：确认可访问 `agent` 仓库的凭据来源，落到本地工作区与构建机。

验收：`desktop/` 下 `agent` 目录非空；`make check-ohmy-src` 退出码为 0；
`desktop/make linux` 能走到 `cargo build` 阶段（不要求产物，要求能进入编译）。

**A2 构建机补齐 Rust 工具链**

动作：在构建机安装 `rustup`（含 stable 工具链）、`go`、`patchelf`、`rpm` 构建依赖，
写入部署脚本而非手工安装。

验收：`cargo --version`、`rustc --version`、`go version`、`patchelf --version`、
`rpm --version` 五条命令在构建机均返回 0；`rustup target list --installed` 包含
构建目标三元组。

**A3 修正 CI 工作流品牌名**

动作：把约 12 处硬编码旧品牌名改为从 `tauri.conf.json` 的 productName 读取。

验收：三个平台的 `EXPECTED` 与实际产物文件名一致；`gen-latest-json.py` 生成的
清单 JSON 中 `productName` 字段与配置一致。

### 组 B：数据层与 runner 抽象（依赖组 A 完成 A1）

**B1 迁移 000033 与 000034**

动作：按 `app-builder/design.md` 第 397 行与第 425 行的 DDL 建两个迁移，
`app_build_artifacts` 表加 `UNIQUE(job_id, channel)`，
`desktop_branding` 表加单例索引。

验收：PostgreSQL 18.6 上 up 后 down，表结构与列顺序复原；
`UNIQUE(job_id, channel)` 重复插入第二条报错；单例索引第二次插入报错。

**B2 ent schema 与 go generate**

动作：新增 `AppBuildArtifact` 与 `DesktopBranding` 两个 schema，
`AppBuildJob` 补 `channel` 关联。

验收：`go generate ./...` 成功；改完 schema 后逐列核对 `information_schema.columns`，
`deleted_at` 是 `entx.SoftDeleteMixin2` 唯一贡献的列且无索引，与 000031 的
`app_branding` 同一模式。

**B3 仓储层**

动作：实现 `repo/artifact.go` 与 `repo/desktop_branding.go`，
参照 `repo/branding.go` 的单例读写模式。

验收：仓储层单测覆盖单例读写、产物按 `(job_id, channel)` 唯一写入、
软删除后不再返回。

**B4 `BuildRunner` 抽象拆分**

动作：把 `runner.go` 现有移动端逻辑抽成接口，拆出 `runner.go`（公共层）、
`android.go`、`desktop.go`、`registry.go`。

验收：`grep` 确认 `runner.go` 内无 Android 专有命令；
接口方法覆盖 `Validate`、`Prepare`、`Build`、`Collect`、`Cleanup`；
`registry.go` 按平台字符串分发；现有移动端行为零回归，
APK 产物仍可下载且 sha256 与改动前一致。

**B5 桌面 runner**

动作：实现 `desktop.go` 六步流水线，按 `app-builder/design.md` 第 231 行的定义。
进程组隔离沿用现有实现：`SysProcAttr.Setpgid = true` 加
`syscall.Kill(-pid, SIGKILL)`，`exec.Command` 的 `Context` 只杀直接子进程，
`npm ci` 与 `cargo` 会留孙进程，必须杀整个进程组。

验收：构建机磁盘低于阈值时按 Requirement 8 拒绝任务并返回
`ErrResourceInsufficient` 11424；构建超时后 `/proc` 下无残留孙进程；
产物目录布局与 `app-builder/design.md` 的产物段一致。

### 组 C：API 与前端（依赖组 B）

**C1 配置与错误码**

动作：`config.MobileBuildConfig` 新增 8 项桌面字段，装配处补 `SetDefault`。
`errcode` 新增 11420-11424 五项。

验收：缺省启动不报错，8 个新 key 均有默认值；
`config.go` 第 477 至 478 行已注明未注册 `SetDefault` 的 key 在 `Unmarshal`
时被静默忽略，所以每个新 key 都要显式补默认值。

**C2 路由**

动作：把 `/api/v1/admin/app-builder` 下的 13 条路由扩到 16 条，
新增桌面品牌读写与产物渠道查询。所有新路由沿用 `requireAdmin()` 校验。

验收：新路由在鉴权中间件之外仍能挡住无管理员角色的请求；
返回体字段与 Requirement 3 与 Requirement 6 的验收标准逐条对应。

**C3 前端平台维度**

动作：把 `app-builder.tsx` 的硬编码平台改为状态，加平台选择器与前置检查面板，
按 Requirement 1 显示每个平台是否可用及原因。

验收：切换平台后提交体的 `platform` 字段正确；
桌面平台在工具链缺失时显示不可用原因而不是报错崩溃；
`tsc -b` 通过，`node --test test/*.test.ts` 通过。

**C4 国际化与页面接线**

动作：新增文案，同步 `App.tsx`、`nav-teams.tsx`、`Page.tsx` 三处接线。

验收：`test/i18n-resources-completeness.test.ts` 通过，en 与 cn 叶子键一致；
`npm run build:offline` 产出成功。

### 组 D：端到端验收（依赖组 C）

**D1 构建链路**

验收：从管理后台提交桌面打包任务，任务进入 `queued` 后 `running`，
最终 `succeeded`；产物按渠道可下载；`worker` 日志无 panic；
`build poll failed` 计数为 0。

**D2 发布链路**

验收：桌面升级发布按 Requirement 12 生成更新清单；
渠道为 `deb` 与 `rpm` 时拒绝发布并返回 `ErrChannelNotPublishable` 11423；
`/builds/:id/download` 缺省 `channel` 时按平台推导主产物，
非 `desktop-linux` 平台强制显式指定。

**D3 多产物一致性**

验收：同一任务下 `app_build_artifacts` 行数等于该次构建产出的渠道数；
既有移动端任务同步写入一条 `channel = apk` 记录，下载与发布行为与改动前一致。

## Correctness Properties

沿用 `app-builder/design.md` 第 549 行的 Correctness Properties 段，
本规格追加三条：

13. `app_build_artifacts.channel` 在同一 `job_id` 下唯一，不允许同渠道二次写入。
14. `desktop_branding` 表在未软删除状态下至多一行。
15. `desktop-linux` 平台的构建任务在工具链前置检查失败时不得进入 `queued`。

## Error Handling

沿用 `app-builder/design.md` 第 564 行的 Error Handling 段与 11420-11424 五项业务码，
本规格追加一项：

| 场景 | 业务码 |
| --- | --- |
| 引擎源码缺失导致 `check-ohmy-src` 失败 | `ErrPlatformDisabled` 11420 |
| 构建机 Rust 工具链缺失 | `ErrPlatformDisabled` 11420 |

两者都返回平台不可用原因，不返回构建失败，让前端能显示原因而不是崩溃。

## Test Strategy

沿用 `app-builder/design.md` 第 590 行的 Test Strategy 段，本规格追加：

1. 组 A 的 A1 与 A2 各自有独立验收命令，任一失败阻断组 B 进入。
2. 组 B 的 B4 抽象拆分必须有移动端回归测试：改动前后 APK 的 sha256 一致。
3. 组 B 的 B5 必须有超时路径测试：验证杀的是进程组，不是直接子进程。
4. 组 C 的 C3 必须有平台切换测试：验证提交体的 `platform` 字段。
5. 组 D 的 D2 必须验证 `channel` 缺省推导与显式指定两条路径。

## Risks and Open Questions

沿用 `app-builder/design.md` 第 618 行的 Risks and Open Questions 段，本规格追加：

1. **引擎源码访问权限**（高）：当前环境无法取得 `agent` 仓库，
   组 A 的 A1 是唯一入口。该阻塞不解除，后续 15 个任务全部无法进入。
2. **构建机内存**（中）：服务器 15.6G 内存无 swap，
   `cargo` 全量构建与 `gradlew` 并发会 OOM。桌面 runner 与打包 worker 已设计为
   各自单协程串行，但仍需限制 `cargo` 的并行度，按 `app-builder/design.md`
   实测基线段的记录设定。
3. **CI 品牌名修正时机**（低）：约 12 处硬编码品牌名与 runner 无耦合，
   可在组 A 期间独立修，不影响后续任务。
4. **sidecar 占位符残留**（中）：组 A 的 A1 完成前，任何构建出的桌面产物
   都含占位符，不可分发。必须在该任务完成前禁止把桌面产物发布到线上清单。

## References

- `.monkeycode/specs/app-builder/requirements.md` — 桌面需求权威定义
- `.monkeycode/specs/app-builder/design.md` — 桌面设计权威定义，含实测基线与既有风险
- `.monkeycode/specs/app-self-service-packaging/design.md` — 迁移号 000035 与业务码 11425/11426 占用方
- `.monkeycode/specs/app-live-preview/design.md` — 迁移号 000036 与业务码 11427/11428 占用方
- `backend/biz/mobilebuild/` — 2713 行现状代码，桌面部分零实现
- `backend/migration/000031_create_app_builder.up.sql` — 单例索引与迁移风格参照
- `backend/migration/000032_add_app_build_jobs_deleted_at.up.sql` — 当前线上最新迁移
- `frontend/src/pages/console/manager/app-builder.tsx` — 前端平台硬编码位置
- `.github/workflows/desktop-linux.yml`、`desktop-windows.yml`、`desktop-macos.yml` — CI 品牌名修正位置
