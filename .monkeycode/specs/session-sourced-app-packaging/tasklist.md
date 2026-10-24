# 会话源码打包 实施计划

Feature Name: session-sourced-app-packaging
Updated: 2026-10-22

- [ ] 1. 持久化层：ent schema 与迁移 000036
  - [ ] 1.1 新增 `backend/ent/schema/appsourcesnapshot.go`
    - `AppSourceSnapshot` 结构体 + `entsql.Table("app_source_snapshots")`
    - 字段：`id`(UUID, Default uuid.New)、`user_id`(UUID)、`task_id`(UUID)、
      `object_key`(String, NotEmpty)、`sha256`(String, NotEmpty)、
      `size_bytes`(Int64)、`file_count`(Int)、`status`(String, Default "uploading")、
      `superseded`(Bool, Default false)、`error`(Text, Optional)、
      `created_at`(Time, Default time.Now)、`completed_at`(Time, Optional, Nillable)
    - `Edges`：`edge.From("user", User.Type).Ref(...).Field("user_id").Unique().Required()`，
      与 `ent/schema/app.go` 的写法一致
    - `Indexes`：`(user_id, created_at)` 与 `(task_id)`，对应设计要求 1
    - 参照 `ent/schema/app.go` 的注释风格，在字段旁说明 `superseded` 是审计标记
  - [ ] 1.2 修改 `backend/ent/schema/app.go`
    - `App.Fields()` 追加 `field.UUID("source_snapshot_id", uuid.UUID{}).Optional().Nillable()`
    - 注释说明空值表示该 App 打包走模板工作区（对应需求 R2 AC3、R3 AC3）
  - [ ] 1.3 修改 `backend/ent/schema/appbuildjob.go`
    - 追加 `source_snapshot_id` 与 `source_task_id`，均 `Optional().Nillable()`
    - 注释说明入队时一次性写入、之后不可变（对应需求 R3 AC1）
  - [ ] 1.4 编写 `backend/migration/000036_create_app_source_snapshots.{up,down}.sql`
    - up：`CREATE TABLE IF NOT EXISTS app_source_snapshots` + 两个索引 + 三个
      `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`
    - 开头写注释说明为何需要该表，风格对齐
      `000035_add_app_build_jobs_app_id.up.sql`
    - down：`DROP COLUMN` 三列，`DROP TABLE IF EXISTS app_source_snapshots`
  - [ ] 1.5 执行 `go generate ./ent` 并校验生成结果
    - 生成前 `cp -r backend/db /tmp/opencode/db-backup`
    - 生成后 `diff -rq` 确认只影响 `db/app/`、`db/appbuildjob/`、
      `db/appsourcesnapshot/` 相关生成文件
    - 通过 background terminal 执行并设置 `memory_percent`，峰值约 2.9 GiB

- [ ] 2. 业务码与 domain 类型
  - [ ] 2.1 在 `backend/errcode/errcode.go` 注册 6 个业务码
    - 接在既有 `114xx` App 打包段之后，避开 11420–11424 已用区间：
      11430 `ErrSnapshotEmpty`、11431 `ErrSnapshotSyncFailed`、
      11432 `ErrSnapshotTooLarge`、11433 `ErrSnapshotNotFound`、
      11434 `ErrSnapshotStale`、11435 `ErrSnapshotCorrupt`
  - [ ] 2.2 新增 `backend/domain/snapshot.go`
    - `Snapshot` 结构体含 `Stale bool` 派生字段，不落库
    - `PresignSnapshotReq`（`TaskID`）、`CompleteSnapshotReq`
      （`SHA256` / `SizeBytes` / `FileCount`）、`FailSnapshotReq`（`Error`）
    - `SnapshotService` 接口六方法，与 design.md 的接口签名逐字一致
  - [ ] 2.3 扩展 `backend/biz/mobilebuild/domain/appbuilder.go`
    - `EnqueueBuildReq`（第 127 行）追加 `SourceSnapshotID` / `SourceTaskID`
    - `BuildJob` 追加同名列，并确认 `BuildJobFromDB` 类映射函数同步补字段

- [ ] 3. 快照仓储层
  - [ ] 3.1 新增 `backend/biz/snapshot/repo/snapshot.go`
    - `NewAppSourceSnapshotRepo`，返回唯一类型名，避免 DI 服务名冲突
    - 实现建 `uploading` 行、按 ID 查询、置 `failed`
  - [ ] 3.2 实现 Complete 的事务方法
    - 单事务内完成「置 `ready` + 写 `completed_at`」与「同会话其余
      `ready` 快照置 `superseded=true`」两步（设计要求 4）
    - 事务失败整体回滚，不留下两份 current 快照
  - [ ] 3.3 实现查询方法
    - `CurrentByTask`：该会话最新一份 `ready` 且 `superseded=false`
    - `ListMine`：分页，只返回 `ready`
    - `Get`：按 ID + user_id 双条件，命中失败统一返回空结果由上层转业务码
  - [ ] 3.4 为仓储层写单元测试
    - 覆盖 Create/MarkFailed/MarkReadyWithSupersede/ListMine/GetCurrentByTask/Get
    - 事务原子性用并发用例验证：并发两个 Complete 后 current 快照数仍为 1

- [ ] 4. 快照 service 与 handler
  - [ ] 4.1 实现 `Presign`
    - 校验会话归属与该会话可构建状态，越权返回会话不存在业务码
    - 校验工作区非空，空则返回 11430（需求 R1 AC3）
    - 校验打包字节数不超 `ObjectStorage.MaxSize`，超限返回 11432（R1 AC5）
    - 对象键 `RepoPrefix/snapshot/{user_id}/{snapshot_id}.tar.gz`，不改
      `biz/uploader`（设计要求 2）
    - presigned URL 有效期读 `ObjectStorage.PresignExpires`
  - [ ] 4.2 实现 `Complete`
    - 从对象存储读取已上传对象，重算 sha256 与字节数比对上报值
    - 不一致则置 `failed`、`error` 记录期望与实际摘要，返回 11431
    - 一致则调用 3.2 的事务方法
  - [ ] 4.3 实现 `Fail`
    - 只允许 `uploading` → `failed`，写 `error` 字段
  - [ ] 4.4 实现过期派生
    - `Stale = tasks.last_active_at > snapshots.completed_at`，在 service 层
      join `tasks` 计算，SQL 内不写时间比较
  - [ ] 4.5 新增 `backend/biz/snapshot/handler/v1/snapshot.go`
    - 结构体命名 `SnapshotHandler`，禁止用 `Handler`（DI 服务名冲突）
    - 路由：`POST /api/v1/users/snapshots/presign`、
      `POST /api/v1/users/snapshots/{id}/complete`、
      `POST /api/v1/users/snapshots/{id}/fail`、
      `GET /api/v1/users/snapshots`、`GET /api/v1/users/snapshots/{id}`、
      `GET /api/v1/users/tasks/{id}/snapshot`
    - 归属校验全在 service 层，越权统一返回 11433，不暴露快照是否存在
  - [ ] 4.6 新增 `backend/biz/snapshot/register.go`
    - `ProvideSnapshot` 注入 repo/service/handler，`InvokeSnapshot` 用 `MustInvoke`
    - 在 `bridge.go` 的模块装配链中挂入，参照 `biz/app/register.go` 的位置
  - [ ] 4.7 为 Presign 的拒绝路径写单元测试
    - 会话不存在、会话不属于该用户、工作区为空、超大小上限、对象存储未启用
    - 断言均不产生快照行

- [x] 5. App 绑定快照
  - [x] 5.1 扩展 `backend/biz/app/repo/` 的 App 仓储
    - 追加设置 `source_snapshot_id` 与清除该方法，不改 `apps` 主表结构
    - 解绑只写 `apps.source_snapshot_id`，不触碰快照表（设计要求 5）
  - [x] 5.2 扩展 App usecase
    - 设置前调 `SnapshotService.Get` 校验快照归属与 `status='ready'`
    - App 处于 `locked` 时拒绝修改，返回 `ErrAppLocked`（需求 R2 AC4）
    - 读取 App 时附带快照主键、来源会话标题、sha256、创建时间、过期状态
      （需求 R2 AC5）
    - 未绑定时返回空主键（需求 R2 AC6）
  - [x] 5.3 新增绑定路由
    - `PUT /api/v1/users/apps/{id}/source`，body 为空表示解绑

- [x] 6. 打包链接入
  - [x] 6.1 扩展 `EnqueueBuild` 的取数与拒绝路径
    - 读 App 后完成既有锁定与配额判定，位置不变（需求 R6）
    - 绑定时调 `SnapshotService.Get` 校验归属与就绪，失败返回 11433，
      不创建任务
    - 判定过期，过期返回 11434，不创建任务（需求 R4 AC5）
    - 通过校验后把快照主键与来源会话主键写入任务行（需求 R3 AC1）
    - 未绑定时零值透传，行为与既有规格一致（需求 R3 AC3）
  - [x] 6.2 扩展 `runner.Config` 增加 `SourceSnapshot`
    - 字段 `ObjectKey` / `SHA256` / `SizeBytes`
  - [x] 6.3 扩展 `worker.runnerConfig`（`worker.go:257`）
    - 任务行带 `source_snapshot_id` 时填充 `SourceSnapshot`
    - 为空则保持零值，步骤整体跳过并报告 skipped
  - [x] 6.4 实现 `overlay-source` 的下载与摘要校验
    - 从对象存储下载到工作区副本内临时文件
    - sha256 与 `SourceSnapshot.SHA256` 不符则中止，任务置 `failed`，
      业务码 11435（需求 R3 AC6）
  - [x] 6.5 实现 `overlay-source` 的路径清洗与写入
    - 条目名为空、绝对路径、`filepath.Clean` 后含 `..` → 中止
    - 条目类型非 regular file 与目录（symlink、设备文件、socket、FIFO）→ 中止
    - 单文件解压后字节数超 `OverlayMaxBytes = 512 << 20` → 中止
    - 条目数超 `OverlayMaxFiles = 20000` → 中止
    - 两阶段遍历：先全量校验再写入，避免校验到一半已落盘
    - 同名覆盖、目录合并、父目录不存在则 `MkdirAll`（需求 R3 AC2）
  - [x] 6.6 把 `overlay-source` 插在 `inject-assets` 之前
    - 最终顺序：复制模板 → overlay-source → inject-assets → rewrite-config →
      prebuild → assemble → verify（需求 R3 AC4）
    - 顺序错会导致会话的 `app.json` 与 `assets/icon.png` 压过 App 记录
  - [x] 6.7 统一中止路径的 cleanup
    - 任一中止先删除工作区副本再返回错误
    - 覆盖后缺 `app.json` 或 `android/gradlew` 的任务置 `failed`（R3 AC7）
    - 任务结束时删除工作区副本，不留残留（R3 AC8）
  - [x] 6.8 `overlay-source` 单元测试（`runner/overlay_test.go`）
    - 覆盖语义：同名覆盖、目录合并、模板独有保留、`./` 前缀
    - 拒绝路径：`..`、绝对路径、嵌套 `..`、symlink、hardlink、空条目名、
      声明字节超上限、条目数超上限、空归档
    - 每个拒绝用例断言工作区副本在失败前后完全一致（两阶段校验的原子性）
    - `safeJoin` 表驱动用例

- [x] 7. 检查点 — 后端编译与单测通过
  - 确保所有测试通过,如有疑问请询问用户
  - 验证命令见 design.md 的 Test Strategy 章节

- [x] 8. 前端：会话侧同步入口
  - [x] 8.1 扩展 `frontend/src/api/Api.ts`
    - presign / complete / fail / list / get / task snapshot 六个方法
  - [x] 8.2 实现会话打包入口面板
    - 「同步源码」按钮，依次调用 presign → 会话执行器打包上传 → complete
    - 展示当前快照：会话标题、创建时间、文件数、sha256 前 12 位
    - 该会话无快照时展示未同步状态（需求 R4 AC1）
    - 同步中展示进行状态，完成后刷新快照信息（需求 R4 AC2）
    - 失败展示原因并提供重试入口（需求 R4 AC3）
    - 过期时按钮高亮，文案提示快照已落后于会话最近活动（需求 R4 AC4）
    - 列出已绑定该会话快照的 App，过期时提示（需求 R4 AC6）
  - [x] 8.3 补充 `session.snapshot.*` 国际化键
    - `frontend/src/i18n/resources/en.ts` 与 `cn.ts` 同步新增
    - 用 `welcomeShell: {` 作为安全锚点定位插入位置，插入前 `grep -n`
      确认上下文唯一
    - 叶子键集合两份必须完全一致
    - 文案不使用表情符号

- [x] 9. 前端：App 编辑页源码来源区块
  - [x] 9.1 在 `frontend/src/components/console/apps/app-editor.tsx` 新增区块
    - 与既有「素材槽位」区块同级
    - 未绑定显示「使用模板工作区」与「选择快照」按钮，弹窗列出 ready 快照，
      支持按会话标题筛选
    - 已绑定显示会话标题、创建时间、文件数、sha256 前 12 位与过期标记，
      提供「解绑」按钮
  - [x] 9.2 提交态联动
    - `app.locked` 时禁用快照绑定控件，复用既有锁定提示文案（需求 R7 AC4）
    - 快照过期时禁用提交入口，提示先在会话内同步（需求 R7 AC2）
    - 任务详情展示本次构建所依据的快照主键与来源会话标题（需求 R7 AC3）
  - [x] 9.3 补充 `consoleApps.editor.sourceSnapshot.*` 国际化键
    - 两份资源叶子键一致，跑完整性测试验证

- [x] 10. 检查点 — 前端类型检查与文案完整性通过
  - 确保所有测试通过,如有疑问请询问用户
  - `npx tsc -b` 必须 0 错误；`node --test test/i18n-resources-completeness.test.ts`
    必须通过；`npm run build` 必须成功

- [ ] 11. 正确性属性验证
  - [x] 11.1 属性 1 品牌优先
    - `runner_test.go` / `overlay_test.go` 的 `TestOverlayThenRewriteKeepsAppRecordAsBrandAuthority`：
      先 `overlaySource` 再 `rewriteConfig`，断言 `expo.name` / `slug` / `version` /
      `android.package` / `android.versionCode` / `extra.baseUrl` 全部取 App 记录值，
      会话自带的 `app.json` 品牌字段被覆盖
    - 会话内的 `assets/icon.png` 走 `injectAppIcon`，其写入顺序在 `overlaySource` 之后，
      与 `app.json` 同一优先序；该顺序由 `Run()` 的步骤序列保证
  - [x] 11.2 属性 2 模板独有文件存活
    - `TestExtractSnapshotMergesOntoTemplate` 断言覆盖不删除模板文件
    - `TestOverlayThenRewriteKeepsAppRecordAsBrandAuthority` 额外断言
      `android/gradlew` 与 `plugins/withAlipayLogin/index.ts` 在覆盖后仍存在
  - [ ] 11.3 属性 3 快照不可变
    - 快照置 `ready` 后 `sha256` 与 `size_bytes` 不被任何写入路径修改
  - [ ] 11.4 属性 4 同会话单 current
    - 任一时刻每个会话最多一份 `superseded=false` 的 `ready` 快照
  - [x] 11.5 属性 5 覆盖不可逃逸
    - `TestExtractSnapshotRejects{EscapingPath,AbsolutePath,NestedDotDot,Symlink,HardLink,EmptyName,OversizedEntry,TooManyEntries,EmptyArchive}`
      与 `TestSafeJoin` 覆盖路径穿越、链接与体积/条目上限
    - `TestOverlaySourceFailsClosedOnDigestMismatch` 断言摘要不符时在**写文件之前**
      中止，工作区副本与覆盖前逐字节一致
  - [ ] 11.6 属性 6 任务工作区独立
    - 一次任务的覆盖结果不影响到下一次任务的模板副本
  - [ ] 11.7 属性 7 归属隔离
    - 用户 A 无法读取、绑定或触发打包用户 B 的快照，越权响应与资源不存在一致

- [ ] 12. 端到端集成验证
  - [ ] 12.1 快照全链路
    - presign → PUT → complete，用既有测试对象存储夹具
  - [ ] 12.2 绑定到产物
    - 绑定快照 → 提交打包 → worker 消费 → 产物存在，断言
      `app_build_jobs.source_snapshot_id` / `source_task_id` 已写入
  - [ ] 12.3 配额穿透
    - 免费档 App 用快照打包成功后 `build_count=1` 且 `locked=true`
      （需求 R6 AC2、AC3）
  - [ ] 12.4 配额不穿透
    - 快照打包失败后 `build_count` 与 `free_builds_used` 保持不变
      （需求 R6 AC4）
