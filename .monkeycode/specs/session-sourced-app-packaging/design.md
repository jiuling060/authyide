# 会话源码打包（Session-Sourced App Packaging）技术设计

Feature Name: session-sourced-app-packaging
Updated: 2026-10-22
Status: draft

## Description

在既有用户自助打包链（`app-self-service-packaging`）上增加一个可选的源码输入源：
会话工作区。

- 会话内新增「同步源码」动作，产出对象存储上的一份不可变快照，落到新表
  `app_source_snapshots`。
- `apps` 增加 `source_snapshot_id`，指向一份快照；`app_build_jobs` 增加同名列做
  产物溯源。
- 打包机在模板工作区副本上插入 `overlay-source` 步骤，用快照递归覆盖模板，随后
  既有步骤原样执行。
- 记账完全沿用 `apps` 与 `entitlements`，不新增任务表、不新增积分通道。

## Architecture

### 现状与目标

现状：`worker.workspaceFor` 每次 `cp -al` 一份全局模板 `/opt/authyide/src/mobile`，
`runner.Config` 只携带品牌、素材、版本号、图标键，没有任何文件注入入口。
`EnqueueBuildReq` 只有 `platform` / `version` / `version_code` / `app_id`。

目标：`EnqueueBuildReq` 增加 `source_snapshot_id` / `source_task_id`；
`runner.Config` 增加 `SourceSnapshot`；`overlay-source` 步骤插在 `inject-assets`
之前，保证 App 记录的品牌素材与配置字段始终压过会话携带的同名文件。

### 步骤顺序

```mermaid
graph LR
    A["复制模板工作区副本"] --> B["overlay-source（仅当绑定快照）"]
    B --> C["inject-assets"]
    C --> D["rewrite-config"]
    D --> E["expo prebuild"]
    E --> F["assembleRelease"]
    F --> G["verify"]
    G --> H["清理工作区副本"]
```

`overlay-source` 排在 `inject-assets` 之前是刻意选择：会话若自带
`assets/icon.png` 或 `app.json`，由 App 实体接管品牌与版本，见需求 R3 AC4。

桌面端顺序为 `ui-next` → `engine` → `browser-extension` → `shell` →
`overlay-source` → `bundle` → `verify`，理由见下方「桌面端接入」。

### 端到端流程

1. 用户在前端某会话内点击「同步源码」。
2. 前端 `POST /api/v1/users/snapshots/presign`，携带会话主键。
   后端校验会话归属与可构建状态，生成快照主键并落 `app_source_snapshots`
   行（`status='uploading'`），签发 `RepoPrefix` 下的 presigned PUT URL。
3. 前端把 presigned URL 交给会话侧执行器，会话在 VM 内 `tar -czf` 打包
   工作区后 `PUT` 到对象存储。
4. 会话侧执行器 `POST /api/v1/users/snapshots/{id}/complete`，上报
   sha256 / size_bytes / file_count。
5. 后端重新计算对象 sha256 与字节数比对，一致则置 `status='ready'`、写
   `completed_at`，并将同会话更早的快照置 `superseded=true`。
6. 用户在 App 编辑页把快照绑定到 App（`PUT /api/v1/users/apps/{id}/source`）。
7. 用户提交打包：`EnqueueBuildReq.SourceSnapshotID` 取自 App 的绑定值，
   worker 在 `overlay-source` 步骤从对象存储下载该 tar.gz 解压到工作区副本。
8. 任务结束时工作区副本删除，快照对象永久保留。

选择 presigned PUT 而非后端中转：`uploader.Upload` 用 `io.ReadAll` 把整个文件
读进内存，不适合快照体量的 tar.gz；presigned PUT 让 VM 直传对象存储，
后端只登记元数据，与既有附件链的签名模型一致。

## Components and Interfaces

### 迁移 000036 与 ent schema

持久化有两层，必须同时改：

- `backend/migration/000036_create_app_source_snapshots.{up,down}.sql`，沿用
  `000035_add_app_build_jobs_app_id` 的幂等风格，服务于真实数据库。
- `backend/ent/schema/` 新增 `appsourcesnapshot.go`，并在既有 `app.go` 的
  `App.Fields()` 追加 `source_snapshot_id`、在 `appbuildjob.go` 追加
  `source_snapshot_id` / `source_task_id`。改完必须跑
  `go generate ./ent` 重新生成 `backend/db/`，否则生成的 ent 类型没有对应字段
  常量，业务代码编译不过。

生成前先用 `cp -r db /tmp/opencode/db-backup` 备份，生成后用
`diff -rq` 确认只影响目标类型的生成文件。

`migration/000036.up.sql` 内容：

```sql
CREATE TABLE IF NOT EXISTS app_source_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL REFERENCES users(id),
    task_id uuid NOT NULL REFERENCES tasks(id),
    object_key text NOT NULL,
    sha256 character varying(64) NOT NULL,
    size_bytes bigint NOT NULL,
    file_count integer NOT NULL,
    status character varying(32) NOT NULL DEFAULT 'uploading',
    superseded boolean NOT NULL DEFAULT false,
    error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone
);

CREATE INDEX IF NOT EXISTS idx_app_source_snapshots_user_id_created_at
    ON app_source_snapshots USING btree (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_source_snapshots_task_id
    ON app_source_snapshots USING btree (task_id);

ALTER TABLE apps ADD COLUMN IF NOT EXISTS source_snapshot_id uuid;
ALTER TABLE app_build_jobs ADD COLUMN IF NOT EXISTS source_snapshot_id uuid;
ALTER TABLE app_build_jobs ADD COLUMN IF NOT EXISTS source_task_id uuid;
```

三个新增列全部可空：空值表示该 App 走模板工作区，与 `app_build_jobs.app_id`
为管理端打包保留空值的手段一致。

down 迁移反向执行：`DROP COLUMN` 三列，`DROP TABLE app_source_snapshots`。

### 上传通道：复用 RepoPrefix

`pkg/oss` 与 `pkg/asseturl` 的允许前缀白名单已经包含 `ObjectStorage.RepoPrefix`
（`pkg/oss.go:313`、`pkg/asseturl.go:44`），会话侧本就能从该前缀取回自己上传的
仓库对象。快照与仓库属同一类对象（会话产生的源码包），因此直接落在 `RepoPrefix`
下，不新增前缀、不改 compose、不改白名单。

对象键规则：`RepoPrefix/snapshot/{user_id}/{snapshot_id}.tar.gz`。

大小上限读取既有 `ObjectStorage.MaxSize`，与 `uploader`、技能包、扩展包共用同一
数值（`biz/uploader/handler/http/v1/uploader.go:82`）。超限返回快照超限业务码。
presigned URL 过期时间复用 `ObjectStorage.PresignExpires`。

不改 `biz/uploader`：上传凭证签发由新模块自己完成，避免给 `UploadUsage` 枚举加
分支后还要回填 `presignFilename` 的用户前缀逻辑。

### SnapshotService 接口

新包 `biz/snapshot`，接口放在 `domain/snapshot.go`：

```go
type SnapshotService interface {
    // Presign 校验会话归属后登记快照行并签发上传凭证。
    Presign(ctx context.Context, req PresignSnapshotReq) (*Snapshot, *domain.PresignResp, error)

    // Complete 校验上报的 sha256 与对象实际摘要一致后把快照置为 ready，
    // 并把同会话更早的快照置为 superseded。两步在同一个事务内。
    Complete(ctx context.Context, id uuid.UUID, req CompleteSnapshotReq) (*Snapshot, error)

    // Fail 把 uploading 中的快照置为 failed 并写入错误信息。
    Fail(ctx context.Context, id uuid.UUID, cause string) error

    // CurrentByTask 返回该会话最新一份 ready 快照，无则返回 nil。
    CurrentByTask(ctx context.Context, userID, taskID uuid.UUID) (*Snapshot, error)

    // ListMine 分页返回该用户名下的 ready 快照。
    ListMine(ctx context.Context, userID uuid.UUID, p domain.PageParam) (*domain.Page, error)

    // Get 校验归属后返回快照；不属于该用户返回快照不存在业务码。
    Get(ctx context.Context, userID, id uuid.UUID) (*Snapshot, error)
}
```

`Snapshot.Stale` 是派生字段，不落库：`tasks.last_active_at > snapshots.completed_at`
即为过期。在 service 层 join `tasks` 计算，避免在 SQL 里写时间比较。

事务边界：`Complete` 必须在单事务内完成「置 ready」与「同会话其余快照置
superseded」，否则并发同步可能让同一会话出现两份 current 快照。

### 用户端路由

```
POST /api/v1/users/snapshots/upload        请求 {task_id, archive, filename}
POST /api/v1/users/snapshots/complete      请求 {id, sha256}
POST /api/v1/users/snapshots/{id}/fail     请求 {error}
GET  /api/v1/users/snapshots               分页列表
GET  /api/v1/users/snapshots/{id}
GET  /api/v1/users/snapshots/current       查询 {task_id}，会话无快照返回空体
```

路由全部挂在用户前缀下，走 `middleware.GetUser`，与 `biz/app` 的用户端路由
同构；不新增管理员端点，快照没有管理员视角。

与初版设计的两处偏差及原因：

- 预签端点落在 `POST /snapshots/upload` 而非 `/presign`，与 `biz/task` 的
  文件上传命名保持一致，客户端只需沿用既有的「上传」动词。
- 摘要与最新快照查询落在 `POST /snapshots/complete` 与
  `GET /snapshots/current?task_id=`，而不是 `/snapshots/{id}/complete` 与
  `/tasks/{id}/snapshot`：`complete` 的 `id` 与路径同义，放到 body 可复用既有
  的绑定写法；最新快照是「按会话查当前」，归到 `snapshots` 资源下比挂在
  `tasks` 下更贴合归属校验（快照本身属于会话）。

所有 handler 的归属校验都在 service 层完成，越权统一返回快照不存在业务码，
不暴露快照是否存在。

### AppBuildService 变更

`EnqueueBuildReq` 与 `BuildJob` 各增两列：`SourceSnapshotID` / `SourceTaskID`。

`EnqueueBuild` 的取数顺序：

1. 读 App，按既有逻辑完成锁定与配额判定。
2. 若 `apps.source_snapshot_id` 非空，调 `SnapshotService.Get` 校验快照归属与
   `status='ready'`；快照不存在、未就绪或归属不符，直接返回快照不存在业务码，
   不创建任务。
3. 判定快照过期：`tasks.last_active_at > snapshot.completed_at`。过期时返回
   快照过期业务码，不创建任务，让前端引导用户先去会话内同步。
4. 通过校验后把快照主键与来源会话主键写入任务行。

配额判定位置不变，仍按 `apps.build_count` 执行，需求 R6 因此零实现成本。

### 递归覆盖：runner 的 overlay-source 步骤

`runner.Config` 增加：

```go
type SourceSnapshot struct {
    ObjectKey string
    SHA256    string
    SizeBytes int64
}
```

worker 在建 `runner.Config` 时，若任务行带 `source_snapshot_id` 则填充该字段；
为空则保持零值，`overlay-source` 步骤整体跳过并报告 skipped。

解压流程：

1. 从对象存储下载到工作区副本内的临时文件，校验 sha256 与请求值一致。
2. 用归档读取器逐条遍历，先做路径清洗：
   - 条目名为空、绝对路径、或 `filepath.Clean` 后含 `..` → 中止，任务置
     `failed`，业务码为快照损坏。
   - 条目类型非 regular file 与目录（符号链接、设备文件、socket、FIFO）→
     同样中止。tar 的 symlink 是路径穿越的第二条通道，必须拦。
   - 单文件解压后字节数超过 `OverlayMaxBytes` → 中止。
   - 条目数超过 `OverlayMaxFiles` → 中止。
3. 校验通过后二次遍历写入工作区副本。同名文件直接覆盖，模板独有文件保留。
   写入目标目录不存在则 `MkdirAll`。
4. 删除临时文件。

两个上限常量放在 runner 包内，`OverlayMaxBytes = 512 << 20`，
`OverlayMaxFiles = 20000`。它们与对象存储的上传上限独立：前者约束解压后的
膨胀，后者约束归档本身。

中止路径统一调用同一个 cleanup，先删除工作区副本再返回错误，保证
`runner.Run` 的错误处理不会把覆盖了一半的工作区留到下次任务。

### 桌面端接入：共享壳工程上的覆盖与回滚

桌面端与移动端的差异是工作区所有权：`worker.workspaceFor` 给移动端 `cp -al`
一份 per-job 副本并在任务结束时删除；桌面端直接复用共享的 `desktop_workspace`
壳工程 checkout，`cleanup` 为空操作。因此桌面端多两件事：

1. 步骤位置：`overlay-source` 排在 `shell` 之后、`bundle` 之前。`shell` 执行
   `git checkout --force` 加 `git pull --ff-only` 重置工作树，覆盖排在它之前会
   被重置清掉；`bundle` 的 `beforeBuildCommand` 在打包阶段重新构建 `ui-next`，
   所以覆盖的前端源码同样参与编译。
2. 任务结束回滚：只记录覆盖**新建**的路径，`Run` 以 `defer` 调
   `rollbackOverlay` 按路径深度倒序删除，共享 checkout 恢复到自身工作树，本次
   会话文件不泄漏到下一个任务。被覆盖的既有文件不记入清单——回滚删除它会
   摧毁模板副本，追踪文件由下一个任务的 `git checkout --force` 自行还原。模板
   已有的目录同样不计入创建清单，其内其他文件保留。

覆盖根为壳工程根（含 `ui-next/`、`src/`、`tauri.conf.json`），快照内的相对路径
按壳工程结构落位。`binaries/` 由 runner 自行暂存 sidecar 与浏览器扩展
（壳工程 `.gitignore` 已排除该目录），落在其下的会话条目直接跳过并计数，避免
会话文件覆盖已暂存产物导致打包阶段失败。

实现上把 overlay 抽为 runner 包内共享的 `sourceOverlay`
（`runner/snapshot.go`），移动端与桌面端各持一份薄封装，共用同一套校验、写入与
上限常量，不复制实现。`DesktopBuildSteps` 与移动端一致在列表中声明
`overlay-source`，实际是否执行由是否绑定快照决定。

### 快照过期判定

R4 AC4 的过期条件在 service 层实现为一次 join：

```sql
SELECT s.*, (t.last_active_at > s.completed_at) AS stale
FROM app_source_snapshots s
JOIN tasks t ON t.id = s.task_id
WHERE s.id = $1 AND s.user_id = $2;
```

`tasks.last_active_at` 语义为会话最近一次活动时间，字段已存在，本特性不改
任务表的更新逻辑。会话在快照完成后再有任何活动即视为过期，属于保守判定：
可能把「快照后又做了一次无变更的对话」误判为过期，代价只是用户多点一次同步。

### 业务码分配

新增业务码接在既有 `114xx` App 打包段之后，避开 `11420`–`11424` 已用区间：

| 业务码 | 常量名 | 触发场景 |
|---|---|---|
| 11430 | `ErrSnapshotEmpty` | 会话工作区为空，无文件可打包 |
| 11431 | `ErrSnapshotSyncFailed` | 打包或上传失败 |
| 11432 | `ErrSnapshotTooLarge` | 归档字节数超过 `ObjectStorage.MaxSize` |
| 11433 | `ErrSnapshotNotFound` | 快照不存在、不属于该用户或未就绪 |
| 11434 | `ErrSnapshotStale` | 快照已过期，需先同步 |
| 11435 | `ErrSnapshotCorrupt` | 解压时发现非法路径、非文件条目或超过上限 |

`ErrSnapshotCorrupt` 用在 `overlay-source` 的中止路径，此时任务已创建，
因此同时把任务置为 `failed` 并写入错误信息，符合 R3 AC6/AC7。

### 前端

会话侧（会话详情 / 打包入口面板）：

- 「同步源码」按钮，点击后依次调用 presign → 会话执行器打包上传 → complete。
- 展示该会话当前快照：会话标题、创建时间、文件数、sha256 前 12 位。
- 过期时按钮变为高亮态，文案提示快照已落后于会话最近活动。

App 编辑页（`app-editor.tsx`）：

- 新增「源码来源」区块，与既有「素材槽位」区块同级。
- 未绑定：显示「使用模板工作区」与「选择快照」按钮，弹窗列出用户名下 ready
  快照，支持按会话标题筛选。
- 已绑定：显示会话标题、创建时间、文件数、sha256 前 12 位与过期标记，提供
  「解绑」按钮。
- `app.locked` 或快照过期时禁用提交入口，提示文案与既有锁定提示复用同一模式。

国际化沿用 `frontend/src/i18n/resources/{en,cn}.ts`，新增键统一挂在
`consoleApps.editor.sourceSnapshot.*` 与 `session.snapshot.*` 命名空间下，
两份资源叶子键集合必须完全一致，由既有
`frontend/test/i18n-resources-completeness.test.ts` 守护。

文案不使用表情符号，与项目约定一致。

## Data Models

### app_source_snapshots（新增）

见迁移 000036。状态取值：`uploading` → `ready` 或 `failed`。

`superseded` 是审计标记，不物理删除被取代的快照：已创建的历史构建任务通过
`app_build_jobs.source_snapshot_id` 引用这些对象，删除会让产物失去溯源能力。

### apps 变更

`source_snapshot_id uuid NULL`。无外键约束：快照是审计对象，允许其行长期存在，
加 FK 只会把「解绑」变成一次跨表写；解绑只改 `apps.source_snapshot_id`，
不触碰快照表。

### app_build_jobs 变更

`source_snapshot_id uuid NULL`、`source_task_id uuid NULL`，均在任务入队时一次性
写入，之后不可变。产物溯源只读，不参与任何查询路径。

## Configuration

本特性不新增任何配置项。快照大小上限、presigned URL 有效期、对象前缀全部复用
既有 `ObjectStorage.*`。因此无需修改 docker-compose，也无需新的环境变量。

## Correctness Properties

1. **品牌优先**：任一 App 记录的名称、包标识、版本号与素材键，在产物中的最终
   值只由 App 记录决定。判定方法：同一会话快照带 `app.json` 与
   `assets/icon.png`，绑定后打包，产物内相应字段与 App 记录一致。
2. **模板独有文件存活**：覆盖不删除模板独有的文件与目录。判定方法：覆盖后
   `android/gradlew`、`metro.config.js`、`babel.config.js`、`plugins/` 仍然存在。
3. **快照不可变**：快照置为 `ready` 后，其 `sha256` 与 `size_bytes` 不再被任何
   写入路径修改。
4. **同会话单 current**：任一时刻每个会话最多有一份 `superseded=false` 的
   `ready` 快照。
5. **覆盖不可逃逸**：归档内任何条目都无法写入工作区副本的父目录。判定方法：
   构造含 `../../etc/passwd`、绝对路径与 symlink 的 tar，任务必须失败且不落盘。
6. **任务工作区独立**：一次任务的覆盖结果不影响到下一次任务的模板副本。
7. **归属隔离**：用户 A 无法读取、绑定或触发打包用户 B 的快照，越权响应与
   资源不存在一致。

## Error Handling

- presign 阶段的失败（会话不存在、会话不属于该用户、会话不可构建、对象存储未
  启用）全部返回业务码，不产生快照行。
- 上传中断：会话侧超时后调 `Fail`，快照置 `failed` 并保留 `error` 字段。
  用户重试会创建新的快照行，不重用 uploading 中的行。
- `Complete` 的摘要比对失败：快照置 `failed`，`error` 记录期望与实际摘要，
  任务不落库，避免半上传对象被绑定。
- `overlay-source` 的下载失败、摘要不符、路径非法、条目超限、单文件超限全部
  归一到快照损坏业务码，任务置 `failed`，日志尾部保留 runner 原始输出供用户
  自查。
- 对象存储整体不可用：`Presign` 与 `overlay-source` 各自的失败路径独立，
  前者返回快照同步失败，后者返回快照损坏并把任务置 `failed`。

## Test Strategy

单元测试：

- `overlay-source` 的路径清洗：绝对路径、`..`、symlink、设备文件、空条目、
  超长单文件、超量条目数，各自断言任务失败且不落盘越界文件。
- 覆盖语义：同名覆盖、目录合并、模板独有保留、空目录条目。
- 快照过期判定：`last_active_at` 早于、等于、晚于 `completed_at` 三种边界。
- `Complete` 事务原子性：置 ready 与置 superseded 在并发下不产生两份 current。
- `EnqueueBuild` 的取数与拒绝路径：无绑定、绑定未就绪、归属不符、过期。

集成测试：

- 快照 presign → PUT → complete 全链路，用既有测试对象存储夹具。
- 绑定快照 → 提交打包 → worker 消费 → 产物存在，断言 `app_build_jobs`
  的 `source_snapshot_id` / `source_task_id` 已写入。
- 配额穿透：免费档 App 用快照打包成功后 `build_count=1` 且 `locked=true`。

回归验证命令（沿用项目既有基线）：

```bash
# backend/
go test -p=1 ./biz/mobilebuild/... ./biz/snapshot/...
go build -p=1 ./cmd/server ./biz/... ./domain/...

# frontend/
npx tsc -b
npx eslint <changed>
npm run build
node --test test/i18n-resources-completeness.test.ts
```

前端文案完整性由 `frontend/test/i18n-resources-completeness.test.ts` 强制：中英文资源
叶子键集合必须完全一致。

## References

- 需求文档：`.monkeycode/specs/session-sourced-app-packaging/requirements.md`
- 既有打包规格：`.monkeycode/specs/app-self-service-packaging/requirements.md`、
  `.monkeycode/specs/app-self-service-packaging/design.md`
- 构建链入口：`backend/biz/mobilebuild/domain/appbuilder.go:127`（`EnqueueBuildReq`）
- 工作区复制：`backend/biz/mobilebuild/worker/worker.go:224`（`workspaceFor`）
- runner 配置与步骤链：`backend/biz/mobilebuild/worker/worker.go:257`、
  `backend/biz/mobilebuild/runner/runner.go`
- 上传与 presign 机制：`backend/biz/uploader/handler/http/v1/uploader.go:71`、
  `:118`
- 对象前缀白名单：`backend/pkg/oss/oss.go:313`、`backend/pkg/asseturl/asseturl.go:44`
- 上传用途枚举：`backend/consts/uploader.go:3`
- 业务码登记：`backend/errcode/errcode.go`（App 段 `114xx`）
- 迁移风格参照：`backend/migration/000035_add_app_build_jobs_app_id.up.sql`
- App 与权益表：`backend/migration/000026_create_apps.up.sql`
- 前端编辑页：`frontend/src/components/console/apps/app-editor.tsx`
- 国际化资源：`frontend/src/i18n/resources/en.ts`、`frontend/src/i18n/resources/cn.ts`
