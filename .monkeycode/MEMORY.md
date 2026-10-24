# 用户指令记忆

本文件记录了用户的指令、偏好和教导，用于在未来的交互中提供参考。

## 格式

### 用户指令条目
用户指令条目应遵循以下格式：

[用户指令摘要]
- Date: [YYYY-MM-DD]
- Context: [提及的场景或时间]
- Instructions:
  - [用户教导或指示的内容，逐行描述]

### 项目知识条目
Agent 在任务执行过程中发现的条目应遵循以下格式：

[项目知识摘要]
- Date: [YYYY-MM-DD]
- Context: Agent 在执行 [具体任务描述] 时发现
- Category: [运维部署|构建方法|测试方法|排错调试|工作流协作|环境配置]
- Instructions:
  - [具体的知识点，逐行描述]

## 去重策略
- 添加新条目前，检查是否存在相似或相同的指令
- 若发现重复，跳过新条目或与已有条目合并
- 合并时，更新上下文或日期信息
- 这有助于避免冗余条目，保持记忆文件整洁

## 条目

[Online 预览构建与验证码验收]
- Date: 2026-07-26
- Context: Agent 在排查 online 构建后登录验证码失败时发现
- Category: 构建方法|测试方法|排错调试
- Instructions:
  - 在 `frontend` 运行 `pnpm run build:online` 验证 online 生产构建。
  - 启动 online 开发预览时显式设置 API 目标，例如 `TARGET=http://38.39.30.160 pnpm run dev:online -- --host 0.0.0.0 --port 11180`。vite.config.ts 固定 dev 端口 11180，`server.allowedHosts` 已含 `.monkeycode-ai.online`，无需改配置。
  - 本机内存 7.8Gi 且无 swap，前端构建用 `NODE_OPTIONS=--max-old-space-size=3584` 即可完成（历史 6144 记录是在机器更空闲时得出，偏低会导致 OOM；后端 go build 需另起一个 memory_percent 40 的后台终端，两者勿并发）。
  - 获得预览地址后运行 `PREVIEW_URL=<URL> pnpm run check:online-preview`，验证 CAP JavaScript、WASM 和 challenge API。
  - 自动健康检查通过后，在浏览器完成一次真实验证码求解和登录，再开始登录后页面的 UI 验收。
  - UI 验收需等待 `document.fonts.ready`，确认 JetBrains Mono Variable 与 Noto Sans SC Variable 已加载，并检查浏览器控制台和 Network 中没有字体资源失败。
  - 在 320px、375px、390px、430px 和 1280px 对照基准页面核对字体族、字号、字重和行高，字体变化应作为构建后高频回归项记录和处理。
  - Vite 日志出现 `Must set target or forward` 表示 `/api` proxy 缺少 `TARGET`，应使用显式目标重启预览。

[AuthyIDE 服务器部署与 SSH 限流]
- Date: 2026-10-17
- Context: Agent 在执行 AuthyIDE 改名与服务器私有化部署时发现
- Category: 运维部署|构建方法|排错调试
- Instructions:
  - 目标服务器 38.39.30.160（root, 22 端口），凭据存放在 `/root/.config_ssh_srv.sh`，不得写入仓库或聊天输出。该文件是「exec sshpass」包装脚本，不能 source（会执行 ssh 并 exit）；循环重试脚本里用 `export SSHPASS=$(awk -F"'" '/^export SSHPASS/{print $2}' /root/.config_ssh_srv.sh)` 提取到环境变量。
  - SSH 突发连接限流，症状 `kex_exchange_identification: Connection closed by remote host`，冷却约 15 分钟：连发 2-3 次后即使隔 100-200 秒的单连接仍被掐。所有操作必须合并进单次连接（`cat tarball | ssh ... 'tar xzf - && bash script.sh'`）。长脚本输出先落盘再 `cat`，ssh 中途 Broken pipe 会丢掉管道内全部输出。
  - 给远程脚本传值不要用 `read`（bash 从管道分块预读，`read` 会吃掉后续内容）：把首行写成纯赋值 `VAR=value`，脚本里 `V=${V:-}` 后 `set -u`。大文件走 SSH stdin，不要 base64 内联进 argv（190KB 就 `Argument list too long`）。
  - 部署前置：`/opt/authyide/src/frontend/node_modules` 已装好（66 条目），增量部署无需 `pnpm install`，直接 `node /root/.cache/node/corepack/v1/pnpm/9.12.3/bin/pnpm.cjs exec tsc -b` 再 `exec vite build --mode offline`；`tsc -b` 基线 0 错误。前端生产构建 `VITE_APP_EDITION=offline pnpm exec vite build --mode offline`。
  - 后端跑在 `/opt/authyide/stack`（compose 项目名 `authyide`），随机凭据在 `stack/creds.env`，管理员初始账号 `admin@authyide.local`。配置全由 `MCAI_*` 环境变量注入（viper `AutomaticEnv` + `SetEnvPrefix("MCAI")`，点号换下划线），仓库内无 `.env.example`。`TASKFLOW_SERVER` 为空会直接 panic 但启动阶段不连，给个假地址（`http://127.0.0.1:9`）即可，仅 VM/Host/端口转发功能失败。
  - 后端在 Docker 内构建（`golang:1.25.8-alpine3.23`），服务器无 Go；Ubuntu 24.04 的 apt 源没有 `docker-buildx-plugin`，只能 legacy builder，上游 Dockerfile 的 `--platform` / `--mount` 需改写为 `COPY go.mod go.sum ./ && RUN go mod download && COPY . . && RUN go build`。
  - compose 的 backend/storage 必须补 `ports: ["127.0.0.1:8888:8888"]`、`["127.0.0.1:9000:9000"]`，否则 nginx 上游全部 502。前端由宿主机 nginx 提供：root 指向 `/opt/authyide/src/frontend/dist`，`/api/` 反代 `127.0.0.1:8888`，`/oss/` 反代 `127.0.0.1:9000`。
  - 健康标准：`docker ps` 全 Up、`docker logs authyide-backend | grep -c 'panic:'` 为 0、日志有 `http server started on [::]:8888` 与 `init team success`、`GET /` 200、`GET /api/` 404（404 才是通，502 是上游断）、`GET /oss/` 403（rustfs 正常）。
  - 重置管理员密码不能改 `MCAI_INIT_TEAM_PASSWORD`（`InitTeam`/`ensureInitTeamMember` 只在用户不存在时创建，`biz/team/repo/user.go`）。服务器无 SMTP，找回密码走不通。正确做法直接改 `users.password` 列：`HashPassword` 是 bcrypt cost 10，宿主机 python3 自带 bcrypt，`bcrypt.hashpw(pw.encode(), bcrypt.gensalt(10))`。admin 有两条记录（`role=enterprise` 与 `role=subaccount`，后者才是实际登录的那个），两条都要改，改完 `bcrypt.checkpw` 预检。
  - 验证脚本先带旧 cookie 打 `GET /api/v1/users/apps`（会话在 redis，compose 重启不失效），200 就复用，失败才临时关验证码，可以把关验证码窗口降到零次。关验证码用 `trap ... EXIT` 保证退出即恢复 compose。
  - 服务器 compose 结构：顶层 `name: authyide`，服务缩进 2 空格、服务内键 4 空格，`environment:` 全部是 **mapping 风格**（`KEY: value`）不是 `- KEY=VALUE`。`stack/creds.env`（POSTGRES_PASSWORD/REDIS_PASSWORD/CLICKHOUSE_PASSWORD/RUSTFS_ACCESS_KEY/…）**不被 compose 引用**，是安装器遗留；实际注入走 backend 的 `environment:`。改 compose 前先备份到 `/root/deploy-backup-<ts>`。
  - 解析 backend 镜像名不要依赖 `docker compose config`：`$DC config | awk` 在本服务器取回空串（会误判失败），直接对原始 compose 文本用 awk 扫 `^  backend:` 到下一个 `^  [a-zA-Z]` 段再取 `image:` 行。改完 compose 后用 `$DC -f <file> -p authyide config > /tmp/x.yml` 做一次解析校验。
  - 服务器改 compose 用 `Artifacts/deploy/compose_patch.py`（`--static` 补 `/opt/authyide/data/static:/app/static:ro` 挂载，`--smtp <密码>` 写邮件配置），它按缩进自动匹配 mapping / block sequence 两种 environment 风格，已验证幂等，且保留 `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: '1'` 这类引号数字不被 PyYAML 污染。
  - SMTP 只读 `smtp.*` 六个键（host/port/username/from/tls），映射 `MCAI_SMTP_*`。**后端没有 IMAP**，收信能力不存在，用户给的 IMAP 设置只供其自家邮件客户端用。`smtp.tls=true` 走 `tls.Dial` 隐式 TLS 直连、`false` 走 `smtp.Dial` **不升级 STARTTLS**，所以 QQ 邮箱必须 `smtp.qq.com:465` + `tls=true`（587+false 会因不 STARTTLS 而无法 AUTH）。`smtp.username` 字段实际未被使用，AUTH 用的用户名是 `smtp.from`，因此 `from` 必须填真实邮箱地址。
  - SMTP 凭据放 `/root/.authyide-smtp.env`（`MCAI_SMTP_PASSWORD=...`，chmod 600，工作区之外，与 `/root/.config_ssh_srv.sh` 同模式），`run-deploy.sh` 读取后通过**同一次** SSH 会话以 `MCAI_SMTP_PASSWORD_VAL='<值>'` 前缀转发给远程脚本；值必须是纯字母数字（脚本有 `[A-Za-z0-9]+` 校验，否则拒跑），这样凭据不落在仓库、脚本或聊天正文里。
  - SMTP 连通性可用宿主机 python3 标准库 `smtplib.SMTP_SSL('smtp.qq.com', 465)` + `login` 直接验证，本机与服务器都跑通过；AUTH 成功后 banner 含 `AUTH LOGIN PLAIN XOAUTH XOAUTH2`。
  - 服务器 nginx `/etc/nginx/conf.d/authyide.conf` **原本没有 `location /static/`**，只有 `/api/`、`/oss/`、静态资源正则、`/` SPA 兜底。后果：`GET /static/authyide.apk` 返回 200 但内容是 2216 字节的 `index.html`（SPA 兜底），HTTP 码正确却拿不到 APK。必须插入 `location /static/ { proxy_pass http://127.0.0.1:8888; }` 再 `nginx -t && nginx -s reload`；校验要比对 `stat -c %s` 与实际下载字节数，只看 200 会误判成功。
  - compose 里补静态卷必须按**容器路径** `/app/static` 去重，不能按完整字符串 `/opt/authyide/data/static:/app/static:ro`：服务器原挂载是 `:rw`，按 `:ro` 判断会漏检并追加出第二条同路径挂载。正确做法是无条件剥掉所有含 `/app/static` 的行再补回规范行，天然幂等且能自愈已产生的重复。
  - 不要把密钥字面量写进部署脚本：本环境的工具链会把敏感值静默改写成 `***`，写进脚本后远程拿到的是字面 `***`（表现为配置项全空）。正确做法是运行期 `mktemp -d` 组装临时目录，把 `/root/.authyide-smtp.env` 以 `.smtp.env` 成员塞进**同一条** SSH stdin 管道流的 tarball，远程 deploy.sh 解析后立即 `chmod 000` + 删除。这样密钥不经过 argv、不落工作区、不走第二次 SSH 连接。
  - `frontend/src/i18n/resources/{cn,en}.ts` 是单个约 4500 行对象。值里未转义双引号会让整文件解析失败并级联约 1900 条 TS 错误全指向同一文件，改用单引号或中文弯引号；同一嵌套对象内重复 key 会被 `tsc -b` 以 TS1117 直接报出（见下条）。新增命名空间的安全锚点是 `welcomeShell: {`，但 edit 工具 `replaceAll: false` 只替换首次匹配，跨文件复用前必须 `grep -n` 确认缩进与上下文唯一。
  - 同一嵌套对象内重复 key 是**硬错误** `TS1117: An object literal cannot have multiple properties with the same name`（会直接指向重复行号），不会静默通过；往已有块里插 key 前必须 `awk`/python 抽出该块的完整叶子键集合比对，`grep -A 60` 会被 60 行截断漏掉后半块。
  - **en.ts 曾被截断到 2077 行**（`consoleApps` 之后 `welcomeShell` 及全部后续命名空间丢失，文件直接 `export default en`），tsc 不报错但整个英文 UI 会只剩 key 路径。恢复来源：`/tmp/opencodes/stage6/push-20261019-075547/frontend/src/i18n/resources/en.ts`（4394 行，最新的本地快照），同批还有 `/tmp/opencodes/stage{4}/`、`/tmp/opencodes/stage/`、`/tmp/opencode/{stage,build,monkeycode}/` 几份 4394-4578 行快照。恢复后要把本次特性新增的块（`tabs.build` + `consoleApps.builds`）重新插回。en/cn 叶子键一致性由 `test/i18n-resources-completeness.test.ts` 强制，跑 `node --test test/*.test.ts` 验证。

[AuthyIDE 改名的不可改名项]
- Date: 2026-10-15
- Context: Agent 在执行 MonkeyCode → AuthyIDE 品牌改名时发现
- Category: 排错调试|环境配置
- Instructions:
  - 不得手工修改 OpenAPI 自动生成的类型名（`MonkeyCodeBackend...`、`github.com/chaitin/MonkeyCode/backend...`），必须先改 Go module path 并重新生成 `frontend/src/api/Api.ts`。
  - 不得修改 MCP 工具运行时名称（`monkeycode-ai_MonkeyCode__*`，见 `frontend/src/components/console/task/message-toolcall.tsx`），它们由服务端下发。
  - 订阅档位 slug `monkeycode-basic` / `monkeycode-pro` / `monkeycode-ultra` 是后端定义值，前端需保持一致。
  - `frontend/src/utils/config.tsx` 中的 `MonkeyCodeOfficialPlugins/main/skills/*` 是 git submodule 路径，改名需同步 `.gitmodules`。
  - 上游分析脚本（Google Analytics `G-RQQN0BK11M` + Matomo `stats.monkeycode-ai.com`）已整体移除；`frontend/src/lib/matomo.ts` 保留但变为惰性无害代码（仅向本地 `window._paq` 数组入队）。

[后端构建链路：本地校验与服务器镜像]
- Date: 2026-10-15
- Context: Agent 在部署 AuthyIDE 后端时验证新写的 biz/app 模块时发现
- Category: 构建方法|环境配置|排错调试
- Instructions:
  - 本地执行环境装有 Go 1.25.6（`/usr/local/go/bin/go`），满足 `backend/go.mod` 的 `go 1.25.0` 要求，可在提交到服务器前先本地验证后端代码。完整流程：`cd backend && go mod download && go generate ./ent && go build ./...`，`go mod download` 依赖 `/go/pkg/mod` 缓存（约 154M 预置）。
  - `go generate ./ent` 耗时数分钟、内存峰值约 2.9 GiB；`go build ./...` 全量约 8 分钟、峰值约 3.4 GiB。必须通过 background terminal 执行并设置 `memory_percent`，前台 shell 会在 200MB 可用内存下被杀。新增 ent schema 后必须先本地跑一次 `go generate ./ent`，生成的 `db/<type>/<type>.go` 才有对应字段常量。改完 schema 可用 `diff -rq <改前备份> db` 确认只影响目标类型的生成文件，未备份前先用 `cp -r db /tmp/opencode/db-backup`。
   - 本机内存有气球，`free` 显示的可用量常被压到 1-2GB 以下。此前（2026-10-15）`go build ./...` 会卡在最后的 `link` 阶段（`a.out` 停在 0 字节，`link` 进程 8 分钟只耗 9 秒 CPU 时间），用 `ps -o cputime= -p <link_pid>` 采样 30 秒即可判定；当时结论是放弃本地链接。**2026-10-20 复测该死锁已不复现**：`cd backend && go build -o /tmp/opencode/server ./cmd/server` 约 9 分钟跑通、峰值约 1.35 GiB，输出 `SERVER_BUILD_OK` 即可作为服务端整体集成校验（DI 装配、register、handler 全部走通），不需要再退回 grep-v 绕开 link 的做法。前提是给 background terminal 至少 `cpu_percent: 500`，否则 15 分钟超时也跑不完。**2026-10-20 晚间该机的 20G 根分区被打满（`df -h /` 显示 100%、0 可用，`/go/cache` 占 2.5G），`go build -o /tmp/opencode/server ./cmd/server` 的编译阶段能通过但 link 阶段报 `mapping output file failed: no space left on device`**，此时按下一行用「跳过 link 的包级编译」验证代码正确性，并把产物留在 `/tmp/opencode/server` 视为旧二进制（时间戳会早于最后一次改动）。
    - background terminal 的 shell 是 `sh` 而不是 `bash`，`${PIPESTATUS[0]}` 和 zsh 风格 `${pipestatus[1]}` 都会报 `Bad substitution`（命令本身可能已成功，只是回显失败，且会把整条命令的退出码顶成 2）；`time` 内建也不可用（`sh: 1: time: not found`）。要拿到真实退出码就用 `cmd > /tmp/opencode/x.log 2>&1; echo "exit=$?" >> /tmp/opencode/x.log`，不要用管道 + PIPESTATUS 回显。
  - 跳过 link 阶段验证全部业务代码的正确写法：`go build $(go list ./... | grep -v -e '/cmd/' -e '^github.com/chaitin/MonkeyCode/backend/migration$')`，约 2 分钟完成，非 main 包不需要链接。`./migration` 必须排除，它只有 `migration_test.go` 没有非测试 Go 文件，`go build` 会报 `no non-test Go files`。`go vet` 对这些包同样可用。
  - 上一条包级编译的前提是内存充裕。2026-10-21 晚间可用内存只剩约 1GiB 时，即使把 `GOCACHE`/`GOTMPDIR` 挪到 `/dev/shm`（3.9G 空余），`go build ./biz/mobilebuild/repo` 也在编译 `backend/db`（ent 生成的巨型包）时被 OOM killer 终止：日志末行是 `github.com/chaitin/MonkeyCode/backend/db: /usr/local/go/pkg/tool/linux_amd64/compile: signal: killed`，终端 `oom_killed: yes`，峰值内存顶满 3.11GiB 的 cgroup 上限。这种终止在待改文件**之前**发生，既不是编译错误也不是语法问题，重试也只会重复。此时改走静态核实：`gofmt -l` 校验语法（exit 0 即语法通过），再逐个到 `db/` 生成代码里确认引用的标识符真实存在（例如 `AppUpdate.SetLocked` 在 `db/app_update.go`、`Entitlement.Tier` 在 `db/entitlement.go`）。腾盘腾内存的优先级：先清 `/dev/shm` 里自己放的构建缓存。
  - 服务器 Ubuntu 24.04 的 docker-ce apt 源里没有 `docker-buildx-plugin`，`docker buildx` 不可用，只能用 legacy builder。上游 `backend/build/Dockerfile` 依赖 BuildKit 特性（`FROM --platform=$BUILDPLATFORM`、`RUN --mount=type=cache|bind`），legacy builder 会因 `--platform=""` 解析失败或不识别 `--mount`；部署时需改写为无 `--platform`、无 `--mount` 的等价 Dockerfile（`COPY go.mod go.sum ./` + `RUN go mod download` + `COPY . .` + `RUN go build`）。`docker-compose-v2` 可从同一 apt 源安装。
  - `clickhouse/clickhouse-server:24.8-alpine`、`redis:7-alpine` 可从 Docker Hub 拉取；`chaitin-registry.cn-hangzhou.cr.aliyuncs.com/basic/{postgres:18.6,rustfs:v1.0.0-rc.5}` 可匿名拉取。

[后端 DI 注册冲突与单实例缺失依赖]
- Date: 2026-10-15
- Context: Agent 在首次启动包含 biz/app 模块的后端时连续遇到四个 DI panic，逐一修复
- Category: 排错调试
- Instructions:
  - 后端使用 samber/do，服务名由 `generateServiceName[T]` 即 `fmt.Sprintf("%T", zero[T])` 生成。`%T` 不带完整包路径，所以同模块下 `biz/app/handler/v1.Handler` 与 `biz/subscription/handler/v1.Handler` 的服务名都是 `*v1.Handler`，两个模块都用 `type Handler struct` 会在启动时 panic：`DI: service `*v1.Handler` has already been declared`。新增 handler 时结构体名称必须全局唯一（用 `AppHandler` 而非 `Handler`）。
  - 上游把部分依赖只在企业版通过 `bridge.WithXxx` 注入，`cmd/server/main.go` 的开源构建路径从未提供，导致 `InvokeAll` 阶段 panic（panic 出现在 `biz/team/register.go` 的 `InvokeTeam`）。这类问题编译期完全不可见，只能启动才暴露。
  - 缺失一：`domain.MemberManager`（`biz/team/usecase/oidc.go` 与 `biz/team/handler/http/v1/user.go` 用 `MustInvoke` 解析）。单实例不提供成员邀请，已在 `biz/team/usecase/member_manager.go` 实现并统一返回 `errcode.ErrMemberInviteUnavailable`（11408），在 `biz/team/register.go` 的 `ProvideTeam` 中 `do.Provide`。
  - 缺失二：`domain.PublicHostUcase`（`biz/git/handler/v1/webhook_github.go` 与 `webhook_codeup.go` 误用 `MustInvoke`）。改为可选注入（`do.Invoke` + 判空），调用点加 `h.pubhost == nil` 短路，与 `biz/task/handler/v1/task.go` 的既有写法一致。
  - 另一类入口差异：错误码本地化只在企业版 `bridge.go` 注册（`w.SetLocale(locale.NewLocalizerWithFile(language.Chinese, errcode.LocalFS, ...))`），OSS 入口 `main.go` 缺失，导致所有错误响应回退成裸 key，形如 `"message \"err-unauthorized\" not found in language \"zh\""`。排查特征：日志里没有任何 locale/toml 加载或解析失败记录（加载器根本没被调用），且换 `Accept-Language` 头只改变报错里的语言名而不改变回退行为。拿到 "not found in language" 文案后 `grep -rn` 源码找不到，说明来自依赖包，再反向搜 `SetLocale` / `NewLocalizerWithFile` 的调用点即可定位哪个入口注册过。
  - 缺失服务可通过静态审计一次找全：panic 消息里的 `available services:` 是该时刻已注册的全部服务名，用脚本提取所有 `do.MustInvoke[X]` / `do.Invoke[X]` 的目标类型与之比对，差集即缺失项（脚本见 `/tmp/opencode/audit_di.py`）。比对时按后缀匹配（`%T` 无包路径）；`do.Invoke`（可选）与 `do.MustInvoke`（必选）必须区分，只有 `MustInvoke` 会 panic。
  - 可在 `backend/_ditest/main.go` 里用 `fmt.Sprintf("%T", (*pkgv1.Type)(nil))` 快速比对两个类型的服务名是否冲突，验证后删除该目录。

[rustfs 崩溃的真实原因与修复]
- Date: 2026-10-15
- Context: Agent 在启动 AuthyIDE 对象存储容器时发现
- Category: 排错调试|运维部署
- Instructions:
  - `chaitin-registry.cn-hangzhou.cr.aliyuncs.com/basic/rustfs:v1.0.0-rc.5` 崩溃循环（`Restarting`、restarts 计数持续增长）的真实原因是 `Io error: Permission denied (os error 13)`：容器以非 root 用户 `rustfs` 运行，无法写入 root 所有的 bind mount 数据目录 `/opt/authyide/data/rustfs`。
  - 修复方式：`chmod 0777 /opt/authyide/data/rustfs && chmod -R a+rwX /opt/authyide/data/rustfs`，然后 `docker compose up -d --force-recreate storage`。修复后 `/health` 返回 200，容器 stable healthy，无需更换镜像。
  - 第二个坑：compose 里 `RUSTFS_SECRET_KEY` 被写成和 `RUSTFS_ACCESS_KEY` 一样的占位值 `authyide`（8 字符），而后端用的是 `creds.env` 生成的 20 字符 secret，导致 S3 SigV4 报 `SignatureDoesNotMatch`，`CreateBucket` 在 `InvokeTeam` 阶段 panic。修复：把 `RUSTFS_SECRET_KEY` 改成与 `MCAI_OBJECT_STORAGE_ACCESS_KEY_SECRET` 相同的值，并先移走 rustfs 旧数据目录（它会持久化首次初始化的凭据）再重建容器。
  - 排查技巧：不要打印明文 key，用 python 对 compose 文本和 `docker inspect` 的环境变量分别算 sha256 前 12 位比对，只输出长度和哈希即可定位不一致项。
  - `minio/minio:latest` 与私有镜像源 `basic/minio:latest` 在该服务器上均返回 `pull access denied`（不是超时，是无权拉取），minio 回退方案不可行，不要再尝试。
  - nginx `/oss/` 指向 `127.0.0.1:9000`，对后端透明；后端配置 `MCAI_OBJECT_STORAGE_ACCESS_ENDPOINT` 保持 `http://<IP>:80/oss` 即可。
  - 客户端与存储服务端直连更快：把 `MCAI_OBJECT_STORAGE_ENDPOINT`（服务端内网地址）设为 `http://authyide-storage:9000`，bucket 名用 `MCAI_OBJECT_STORAGE_BUCKET=authyide` 覆盖上游默认 `monkeycode-ai`。
  - viper `AutomaticEnv` 下空字符串环境变量等于未设置，因此 `MCAI_LOKI_ADDR=""` 无法覆盖默认值；要用不可达地址（如 `http://127.0.0.1:1`）才能真正覆盖。`MCAI_TELEMETRY_SERVICE_NAME=authyide-backend` 与 `logger.service_name`（需 `mapstructure:"service_name"` 标签，否则下划线键不匹配）可清掉日志与遥测里的 monkeycode 字样。

[ClickHouse 24.8 拒绝 DateTime64 列上的 TTL]
- Date: 2026-10-15
- Context: Agent 在后端启动时遇到 `migration failed in line 0: CREATE TABLE IF NOT EXISTS task_logs` 与 `Dirty database version 1` 时发现
- Category: 排错调试
- Instructions:
  - ClickHouse 24.8 要求 TTL 表达式结果必须是 `DateTime` 或 `Date`，直接写 `TTL ts + INTERVAL 60 DAY`（`ts DateTime64(9,'UTC')`）会报 `Code: 450 BAD_TTL_EXPRESSION: TTL expression result column should have DateTime or Date type, but has DateTime64(9, 'UTC')`。
  - 修复：`TTL toDateTime(ts) + INTERVAL 60 DAY`，与同目录 `000002_create_model_usage_events.up.sql` 的写法保持一致。`clickhouse_single` 与 `clickhouse_cluster` 两份迁移都要改。
  - golang-migrate 在失败时会把版本标为 dirty，之后每次启动都直接报 `Dirty database version 1. Fix and force version.`，即使 SQL 已修好也不会自愈。修复 SQL 后必须清空迁移表：`clickhouse-client --database authyide --query "DROP TABLE IF EXISTS schema_migrations; DROP TABLE IF EXISTS task_logs; DROP TABLE IF EXISTS model_usage_events"`，驱动下次启动会重建并重新执行。
  - ClickHouse 表由 `pkg/clickhouse/client.go` 的构造函数在 DI 解析期执行迁移，不是 main 的显式迁移步骤，因此错误栈会挂在 `biz/team/register.go` 的 `InvokeTeam` 上，容易误判为 DI 问题。真实错误要用 `docker exec authyide-clickhouse clickhouse-client --database authyide --query "SHOW TABLES"` 和手动重放建表语句才能看到。
  - 迁移 SQL 被打进镜像（Dockerfile `COPY migration ./migration`），改完必须重建后端镜像并 `docker compose up -d --force-recreate backend` 才生效。

[web.BindHandler 的请求参数必须是值类型]
- Date: 2026-10-15
- Context: Agent 在 E2E 验证 biz/app 接口时发现全部返回 400「参数错误」
- Category: 排错调试
- Instructions:
  - `web.BindHandler[T any](fn func(*Context, T) error)` 内部是 `var t T; ctx.Bind(&t)`。Handler 的请求参数若写成指针（`req *domain.XReq`），类型推断得到 `T = *domain.XReq`，于是 `ctx.Bind` 收到的是 `**domain.XReq`（nil 指针），绑定必然失败并返回 400 `参数错误`。正确写法是值类型：`req domain.XReq`，调用 usecase 时再传 `&req`。仓库内既有 handler 全部是值类型，`grep -rn 'func (.*) \w*(c \*web\.Context, .* \*domain\.'` 可用于找出不合规项。
  - 两个失败点在 echo v4.15.1：`bind.go:195` 的 `bindData` 对非 struct 目标直接报错；GET 请求无 `Content-Type` 时 `bind.go:121` 落入 `default: return ErrUnsupportedMediaType`，所以 GET 也一起 400，而不是只有 POST。
  - 排查陷阱：401/400 都无法证明路由存在（group 级 auth 中间件在路由匹配前执行）。`ctx.Failed` 会写日志 `request failed`（带 trace_id 与 err），是确认绑定失败的可靠依据；响应体里的 trace_id 可直接用于日志检索。
  - `w.Group("/api/v1/users/apps")` + `v1.GET("")` 注册出的路径是 `/api/v1/users/apps`（echo `Group.Add` 直接做字符串拼接），这个写法本身没问题，不是 400 的原因。

[ent 软删除必须走 Delete() 而不是 Update()]
- Date: 2026-10-15
- Context: Agent 在 E2E 验证 App 删除时发现接口返回 200 但行未删除
- Category: 排错调试
- Instructions:
  - 带 `entx.SoftDeleteMixin2` 的 schema（app、model、gitbot、host、image、mcptool、mcpupstream、notifychannel 等）：查询被拦截器自动加上 `WHERE deleted_at IS NULL`（`entx.SkipSoftDelete(ctx)` 可包含已删除数据）；删除由钩子拦截 `OpDeleteOne|OpDelete` 并改写成 `deleted_at = now()` 的 update。
  - 因此 repo 的 Delete 必须写 `r.db.X.Delete().Where(x.UserID(uid)).Where(x.ID(id)).Exec(ctx)`（参考 `biz/setting/repo/model.go` 的 `modelRepo.Delete`）。写成 `UpdateOneID(id).Exec(ctx)` 是无操作，接口照样返回 200 成功但行原封不动。
  - 排查要点：`SELECT count(*) FROM apps` 不含 `deleted_at IS NULL` 会把已软删除行也算进去，容易误判删除失败；反过来接口返回 200 也不等于行已删除，必须查 `deleted_at IS NOT NULL` 或对已删除 id 再 GET 一次确认返回 404。

[Submodule 实际可克隆性与打包执行链路缺口]
- Date: 2026-10-16
- Context: Agent 在被要求克隆 `.gitmodules` 中声明的两个 submodule 时发现
- Category: 构建方法|排错调试
- Instructions:
  - 本 fork 的 `.gitmodules` 未被 git 跟踪（`git status` 显示 `?? .gitmodules`），因此 `git submodule update --init` 是空操作（退出码 0、无输出）。必须直接把仓库 clone 到 submodule 路径（`git clone --depth 1 <url> plugins`）。
  - `git@github.com:chaitin/OhMyAgent.git` 已不可访问，`git ls-remote` 返回 `Repository not found`、`https://github.com/chaitin/OhMyAgent` 返回 404；上游 `chaitin/MonkeyCode` 的 `.gitmodules` 内容完全相同，即这是上游自身的状态（私有或已删除），不是本仓库配置错误。`https://github.com/chaitin/MonkeyCodeOfficialPlugins` 公开可访问（200）。
  - 全仓库没有任何代码引用 `agent/` 路径（前端、后端、packer、tools 均无），所以 agent submodule 缺失不影响构建与运行；`plugins/skills` 被 `frontend/src/utils/config.tsx` 的 `defaultSkills`（`MonkeyCodeOfficialPlugins/main/skills/*`）间接引用。
  - SSH URL 无法用密钥认证时可用 `-c url.https://github.com/.insteadOf=git@github.com:` 在 fetch 期改写为 HTTPS，无需修改 `.gitmodules`。

[打包管线已删除：服务器遗留与部署要点]
- Date: 2026-10-17
- Context: Agent 在删除 Flutter 打包构建管线并部署用户中心广告配置时发现
- Category: 运维部署|构建方法|排错调试
- Instructions:
  - Flutter 打包管线（`backend/cmd/appworker`、`backend/pkg/appbuild`、`packer/scaffold.sh`、`packer/preview.sh`、`packer/flutter_overlay/`、前端 `app-builds.tsx`）已整体删除。广告网络选择保留但只存储，不做投放、结算、SDK 托管与 App ID 校验。`packer/ad_networks.yaml` 是广告网络目录唯一权威源，后端启动读入内存并暴露 `GET /api/v1/users/apps/ad-networks`；后端已挂载 `/opt/authyide/src/packer:/opt/authyide/src/packer:ro`，对齐默认路径 `MCAI_AD_NETWORKS_FILE`，yaml 缺失或解析失败只降级内嵌 fallback 并 warn。
  - 迁移不需外部工具：`pkg/store/entdb.go` 的 `MigrateSQL` 用 go-migrate 以 `file://migration` 在启动时执行 `backend/migration/*.sql`，SQL 经 `COPY migration ./migration` 打进镜像。改迁移只需重建 `authyide/backend:local` 并重启 backend，版本记在 postgres `schema_migrations`。
  - 密码登录入口 `POST /api/v1/users/password-login`，会话走 cookie（`Session.Save`，`consts.MonkeyCodeAISession`），响应体是 user 对象而非 token，curl 需 `-c/-b` 存 cookie jar。
  - 验证码默认开启（`config.go` 的 `v.SetDefault("security.captcha_enabled", true)`，可用 `MCAI_SECURITY_CAPTCHA_ENABLED=false` 关），`go-cap` 的 50x32 三数字图不适合程序化求解。冒烟验证登录后接口时临时在 compose 加该变量，并用 `trap ... EXIT` 保证退出即恢复。
  - 本地前端生产构建必须显式加大堆：`NODE_OPTIONS=--max-old-space-size=6144 npm run build:online`，默认堆约 1.6GB 会在 vite transform 阶段 `FATAL ERROR: Ineffective mark-compacts near heap limit` OOM（退出码 134）。加参后约 2 分钟，产物 `dist/` 约 20MB。
  - compose 删服务块用 awk 从 `^  worker:` 扫到 `^networks:` 整段丢弃，比 YAML 库改写安全（PyYAML 1.1 会把 `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: '1'` 这类字符串数字解析成布尔并破坏原文件）。给 backend 加卷用 sed 锚定既有 `      - /opt/authyide/data/static:/app/static:rw` 追加。
  - `tar -xzf` 解压是**追加式**的，本地删掉的文件在服务器不会消失。ent 重新生成后 `db/appbuildjob*` 一整组文件会残留导致 `go build` 报 `no required module provides package .../db/appbuildjob`。正确做法：上传本地文件清单做 `comm -13` 差集删除，再 `find . -type d -empty -delete`。`frontend/dist` 同理，不先清空就直接解压会让旧 hash 命名产物一直留在磁盘并被 grep 命中，误判还有残留引用。
  - 远程命令里不要写 `pkill -f "bash /root/xxx.sh"`：`-f` 匹配整条命令行，当前 ssh 会话的命令字符串本身就含 `xxx.sh`，会杀掉自己的 shell，表现为 ssh 成功但后续命令无输出且退出码非 0。`pgrep -f xxx.sh` 同理永远返回 true；判断任务结束要用日志里的结束标记。
  - 部署脚本要在**首次修改 compose 之前**就设 `trap restore EXIT`，恢复动作本身要幂等（备份文件不存在时静默跳过）。分阶段脚本若 trap 设在中段，前面的失败享受不到恢复保护。`nohup bash xxx.sh > log 2>&1 &` 配合 `&&` 链式命令时，远程 ssh 可能因后台 job 未脱离而把整条链判为非 0，导致重试循环反复触发；把启动动作单独写成远程脚本文件执行更可靠。
  - 服务器上 `/opt/tools/flutter`、`/opt/tools/android-sdk`、`/opt/authyide/builds` 与 `/etc/profile.d/authyide_toolchains.sh` 已无消费者，可另行清理，不影响后端运行。

[ent 字段默认值与 migration 默认值是两套]
- Date: 2026-10-17
- Context: Agent 给 `users` 表加 `email_verified` 列做邮箱注册验证时发现
- Category: 排错调试|构建方法
- Instructions:
  - ent 的 `field.X(...).Default(...)` 只管后续 `Create()` 未显式赋值的新行，不会回填既有行；migration 的 `ALTER TABLE ... DEFAULT` 才决定既有行取值。两者必须各自独立设定，不要假设其中一个是权威。
  - 给存量表加「可信来源标记」类列（如 `email_verified`）时，正确组合是：ent 默认 `true`（绝大多数创建路径——团队建成员、OIDC、OAuth、Git Bot 任务用户——由可信管理员或身份源创建），migration 也写 `DEFAULT true`，然后只在唯一需要 `false` 的创建路径（邮箱注册）显式 `.SetEmailVerified(false)`。
  - 若把 ent 默认误设为 `false`，所有历史创建路径都会落成未验证，密码登录被 `PasswordLogin` 的全局校验拒掉，表现为「存量用户全部登不上」这类难定位的回归。全仓 `\.User\.Create\(` 排除 `*_test.go` 能列出全部创建点（本次 5 处：`biz/user/repo/user.go`、`biz/user/repo/oauth_login.go`、`biz/task/repo/gittask.go`、`biz/team/repo/user.go` 两处）。
  - `web.Err` 的 `code`/`id` 字段不可导出，无法按错误码比较；但 `errcode.Err*` 是包级单例指针，handler 里可直接 `err == errcode.ErrXxx` 做分支，用于把特定错误（如未验证邮箱）从「登录失败」的兜底掩码里挑出来给用户看。

[移动端打包路径与阻塞项]
- Date: 2026-10-18
- Context: Agent 在把 `mobile/` Expo 应用从 MonkeyCode 改为 AuthyIDE 品牌并准备打包时发现
- Category: 运维部署|构建方法
- Instructions:
  - 移动端是 Expo / React Native 应用（`mobile/`），此前的 Flutter 打包管线已整体删除，不要再往 `flutter build` 方向找。品牌配置集中在 `mobile/app.json`（`expo.name`/`slug`/`scheme`/`ios.bundleIdentifier`/`android.package`/`extra.updatesServer`/`updates.url`）与 `mobile/package.json` 的 `name`。
  - 默认服务器地址在 `mobile/src/api/client.ts` 的 `DEFAULT_BASE_URL`；支付宝 scheme 兜底在 `mobile/src/native/alipayAuth.ts`；OTA 更新通道走 `extra.updatesServer` 与 `updates.url`，指向发布静态目录而非接口域名。
  - 主题点缀色集中在 `mobile/src/theme.tsx` 的 `ACCENTS`，默认色由 `makeTheme(false, ...)`/`makeTheme(true, ...)`/`PrefsContext`/`useState` 四处硬编码，改默认色必须四处同步；用户可选的其余点缀色保留原样。
  - 登录页品牌与主视觉集中在 `mobile/app/login.tsx`：标题文案、邮箱 placeholder、`heroGreen`/`heroGreen2` 常量、`HeroBackground` 与 `LoginButtonBackground` 两个 SVG 渐变里的十六进制色值都要改。变量名可以不改，只换色值最安全。
  - 本执行环境原本没有任何 Android 工具链（无 `eas`、无 `adb`、无 `xcodebuild`、无 `java`、无 `android-sdk`），`mobile/node_modules` 也未安装。已全部补齐：`apt-get install -y --no-install-recommends openjdk-17-jdk-headless`（本机 Debian 12 走清华镜像）；Android SDK 装到 `/opt/android-sdk`（cmdline-tools 11076708 + platform-tools + platforms;android-35/36 + build-tools;35.0.0/36.0.0），`/opt/android-sdk/licenses` 已接受。
  - `sdkmanager --licenses` 必须用 `yes |` 管道喂无限 y：`echo "yes" |` 只输出一行，遇到 8 份 license 会全部跳过，随后 `sdkmanager --install` 报「license is not accepted」且静默不装任何包。
  - 移动端依赖用 `npm ci --no-audit --no-fund`（`mobile/.npmrc` 是 `legacy-peer-deps=true`），1059 个包约 747MB、2 分钟装完；`npx tsc --noEmit` 通过（0 错误）。
  - 原生工程用 `npx expo prebuild --platform android --no-install` 生成 `mobile/android/`，`local.properties` 需手写 `sdk.dir=/opt/android-sdk`；`gradle.properties` 的 `reactNativeArchitectures` 缩到 `arm64-v8a` 可显著缩短构建时间（默认四架构全编）。
  - `./gradlew assembleDebug` 出的包**不能独立装机**（依赖 Metro 推 JS，详见下方「debug 变体不可装机」一节），只能做本地联调。要给用户安装必须走 `assembleRelease`。上 Play Store 前必须把 `release` 的 `signingConfig signingConfigs.debug` 换成独立 keystore，`eas.json` 的 `serviceAccountKeyPath` 指向的 `google-service-account.json` 仓库里不存在。
  - Gradle 命令行必须显式 `export JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64` 与 `ANDROID_HOME=/opt/android-sdk`，后台终端不继承登录 shell 的环境变量。
  - 首个 debug 构建实测 38-40 分钟、约 770 个 task，瓶颈是 react-native-worklets / reanimated / expo-modules-core / nitro-modules 的 NDK 原生 C++ 编译。Gradle 会自动经 `sdkmanager` 拉取 NDK 27.1 与 CMake，无需手工安装。
  - 内存是硬约束：cgroup 3.89GiB 会让 Gradle daemon 在 Kotlin 编译期被 OOM kill（报 `Gradle build daemon disappeared unexpectedly`，daemon 日志里 `oom_killed: yes`）。须把终端 `memory_percent` 提到 70（5.45GiB），并在 `gradle.properties` 加 `org.gradle.parallel=false`、`org.gradle.workers.max=2`、`kotlin.daemon.jvmargs=-Xmx1536m -XX:MaxMetaspaceSize=384m`。调低并发比调高 `-Xmx` 更能压低峰值。
  - 磁盘只有 20G 且易满，`df` 在此环境不可靠（`stat -f` 与 `df` 都可能给出 0，实际仍可写入；用 `dd` 实测最准）。构建产物会把盘打满导致 `No space left on device` 与 `Failed to create parent directory`。可安全腾盘的对象：`go clean -cache`、`/tmp/go-build*`（Go 编译临时目录）、已解压过的 `/tmp/commandlinetools-*.zip`、未被 CMakeCache.txt 引用的 NDK 版本（当前构建只用 `ndk/27.1.12297006`，27.0 可删，2G）、`/root/.gradle/caches/9.0.0/generated-gradle-jars`、`/root/.cache`、`/root/.gradle/daemon/*/` 日志、`frontend/node_modules`（876MB，`npm ci` 2 分钟可还原）、`android/app/build/intermediates/cxx/Debug` 与 `android/app/.cxx/Debug`（debug 变体原生构建产物，与 release 的 `RelWithDebInfo` 目录互不影响）。不要删 `/root/.gradle/caches/modules-2`（1.7G 依赖缓存）和 `mobile/node_modules`。

  - 本执行环境的前端依赖用 **pnpm**（`frontend/pnpm-lock.yaml`），不是 npm——`npm ci` 会直接报 `The npm ci command can only install with an existing package-lock.json`。装依赖用 `pnpm install`，跑脚本 `pnpm run dev:online` / `pnpm run build:online`。
  - 环境的 `/usr/local/bin/pnpm` 是 corepack 垫片，指向 `/root/.cache/node/corepack/v1/pnpm/12.14.0/bin/pnpm.cjs`。`rm -rf /root/.cache` 会把这个垫片打穿（报 `Cannot find module .../pnpm.cjs`），且 `corepack prepare pnpm@12.14.0 --activate` 装出来的是 `bin/pnpm.mjs` 而非 `.cjs`，依旧不可用。修复：`npm install -g pnpm@12.14.0 --force`（`--force` 必需，否则报 `File exists: /usr/local/bin/pnpm`）。
  - pnpm 12 默认拦截依赖的 postinstall 脚本（`Ignored build scripts: esbuild, msw`），`pnpm rebuild` 同样被拦。但 esbuild 实际可用：二进制是 JS shim，真实二进制在 `node_modules/.pnpm/@esbuild+linux-x64@0.27.7`，直接跑 `node_modules/.pnpm/esbuild@0.27.7/node_modules/esbuild/bin/esbuild --version` 能正常输出，Vite 可正常启动，无需 approve-builds。
  - 前端 dev server 启动命令 `TARGET=http://38.39.30.160 pnpm run dev:online`（host/port 已写死在 vite.config.ts），起来约 3 秒，`curl http://127.0.0.1:11180/` 返回 200 即通；日志里的 `Cannot optimize dependency: buffer` 是无害警告。

[移动端 APK：debug 变体不可装机，必须 assembleRelease]
- Date: 2026-10-19
- Context: Agent 交付的 debug APK 装机后一直卡在启动页时定位
- Category: 构建方法|排错调试|运维部署
- Instructions:
  - **`assembleDebug` 的产物不能离线装机**：React Native gradle 插件的 `debuggableVariants` 默认 `["debug"]`，debug 变体会**跳过 JS bundle 打包**（`android/app/build.gradle` 里 `bundleCommand = "export:embed"` 只在非 debuggable 变体执行），APK 的 `assets/` 下只有 `app.config`、`expo-root.pem`、`fingerprint`，运行时依赖 Metro 推送 JS，连不上就永久停在启动页。校验方法：`unzip -l app.apk | grep -iE "\.bundle|_expo/static/js"`，为空即不可用。
  - 可装机产物必须走 `./gradlew assembleRelease --no-daemon`：release 不在 debuggableVariants 内会打 bundle，`android/app/build.gradle` 的 `release` 已复用 `signingConfigs.debug`（`androiddebugkey`/`android`），可直接侧载，无需独立 keystore；`enableMinifyInReleaseBuilds` 默认 false，所以 release 不会走 R8。
  - **`mobile/android/` 被 `mobile/.gitignore:12` 忽略，不在 git 里，克隆/同步都不会带上**。在服务器上构建前必须 `./node_modules/.bin/expo prebuild --platform android --no-install` 生成原生工程，再手写 `android/local.properties` 的 `sdk.dir=/opt/android-sdk`，再补 `android/gradle.properties` 的 `reactNativeArchitectures=arm64-v8a` 与 gradle/kotlin 内存参数（prebuild 会重置 gradle.properties，调优必须在 prebuild 之后做）。
  - 服务器 Android 构建环境：JDK 17（`/usr/bin/java`，openjdk 17.0.20.1）与 Node v20.20.2/npm 10.8.2 已预装；需自行装 `/opt/android-sdk`（cmdline-tools 11076708 + platform-tools + platforms;android-35/36 + build-tools;35.0.0/36.0.0），`yes | sdkmanager --licenses` 必须用管道喂无限 y。构建脚本 `/root/apkbuild-payload/sec-tools/apkbuild.sh`，日志 `/root/apkbuild-<ts>.console.log`，产物经 `cp` 覆盖 `/opt/authyide/data/static/authyide.apk`（覆盖前留 `.bak-<ts>`）。
  - 本机 20G 盘装不下 `assembleRelease`（`RelWithDebInfo` 与 `Debug` 两套原生产物叠加），且本机已删掉 `mobile/node_modules`；release 构建一律在服务器上做（72G 空盘、13.6G 可用内存、8 核）。
  - **gradle 输出绝不能管道到 `tail -N`**：`./gradlew assembleRelease 2>&1 | tail -40` 会把全部输出缓冲到 `tail`，gradle 结束前日志文件里除了步骤标题什么都没有，构建期间完全失明（实测 1h20m 零输出被误判为卡死）。要实时可见就重定向到独立文件 `> gradle-release.log 2>&1` 让监控直接 tail 该文件。
  - **只改 `app.json` / `src/` 时不必全量重编**：`expo prebuild` 是增量更新 `android/`（只重写受影响的 manifest 与资源），随后 `assembleRelease` 命中 gradle 缓存，NDK 原生 C++ 不重编，几分钟出包。所以「改移动端配置」的完整路径是：改 `app.json`/`src` → 同步到服务器 → `prebuild` → 重写 `local.properties` → 重新调优 `gradle.properties`（prebuild 会重置它）→ `assembleRelease` → 校验 → 覆盖静态目录。
  - 移动端后端地址的唯一权威来源是 `app.json` 的 `extra.baseUrl`（`src/api/client.ts` 从 `Constants.expoConfig.extra.baseUrl` 读取）；`expo-build-properties` 插件的 `android.usesCleartextTraffic` 决定是否放行明文 HTTP；`updates.enabled` 必须显式关闭，否则 OTA manifest 域名一旦可解析并被人抢占，APP 会拉取并执行第三方 JS（供应链风险）。改部署地址只动 `app.json`，不要改 `client.ts` 里的字面量。
  - 校验 APK 是否真的能连后端的三项检查：`unzip -l` 看 `assets/index.android.bundle` 是否存在；`unzip -p APK assets/app.config` 看 `extra.baseUrl`；`aapt2 dump xmltree --file AndroidManifest.xml APK | grep -iE 'usesCleartext|networkSecurity'` 看明文流量放行（Android 9+ 默认拦截明文 HTTP，此项缺失则 APP 装得上但连不上后端）。

[本机 Android 构建的失败模式与 Gradle 9 变化]
- Date: 2026-10-22
- Context: Agent 在本机尝试 `assembleDebug` 给真机测试包时发现
- Category: 构建方法|排错调试|环境配置
- Instructions:
  - 沙箱在不同会话间会重置，本机工具链必须先探再信：2026-10-22 复查时 `/opt/android-sdk`、`mobile/node_modules` 均不存在（本条上节记录的「已全部补齐」已失效），`docker` 也没有；只有 `/usr/lib/jvm/java-17-openjdk-amd64`（openjdk 17.0.20.1）、`~/.gradle`（1.7G `modules-2` 缓存保留）、`~/.npm` 幸存。重装成本：SDK 750M（cmdline-tools 11076708 + platform-tools + platforms;android-35/36 + build-tools;35.0.0/36.0.0）+ `npm ci` 1059 包 / 747M / 2 分钟。
  - **本机根分区 20G 极易打满**（实测 19G 已用、480M 可用，`df` 在满盘时也会给出可读数字）。`assembleRelease` 一定放服务器。临时腾盘只能靠 `mv` 挪走自己放的缓存（`rm` 受禁止删除约束），`/dev/shm` 的 1.6G Go 缓存（`/dev/shm/go-tmp/gocache`）腾不出、也不可搬到根盘。
  - Gradle 9.0.0 **仍支持** `--no-daemon`（`./gradlew --help | grep -- "--no-daemon"` 可确认），`org.gradle.daemon=false` 已不是合法属性，不要写进 `gradle.properties`。
  - `android.enableR8=false` 在 AGP 7.0 已被移除，写在 `gradle.properties` 会让整个 `:app` 配置直接失败（`Failed to apply plugin 'com.android.internal.application' > The option 'android.enableR8' is deprecated. It was removed in version 7.0`）。release 默认不走 R8，无需该属性。
  - 把工作区放到 `/dev/shm` 只能把 `android/build`、`app/build` 移出根盘；`settings.gradle` 的 `includeBuild` 会到 `node_modules` 里编译 expo / react-native 的 gradle 插件，产物落在 `node_modules` 内（本次在根盘），所以 `/dev/shm` 方案对根盘压力只是部分缓解。
  - `cp -al` 跨文件系统（根盘 → `/dev/shm` tmpfs）直接报 `Invalid cross-device link`；要把热缓存「搬到」tmpfs 又复用原数据，只能用 `ln -s` 软链（`ln -sfn` 在目标已存在为目录时会先 `mv` 走旧目录再建链）。
   - Expo/RN gradle 插件编译阶段约 6 分钟、32 个 task，是第一次配置的成本；同参数重试会 UP-TO-DATE，所以调试 `gradle.properties` 时先看是否已有缓存，别急着换方案。

[release APK 未签名：prebuild 会把签名配置冲掉]
- Date: 2026-10-23
- Category: 构建方法|排错调试|运维部署
- Instructions:
  - **`./gradlew assembleRelease` 出来的 APK 可能是无签名的，装上就会失败**。`expo prebuild` 每次都用 RN 模板重写 `android/app/build.gradle`，而模板的 `release` buildType 里只有 `signingConfigs.debug` 这一行会被 prebuild 冲掉；缺了它 gradle 不会报错，直接产出「看起来完全正常」的 APK：有 `assets/index.android.bundle`、有 `assets/app.config`、有 4 个 dex、大小与上一版一致，唯独没有签名。必须校验签名而不是只校验 bundle。
  - **构建脚本里两处兜底都建议保留**：prebuild 之后把 `signingConfig signingConfigs.debug` 补回 `release {` 块（`awk` 幂等插入，同时断言 `android/app/debug.keystore` 存在）；`assembleRelease` 之后若检测不到签名，就用 `zipalign -p -f 4` + `apksigner sign --v1/v2/v3` 加一个自建 keystore（`keytool -genkeypair` 生成，`/root/apk-side-load.keystore`）重签。
  - **判签名的唯一可靠方式是 `apksigner verify`**，不要自己扫字节。APK Signing Block 的魔数在 build-tools 36.0.0 产物里是 `APK Sig Block 42` 后跟 **`\x00` 而不是 `\n`**，用 `b'APK Sig Block 42\n'` 搜会得到 false negative。判法：`apksigner verify -v APK | grep -E "^Verifies|Verified using v"`，看到 `Verifies` 与 `v2: true` 即可（minSdk 24 只需 v2）。同时确认 `v1 scheme` 状态——`--v1-signing-enabled true` 在 build-tools 36.0.0 下仍可能报 `v1: false` 却在 META-INF 里留下 `.SF/.RSA`，两者不一致时以 verify 输出为准。
  - **构建工作目录是 `android/`，不是项目根**：`gradlew` 在 `android/gradlew`。从 `mobile/` 根直接 `./gradlew assembleRelease` 会报 `./gradlew: No such file or directory` 并被误判为 prebuild 失败——真正的失败点其实是 cwd。
  - **发布完成可以用 HTTP 探测，不必烧 SSH**：`curl -D - -o /dev/null http://38.39.30.160/static/authyide.apk` 读 `Last-Modified`，发布一发生该头立刻变成当前时间；HEAD 在该 nginx 上返回 405，必须用 GET（或 `-r 0-2047` 范围请求）取响应头。构建长跑期间用它轮询比 SSH 取日志安全得多。
  - 产物入库位置是 `Artifacts/`（`.gitignore:14` 已忽略该目录，已有 `app-debug.apk`），例如 `Artifacts/authyide-android.apk`，用户可从文件面板直接下载，不必等 SSH 把文件推上服务器。


[正确性属性 1/2/5 的单测落点]
- Date: 2026-10-22
- Context: Agent 为会话源码打包补正确性属性验证时写入
- Category: 测试方法
- Instructions:
  - `backend/biz/mobilebuild/runner/overlay_test.go` 现有 `fakeObjectSource`（`GetObject` 返回单个 tar.gz body）与 `overlaySourceRunner`，可在不接对象存储的情况下驱动 `overlaySource` + `rewriteConfig`；`domain.SourceSnapshot.SHA256` 传空串即跳过摘要校验，传错值即触发 fail-closed 分支。
  - 属性 1（品牌优先）用「先 `overlaySource` 再 `rewriteConfig`」的顺序测试表达，断言 `app.json` 六个字段全部来自 `Config`；属性 2（模板独有文件存活）断言 `android/gradlew` 与 `plugins/withAlipayLogin/index.ts` 覆盖后仍在；属性 5（fail-closed）用 `listFiles` 前后对比断言工作区逐字节未变。
  - `Runner` 的 `logger` 字段不可为 nil（`NewRunner` 里 `logger.With(...)`），测试用 `slog.New(slog.DiscardHandler)`。
  - 跑单个包：`cd backend && GOTMPDIR=/dev/shm/go-tmp GOCACHE=/dev/shm/go-tmp/gocache CGO_ENABLED=0 go test -p 1 ./biz/mobilebuild/runner/`（约 0.2s）。

[后端邮件与 OAuth 的 base_url 硬依赖]
- Date: 2026-10-19
- Context: Agent 放开登录页「注册」「忘记密码」offline 门禁后发现
- Category: 环境配置|排错调试|运维部署
- Instructions:
  - `MCAI_SERVER_BASE_URL` 是整个外部链接生成的唯一来源，compose 里写法是 `http://${REMOTE_IP}:${NGINX_PORT:-80}`；`REMOTE_IP` 未设置时值变成 `http://:80`，注册验证邮件链接（`biz/user/usecase/user.go:132` 的 `{BaseURL}/api/v1/users/register/verify?token=`）与重置密码邮件链接（`user.go:218` 的 `{BaseURL}/resetpassword?token=`）会拼成坏链接，收件人点开即 404。放开注册/忘记密码门禁后此项必须正确，默认值是空串（`config/config.go:338`）。
  - 判定方法：`docker inspect <backend容器> --format '{{range .Config.Env}}{{println .}}{{end}}' | grep MCAI_SERVER_BASE_URL`，拿到的是运行中的真实值，比看 compose 文件可靠。同类依赖 base_url 的还有 `MCAI_LLM_PROXY_BASE_URL`、`MCAI_OBJECT_STORAGE_ACCESS_ENDPOINT`、`MCAI_TEMPLATE_PROJECT_ZIP_URL`、`MCAI_JWT_RELAY_HOST`、`RELAY_DOMAIN`。
  - 前端登录页平台登录按钮的渲染条件是后端 `GET /api/v1/users/oauth/providers` 返回的数组非空（`login.tsx` 的 `oauthProviders.length > 0`），`oauthFallbackProviders` 只在 fetch 抛错时兜底，空数组不触发。所以凭据没配时按钮不显示是预期行为，不是前端 bug；配好凭据后自动出现，无需前端改动。
  - `providerEnabled`（`biz/user/usecase/oauth_login.go:263-278`）要求同时满足 `enabled=true` 且 `client_id`/`client_secret` 非空；compose 默认 `wechat/qq/douyin.enabled=true` 但凭据空、`google/github.enabled=false`，所以 google/github 即使给了凭据也要显式把 `enabled` 改成 true。OAuth 回调地址由后端用 `{Server.BaseURL}/api/v1/users/oauth/{平台}/callback` 拼出，同样受 base_url 影响。
  - 后端配置全部走 `MCAI_*` 注入（viper `AutomaticEnv` + `SetEnvPrefix("MCAI")` + `SetEnvKeyReplacer(".", "_")`），改 `server.base_url` 对应的环境变量名是 `MCAI_SERVER_BASE_URL`。

[用户指令摘要]
- Date: 2026-10-18
- Context: 用户要求把服务器明文凭据改为加密存储，并纠正部署方式
- Category: 工作流与协作|运维部署
- Instructions:
  - 所有密码类凭据（数据库口令、ClickHouse 口令、对象存储 access key secret、SMTP 授权码、管理员初始口令、TaskFlow 回调令牌）一律加密存储，禁止明文写进 `docker-compose.yml` 或任何会被同步/备份/查看的配置文件。
  - 禁止「本地构建产物 + 打包 tarball + SSH 上传」的部署方式；构建、加密、配置修改、部署全部在目标服务器上直接执行。
  - **不做本地预览部署**（2026-10-22 用户重申）：不要起前端 dev server、不要跑 `npm run dev` / `pnpm run dev:online` 之类的本地服务来给预览链接，也不要调用 `deploy-website` skill。所有改动直接在 38.39.30.160 上构建与部署，验收通过 SSH 在服务器上 `curl` 站点完成。
  - 每次改动后必须核验服务端实际运行版本，不能只看部署脚本退出码：`docker image inspect --format '{{.Created}}'` 对比源码 mtime，前端按文件 sha256 逐一对比，避免缓存命中导致旧镜像/旧 dist 继续运行。
  - 服务器现有工具链：Node v20.20.2、pnpm 9.12.3、Docker 29.8.2（legacy builder）、无 Go（Go 只在镜像内编译）、有 python3、有 openssl；`age` 未安装。
  - 服务器宿主机 RAM 是 **15.6GiB**（`free -m`），3.89GiB 那档是本机终端 cgroup 上限，不是主机内存；服务器上 `NODE_OPTIONS=--max-old-space-size=10929` 的 Vite 构建 3m1s 跑通，`tsc -b` 0 错误。
  - **部署必须同步源码而不是只同步产物，且必须覆盖所有源码目录**：早前只送过 frontend 的 dist、backend 源码与 APK 产物，导致服务器 `frontend/src` 停留在 2026-10-17（`pages/docs.tsx` 根本不存在）、`frontend` 根配置与 `mobile/src` 完全未同步（`mobile/` 自 2026-10-15 克隆后零改动）。服务器源码在 `/opt/authyide/src/{frontend,backend,mobile}`；前端构建用 `node /root/.cache/node/corepack/v1/pnpm/9.12.3/bin/pnpm.cjs exec tsc -b` 然后 `VITE_APP_EDITION=offline node ... exec vite build --mode offline`。
  - **核对部署完整性要用 sha256 清单 diff，不能用文件数量**：数量能对上不代表内容对得上（`tar -xzf` 是追加式，服务器会残留旧文件）。本地与服务器分别 `find ... -print0 | sort -z | xargs -0 sha256sum` 生成清单再 diff，区分「仅本地有 / 仅服务器有 / 同路径 hash 不同」三类。已知无害差异：`backend/build/Dockerfile.legacy` 服务器独有（绕开 buildx 时创建，该保留），`backend/_ditest/main.go` 仅本地有（下划线目录 Go 工具链忽略）。
  - **SSH 限流窗口内尽量一次性做完**：探针连接本身也会重新计时冷却，成功后不要立刻再开新连接，把上传、解压、后台启动合在同一条 SSH 会话里；ops 脚本用 `nohup bash ... > log 2>&1 &` 脱离，再用下一次连接取日志。
  - **定时轮询本身也会把限流打穿**：每 5 分钟一次 SSH 的后台监控，在构建长跑期间会先正常数次然后整段被 `kex_exchange_identification` 拒绝（监控与新发连接同时被掐）。被拒之后不要继续用探针试连通性——每次失败都会重置冷却计时。正确做法是**先杀掉轮询终端**，再整体静默 20 分钟以上，然后一次性做完「查状态 + 下发 + 启动」；本地等待脚本自己负责把两次 SSH 调用间隔卡在 300s 以上。
  - **base64 内嵌进 SSH 命令行有硬上限，实测会撞 `Argument list too long`**：`sshpass`/`execve` 在 `ARG_MAX`（本机 2097152）上整体核算 argv+env，400KB 原始文件（base64 后 ~540KB）已无法内嵌，且失败发生在网络握手前（exit 126），不会计入服务器限流窗口。正确做法是 **tar 管道走 SSH stdin**：`tar -czf - -C <暂存目录> . | ssh root@host 'mkdir -p <远端> && tar -xzf - -C <远端>'`——数据走 stdin、argv 极短，同一条连接内即可上传。**注意 `tar` 侧会报 `Child returned status 141`（SIGPIPE）**，那是 ssh 失败后的连带现象，不是 tar 本身的错；判定要看 ssh 的返回码与远端标记字符串。
  - 上传与触发分两条连接（限流要求间隔 >= 300s）：连接 1 上传暂存目录，连接 2 就地安装后 `nohup` 触发部署。`cp -af` 是覆盖式不删除，符合禁止删除的约束。
  - **远端触发部署要 `ssh 'bash -s'` 喂脚本，别把 `nohup ... &` 拼在 `&&` 链末尾**：`A && B && nohup X &` 会被解析成 `(A && B && nohup X) &`，整条链全部后台化，而该链持有 SSH 会话的 stdout 管道，于是 `ssh` 永不返回、本地脚本一直挂着（实测挂满 10 分钟无输出）。远端脚本改用 `printf '%s\n' "$SCRIPT" | ssh 'bash -s'` 传入，nohup 的 stdout/stderr/stdin 三个 fd 全部重定向后子进程才真正脱离会话。
  - **i18n 值改动不触发 TS 错误但会让暂存目录过期**：已上传到服务器的暂存目录不会自动同步，改完文案必须重传，否则服务器部署的还是旧版。幂等触发脚本用 `find + cmp` 逐个对比暂存与已安装文件来判断是否需要重跑，只比单个文件会被漏判。
  - **`docker build` 命中缓存不算重建**：全部 step `Using cache` 时镜像 ID 与 `Created` 时间戳都不变，日志里 `Successfully built <同一个ID>` 极易误判为已重建。判断是否真重建要对比 `docker image inspect --format '{{.Created}}'` 与源码 mtime，必要时 `docker build --no-cache`。
  - AuthyIDE 密钥金库：明文密钥全在 `/opt/authyide/secrets.env.enc`（openssl aes-256-cbc + pbkdf2 -iter 200000），主密码 `/root/.authyide-master.key`（chmod 600，随机 hex）；`docker-compose.yml` 里所有密钥已换成 `${VAR}` 占位（含 redis `command` 的 `--requirepass` 与 `healthcheck` 里的 `redis-cli -a`），`CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: '1'` 这类数字必须保留单引号。启动统一走 `/usr/local/bin/authyide-up`（解密到 `/run` 临时文件再 `docker compose --env-file`，退出即删），直接跑 `docker compose up` 会把密钥解析成空串。重构脚本在 `/root/sec-tools/sec.py`，日志在 `/root/ops2-*.log`。
  - `MCAI_LOGGER_LEVEL` 必须是 `info`：`debug` 时启动日志的 `print config` 会把 SMTP 授权码、Postgres/ClickHouse 口令、对象存储 secret、管理员初始口令全部明文打进 `docker logs`，任何能读日志的人都能拿到。改后用 `docker logs --since 4m authyide-backend | grep -c '"level":"DEBUG"'` 验证为 0。
  - `/opt/authyide/stack/creds.env`（chmod 600，不被 compose 引用）仍存明文口令，但它的每个值都已进金库，删除前需用户拍板。
  - 服务器容量：`/dev/vda1` 97G、内存 15.6G。已做过一次 `docker container prune -f` + `docker image prune -f`（清掉 38 个 dangling 镜像与 17 个已退出容器，回收 32.22G），当前已用 26G、剩 72G。清理顺序必须先容器后镜像（部分 dangling 镜像被已退出容器占用，镜像 prune 会拒删）；不要 `docker system prune -f`（会顺带清未使用网络）。残留 2.4G 标记可回收的是 `node:20`/`node:20-slim`/`node:20-alpine`/`golang` 四个构建基底，构建后端与前端镜像还要用，不要删。本机 20G 盘跑不完的 `assembleRelease`（需 ≥40G）在服务器上是可以跑的，前提是先在服务器上装 JDK17 与 Android SDK。

[前端与移动端品牌替换]
- Date: 2026-10-19
- Context: 按用户要求把移动端猴子 logo 换成熊头、欢迎页去云版 SaaS 营销内容、模型显示名中性化
- Category: 工作流与协作
- Instructions:
  - 移动端**可见**品牌只剩 `app/(tabs)/profile.tsx` 三处：`ABOUT_LINKS`（官网/文档/GitHub）、App 名称与副标题、「Building with X」页脚。`src/config.ts` 的 `MonkeyCodeOfficialPlugins`、`src/api/baizhi.ts` 的 `getMonkeyCodeBaizhiLoginUrl`、`src/client.ts` 的 `mc.baseUrl`、`src/updates/useOtaUpdate.ts` 等属运行时标识，不能改。
  - logo 替换只需覆盖 `mobile/assets/logo-light.png` 与 `logo-dark.png`；这两个文件**未被 app.json 引用**（只走 Metro JS bundle），所以改完不必跑 `expo prebuild`，直接 `assembleRelease` 即可。app.json 已显式设 `android.package` 与 `scheme`，故 Expo 插件里的 `packageName`/scheme fallback 从不触发，改 fallback 值不会影响原生产物。
  - i18n 里给徽标赋**空字符串**即可让徽标整块不渲染：`model-select.tsx:276/310` 与 `task-detail.tsx:890/924` 都是 `group.badge ? <Badge>…</Badge> : null`，空串为 falsy。这是去掉「Pro 会员免费」这类 SaaS 措辞又不留空胶囊的最简改法。
  - 模型分档显示名保留「基础模型 / 专业模型 / 旗舰模型」：用户要求「只改显名」指的是去掉 MonkeyCode 品牌字样，而不是改掉分档本身，`基础/专业/旗舰` 对应后端 `monkeycode-basic/-pro/-ultra` 三个能力档位，属中性描述。真正 SaaS 专属的是「会员」二字——已全局改成「版」（基础版/进阶版/高级版），`cn.ts` 现零处「会员」、`en.ts` 零处 Membership。
  - 前端 `common.tsx` 的 `getModelDisplayName` 兜底分支原本直接返回原始 `modelName`，第三方模型名会带出 `monkeycode-<pkg>/` 公共包前缀；改用同文件已有的 `stripBuiltinPublicModelPackagePrefix()` 即可，模型 ID 一律不动。
  - `frontend/src/components/welcome/downloads.tsx` 是零引用孤儿（全仓库无 import），内含上游 App Store 直链，不会进 bundle；删除前需用户拍板。
  - `frontend/src/pages/self-hosting.tsx` 两条安装命令指向 `authyide.app` 与 `release.authyide.app`，`getent hosts` 实测两个域名均无解析（`monkeycode-ai.com` 反而能解析到 118.31.65.100），是死链；改成什么属产品决策，需用户确认。

[移动端 APK 产物校验方法]
- Date: 2026-10-19
- Context: 部署脚本用 base64 前缀匹配判断新 logo 是否进包，结果假阴性导致构建成功的 APK 被判定失败而跳过部署
- Category: 构建方法|排障与调试
- Instructions:
  - **APK 里的图片是哈希命名的 `res/XX.png`，不在 JS bundle 内**。对 `assets/index.android.bundle` 做 base64 匹配必然是假阴性（实测 bundle 里 `data:image/png;base64` 仅 1 处、PNG 魔数 `iVBORw0KGgo` 0 处，209 个 png 全在 `res/` 下）。正确做法是 `unzip -o -q APK 'res/*.png' -d /tmp/x` 后 `find /tmp/x -name '*.png' -print0 | xargs -0 sha256sum` 再比对。
  - 资源名由 `assets/app.manifest` 的 `packagerHash` 派生：`logo-light` → `packagerHash f224257504dc938be822a15cfbe93790` → `res/EH.png`，与 `resourcesFilename` 无关，APK 里也不存在 `res/drawable*/assets_logolight.png`。资源内容确定性：同一 logo 源文件每次构建产出的 `res/EH.png` sha256 恒为 `d09de6206cda14d8c61fb3d6a02b668587d2b49fbb2a82d323542`（60231 字节源 → 59835 字节产物，Expo 重编码但不改像素），旧猴子 logo 对应 `532a7558c94843a4fc176e84e35190e033534dc5d815bdc25b5d465110b5c774`。
  - 比对新旧 APK 只需 diff 两份 `res/*.png` sha256 清单，209 个 png 中只有 1 条差异即证明只有 logo 变了，比逐个字段断言更省。服务器无 PIL/identify/pngcheck，`python3 -c "import PIL"` 直接 ImportError，所以只能用 sha256 判像素；本地有 PIL 时可用 `Image.open().tobytes()` 的 sha256 验证像素级一致。
  - **`gradlew` 在 `$MO/android/` 下，不在 `$MO/`**。部署脚本 `cd "$MO"` 后直接跑 `./gradlew` 会 `exit=127` / `No such file or directory`，127 极易被误读成 JDK 或 SDK 缺失，实际只是路径错；同类问题还有资源检查要写 `$MO/assets/logo-light.png`（绝对路径）而不是 `assets/logo-light.png`。
  - **远端观察脚本必须三态判定**：有 done 标记=结束、有 `FATAL`/`跳过部署`=失败、既无 MARK_RUNNING 又无 done=未知需人工介入。只 grep 成功标记的监控实测把一次 gradle 失败连续误报为 `STILL_RUNNING` 超过 2 小时。
  - 品牌残留要区分可见与功能性：`https://monkeycode-ai.com`（登录页 TextInput placeholder）与 `https://monkeycode.docs.baizhi.cloud/...`（Git token 「如何获取」链接）属用户可见死链，前者改中性占位 `https://example.com`，后者改自指当前实例 `` `${getBaseUrl().replace(/\/+$/,'')}/docs#gitbot` ``（前端 `App.tsx` 确有 `/docs` 路由、`docs.tsx` 确有 `id="gitbot"` 锚点）。`src/git.ts:47` 的 `getBaseUrl() === 'https://monkeycode-ai.com'` 是功能分支，决定 GitHub App 安装地址在 `apps/monkeycode-ai` 与 `apps/mcai-dev-nb` 之间切换，改它需要确认 AuthyIDE 自己的 GitHub App slug，属产品决策。`monkeycode-basic/-pro/-ultra` 模型 ID 前缀、`monkeycode_ai_session` cookie 名一律不动。

[服务端 APP 打包（mobilebuild）构建与部署]
- Date: 2026-10-20
- Context: 为「移动端 APP 在线打包」功能在 38.39.30.160 上接线后端镜像、compose 挂载/env 与构建工作区
- Category: 构建方法|运维与部署
- Instructions:
  - **服务器已装好主机侧工具链，不需要再装**：`/opt/android-sdk`（build-tools 35.0.0/36.0.0、ndk 27.0.12077973/27.1.12297006、platforms android-35/android-36）、`/usr/bin/java`（openjdk 17.0.20.1）、Node v20.20.2。磁盘 97G/剩约 58G、内存 15.6G/可用约 13.5G、**无 swap**。
  - **服务器没有 `docker buildx`**（`docker buildx` 直接 unknown command），`build/Dockerfile` 里的 `--platform=$BUILDPLATFORM` / `--mount=type=cache` 一律被拒。部署必须用 `build/Dockerfile.legacy`（`Artifacts/deploy/deploy.sh` 第 6 步就是这么调的），语法只用 `FROM`/`WORKDIR`/`ENV`/`COPY`/`RUN`/`EXPOSE`/`CMD`，构建命令是 `docker build --network=host -f Dockerfile.legacy -t <image> ..`，在 `build/` 目录下执行。
  - **后端运行时镜像必须是 Debian，不能是 alpine**：gradle 在容器内跑，而挂载进来的 `/usr/lib/jvm/java-17-openjdk-amd64` 与 Android SDK 主机工具都是 glibc 构建，musl 会 `error while loading shared libraries`。方案：builder 阶段 `golang:1.25.8-alpine3.23`，最终阶段 `node:20-bookworm-slim`（服务器本地已有镜像，293MB）+ `apt-get install -y --no-install-recommends openjdk-17-jdk-headless tzdata`；保留 node 是因为 `expo prebuild` 从挂载工作区的 `node_modules` 解析。JDK 与 SDK **不从镜像烘焙、走挂载**，否则每次改后端代码重建镜像都要重装 apt 包。
  - **gradle 必须能在容器内直接跑**，验证方式是镜像建好后 `docker run --rm <image> bash -c 'java -version; node -v'` 能输出。**不要**在容器里跑宿主的 `/usr/bin/java` 路径以外的 JVM；`JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64` 必须与镜像内实际路径一致。
  - **打包需要的 3 个新挂载 + 1 个改模式**（`authyide-backend`，容器内 root）：`/opt/authyide/src/mobile:/opt/authyide/src/mobile:rw`、`/opt/android-sdk:/opt/android-sdk:ro`、`/opt/authyide/data/gradle:/root/.gradle:rw`（gradle 缓存持久化，可省 30 分钟）；**必须把已有的 `/opt/authyide/data/static:/app/static:ro` 改成 `:rw`**——发布原生版本时后端要把 APK 和更新清单写进 `/app/static`，`:ro` 会让发布静默失败。构建期数据目录另建 `/opt/authyide/data/mobile-assets`、`/opt/authyide/data/build-logs`（后端读，不挂也行，但挂了才能在宿主机看日志）。
  - **`cp -al` 的硬链接只隔离新建文件，不隔离原地写入**：`workspaceFor` 用 `cp -al` 给每个用户任务复制模板工作区，目录是副本、文件共享 inode。`os.WriteFile` 是原地截断、作用于共享 inode，等于用户任务直接改写模板本身。已改造成 `atomicWriteFile`（同目录临时文件 + `os.Rename`）：rename 只替换目录项、不动 inode，模板保持不变，构建中断也不会留下半截 `app.json`。runner 内写文件的唯一入口就是它，新增写入点必须复用，不要直接 `os.WriteFile`。**为什么不能反过来给 admin 也用隔离副本**：R7 AC1 要求 admin 沿用现有工作区，且 `cp -al` 下 gradle 与 `npm ci` 会生成新文件而不影响模板，隔离副本反而违背 AC1。
  - **runner 的产物路径契约**（改前端/移动端前必读）：`expo prebuild --platform android --no-install` 在 `<workspace>` 下跑，产物 APK 恒为 `<workspace>/android/app/build/outputs/apk/release/app-release.apk`，构建命令是 `cd <workspace>/android && ./gradlew assembleRelease --no-daemon`；prebuild 会重置 `android/gradle.properties`，所以调参（`reactNativeArchitectures=arm64-v8a`、`org.gradle.workers.max=2`、`kotlin.daemon.jvmargs=-Xmx1536m`）必须在 prebuild **之后**做。runner 还会写 `android/local.properties` 的 `sdk.dir`。
  - **原生更新链路**：后端把 `{version, url}` 原子写入 `<static_files.dir>/app-version/android.json`，`url` = `<MCAI_SERVER_BASE_URL>/static/mobile/<version>/authyide.apk`；`MCAI_SERVER_BASE_URL="http://38.39.30.160:80"`，nginx `/etc/nginx/conf.d/authyide.conf` 的 `location /static/` 已代理到 `127.0.0.1:8888`。移动端 `checkAppUpdate()` 读 `app.json` 的 `extra.updatesServer` 拼 `/app-version/android.json`，`APP_VERSION_RE = /^\d+$/`（任意位数字，后端 `^\d{6,8}$` 是更严的兼容子集）。
  - **OTA（JS 热更新）在本部署不可用且必须保持关闭**：`mobile/app.json` 是 `updates.enabled: false`，expo-updates 需要 HTTPS + EAS 签名 manifest，两者都没有。runner 的 rewrite-config 步因此**始终写 `updates.enabled: false` 并删掉 `updates.url`**，只写 `extra.updatesServer` 供原生清单查询；给构建产物设置一个不可达的 `updates.url` 会让 expo-updates 在启动时抓 manifest 失败。后端 `MCAI_MOBILE_BUILD_OTA_ENABLED=false`，前端已移除 OTA 发布入口。
  - **部署脚本与 SSH**：`Artifacts/deploy-appbuilder/deploy.sh`（服务器上跑，10 步：patch compose → 同步 backend/mobile/frontend → 建数据目录 → build 镜像 → recreate → health check）、`Artifacts/deploy-appbuilder/compose_patch.py`（幂等纯文本 patch，**禁止 YAML round-trip**：`CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: '1'` 会被 PyYAML 折叠成布尔）。payload 打包后体积很小（backend-src.tgz 2.0M、mobile-src.tgz 2.1M、frontend-dist.tgz 10.6M）。
  - **SSH 调用脚本必须显式 `export SSHPASS`**：只 `eval "SSHPASS=..."`（不带 export）时变量留在父 shell，子进程脚本里的 `sshpass -e` 会报 `SSHPASS environment variable not set`。`/tmp/opencode/deploy-appbuilder/run-remote.sh` 已修。
  - **SSH 限流是情景性的**，症状是 `kex_exchange_identification: Connection closed by remote host`；重试循环间隔 130s、通常 2-4 次内成功，单次尝试可能第 1 次就通。整段上传+部署要包在同一个重试单元里，否则中途断开会留下半部署状态（好在 patch compose 幂等、backend 同步是 `rm -rf` 后重解包，可安全重跑）。
  - **前端本仓库没有 `test` script**：类型检查跑 `./node_modules/.bin/tsc -b`（`pnpm` 在 `/usr/local/bin/pnpm`，`node_modules/.bin/tsc` 比走 corepack 更稳），测试跑 `node --test test/*.test.ts`（Node 自带 TS 类型剥离）；`build:offline` = `tsc -b && vite build --mode offline`，完整构建约 2m31s、产物 433 个文件。i18n 叶子键一致性由 `test/i18n-resources-completeness.test.ts` 强制，增删 key 必须 en/cn 同步。
  - **`node --test test/*.test.ts` 的既有基线是 262 例 / 228 pass / 34 fail**（2026-10-20 实测）。34 个失败全是 AuthyIDE 改名遗留的陈旧断言：`document-meta`、`console-tasks-i18n`、`task-detail-preview-publish`、`welcome-shell-i18n`、`welcome-home-i18n`、`terminal-native-i18n`、`member-seat`、`skill-package` 等仍断言 `MonkeyCode 智能任务`、`长亭科技`、`CyberServal`、`SafeLine WAF`、`CyberServal Co., Limited` 这些字符串，而 `src/` 全库已零处残留（`grep -rl 长亭 src/` 无结果）。判定改动是否引入回归要对比这 34 个失败清单是否变化，不要去修它们。
  - **只改挂载/env 时走增量部署，不要跑完整 `deploy.sh`**：完整脚本会 `rm -rf` 重解包 backend/mobile/frontend 并重编镜像（约 7 分钟），只为加几个 mount 是浪费。增量脚本 `/tmp/opencode/deploy-appbuilder/30-desktop-sync.sh` 六步：备份 compose → `compose_patch.py` → 建目录 + 工具链探测 → `docker compose -p authyide up -d --force-recreate backend` → 等 55s → 健康检查。挂载在容器启动即生效，后端不会因新增未知路径崩溃（`MkdirAll` 只在 runner 实际执行时才调用）。
  - **`compose_patch.py` 的 `BUILD_TARGETS` 是归一化白名单**：容器内路径命中该列表的既有挂载会被丢弃后用规范值重写（`:ro`→`:rw` 就是这么生效的），未命中的外部挂载原样保留（服务器上已有 `/opt/authyide/src/packer:/opt/authyide/src/packer:ro` 未被破坏）。新增挂载时若容器路径会与既有挂载冲突，必须同时把它加进该列表。
  - **服务器已有桌面工作区但无桌面工具链**：`/opt/authyide/src/desktop`（819 条目，Oct 15 同步，早于本机最新版本）、`/opt/authyide/src/browser-extension`、`/opt/authyide/src/plugins/skills`（29 条目）均已存在；`/opt/authyide/src/agent` 为空（0 条目），引擎缺失与本机一致。宿主 `cargo`/`rustc`/`go`/`patchelf`/`rpm` 全部 MISSING，只有 `node`/`npm`——桌面构建目前只能在 workspace 机器上做，服务器侧只能出移动端 APK。
  - **本次 SSH 第 4 次重试才通**，与之前几次一致；上传与部署必须包在同一个重试单元（`run-remote.sh` 包住 `30-run.sh`），否则中途断开会留下半部署状态。
  - **服务器上的 payload 不会自动跟随本地源码更新，会静默回滚**：`/root/deploy-appbuilder/*.tgz` 是上次部署时的快照。改完本地后端源码后必须重新打包并上传，否则下次跑 `deploy.sh` 会用旧 payload 把后端回滚到没有修复的版本（本次就是旧包缺 `migration/000032` 与 `worker.NewWorker` 的 Provide，跑一次会回到 crash loop）。重打命令：`tar -czf backend-src.tgz backend`（根为 `backend/`）、`tar -czf mobile-src.tgz --exclude=node_modules --exclude=.expo --exclude=android --exclude=.gitignore --exclude=ota-server --exclude=package-lock.json -C mobile .`（根为 mobile 顶层，`android/` 由 runner 的 `expo prebuild` 生成，不能同步）、`tar -czf frontend-dist.tgz -C frontend dist`。上传前先在服务器 `cp *.tgz *.tgz.bak-<TS>` 留退路。
   - **`deploy.sh` 第 8 步的凭据掩码原先锚在引号上，对 compose 的无引号标量永不匹配**，日志里会打出 `MCAI_CLICKHOUSE_PASSWORD`、`MCAI_REDIS_PASS`、`MCAI_DATABASE_MASTER` 的明文。已改为对整行值做掩码：`sed -E 's/^([[:space:]]*-[[:space:]]*[A-Za-z0-9_]+):.*/\1: <masked>/'`。改这类脱敏逻辑后要把本地 `.out` 日志一并扫一遍（`grep -rlE 'MCAI_(DATABASE_MASTER|CLICKHOUSE_PASSWORD|REDIS_PASS)'` + 同一条 sed）。
   - **只改代码不改 compose 时走三阶段增量部署，不要跑完整 `deploy.sh`**：`/tmp/opencode/deploy-appbuilder/101-sync.sh`（tar 流式同步 backend/ + frontend/ 到 `/root/stage-appuser`，不落地本地 tarball，自带 6 次重试）、`102-run.sh` → `102-backend-build.sh`（备份旧 backend → 装新源码 → `Dockerfile.legacy` 重建镜像 → `authyide-up up -d --force-recreate backend` → 等 55s → 健康检查）、`103-run.sh` → `103-frontend-build.sh`（装 src/test/scripts → `pnpm exec tsc -b` → `VITE_APP_EDITION=offline pnpm exec vite build --mode offline`）。102/103 的 driver 各自先 hold 300s 冷却再连，避免限流。
   - **同步正确性靠双向 sha256 逐行比对**：sync 脚本先打本地指纹，远程解压后再打一遍同清单的指纹，肉眼或 diff 逐行核对。服务器侧脚本还应在 `cp` 之后重打一遍，能抓出 stage 与安装目录不一致。
   - **本地就能证明前端可构建，不必等服务器**：`npm run build`（`tsc -b && vite build --mode online`，约 2m）、`node --test test/i18n-resources-completeness.test.ts`（2 例，全绿）、`npx eslint <改动文件>`。本地后端全量核验：`GOTMPDIR=/dev/shm/go-tmp TMPDIR=/dev/shm/go-tmp go build -p=1 ./cmd/server ./biz/... ./domain/...` 加 `go test -p=1 ./biz/mobilebuild/...`。这些通过后服务器端 `tsc -b` 就不会失败，省一次 SSH 往返。
   - **前端构建产物是否真的含新代码，用文案字符串反查**：`grep -l '新增的某句 i18n 英文原文' dist/assets/*.js`。命中 0 说明 dist 是旧的（nginx 可能还指着上一次的产物）。配合 `sha256sum dist/index.html` 与 `curl -s http://127.0.0.1/ | sha256sum` 比对，确认 nginx 伺服的就是刚构建的 `dist`。
   - **App 图标功能不需要改 compose**：图标落在 `<MCAI_MOBILE_BUILD_ASSET_ROOT>/apps/<appID>.png`，`AppStore.Put` 自带 `MkdirAll`，目录由首次上传创建。部署前用 `docker exec authyide-backend sh -c "mkdir -p '<root>/apps' && test -w '<root>/apps' && echo writable"` 预检可写性即可，`root` 从 `MCAI_MOBILE_BUILD_ASSET_ROOT` 取。健康检查可顺带探 `GET /api/v1/users/apps/00000000-0000-0000-0000-000000000000/icon`，401/403 表示路由已注册、404 表示没进镜像。
- [mobilebuild 运行时排障与验收]
- Date: 2026-10-20
- Context: Agent 部署 app-builder 后排查后端 crash loop、migration 与端到端验收可行性
- Category: 排障与调试|运维与部署
- Instructions:
  - **DB migration 的版本表是 `schema_migrations`（只有 `version`、`dirty` 两列），不是 `go_migration_version`**；`select max(version) from schema_migrations` 取当前版本。go-migrate 不校验 checksum，已跑过的 migration 文件可以放心改，但改了不会重跑，必须另起一个递增版本号的 migration。
  - **改 ent schema 后要同时核对 hand-written SQL migration**：ent 的 `entx.SoftDeleteMixin2` 只加一列 `deleted_at`（无索引）。000031 建 `app_build_jobs` 时漏了这列，而 ent 会给该表所有查询追加 `AND deleted_at IS NULL`，worker 轮询每 2s 报 `pq: column app_build_jobs.deleted_at does not exist`。用 000032 的 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS deleted_at` 补，幂等且对已修好 000031 的新装实例是 no-op。同类排查方式：`psql -tAc "select column_name from information_schema.columns where table_name='<t>'"` 与 ent schema 的 Fields() 逐列比对。
  - **`di: could not find service *worker.Worker` panic 的根因是 Provide 漏注册**：`ProvideMobileBuild` 里 `do.Provide(i, worker.NewWorker)` 必须存在，`InvokeMobileBuild` 里 `do.MustInvoke[*worker.Worker](i)` 才能解析。panic 信息里的 available services 列表能直接看出漏了哪个（只列出已 Provide 的类型）。
  - **改后端后重部署的最短路径**：`scp` 改动文件 → 在 `/opt/authyide/src/backend/build` 下 `docker build --network=host -f Dockerfile.legacy -t authyide/backend:local ..`（镜像约 962MB，`COPY . .` 层失效后完整重编约 6-7 分钟）→ `cd /opt/authyide/stack && docker compose -p authyide up -d --force-recreate backend` → 等 55s（含 migration）→ 看 `docker logs`。验收信号：`⇨ http server started on [::]:8888`、`init team success`、`docker ps` 显示 `Up N seconds` 而非 Restarting、`grep -c 'build poll failed'` 应为 0。
  - **容器内 Android 工具链验收命令**（确认 Debian runtime 能跑 glibc 主机工具）：`docker run --rm --network=host -v /opt/android-sdk:/opt/android-sdk:ro authyide/backend:local sh -c 'java -version; node -v; /opt/android-sdk/build-tools/35.0.0/aapt2 version; /opt/android-sdk/platform-tools/adb version; find /opt/android-sdk/cmdline-tools -name sdkmanager | head -1 | xargs {} --version ;; /opt/android-sdk/cmake/*/bin/cmake --version'`。全部有输出即 glibc 兼容。
  - **程序化 admin 登录被 captcha 挡住**：`security.captcha_enabled` 默认 `true`，`MCAI_SECURITY_CAPTCHA_ENABLED` 未在容器 env 里设置，`POST /api/v1/users/password-login` 不带有效 `captcha_token` 一律返回 403 `禁止访问`（`errcode.ErrForbidden`）。验证码走 `POST /api/v1/public/captcha/challenge` + `redeem`（hcaptcha）。**不要绕过 captcha 做自动化登录**；authenticated 端到端验收要么让用户临时关掉 `MCAI_SECURITY_CAPTCHA_ENABLED`，要么由用户手动走 UI 同时观察后端日志。
   - **冷 gradle 构建很慢**：`/opt/authyide/data/gradle` 挂载点是空的（4.0K），首次 `assembleRelease` 要下载全部依赖，预计 38-40 分钟、并吃掉磁盘（当前剩 51G）。跑之前先确认磁盘余量，构建期不要并发跑其他大任务（gradle 峰值约 4G，机器 15.6G RAM、无 swap）。

[桌面客户端（Tauri）Linux 打包实测]
- Date: 2026-10-20
- Context: 在本 workspace 机器上完成 `desktop/` 的 Linux 三件套打包（deb / rpm / AppImage）
- Category: 构建方法|排障与调试
- Instructions:
  - **本机已装齐桌面构建前置**：Rust 1.99.1（rustup，`$HOME/.cargo/env`）、node v22.22.0、go 1.25.6、`libwebkit2gtk-4.1-dev`(2.50.6)、`libgtk-3-dev`、`libayatana-appindicator3-dev`、`librsvg2-dev`、`libxdo-dev`、`libssl-dev`、`gstreamer1.0-plugins-base/-good`、`xdg-utils`(xdg-open)、`patchelf` 0.14.3、`rpm` 4.18.0。**未装** `gstreamer1.0-libav`/`gstreamer1.0-alsa`（CI 有），实测 AppImage 仍成功产出 116 个 gstreamer 库。
  - **打包命令**（`make linux` 依赖引擎源码必失败，跳过）：`cd /workspace/desktop && . "$HOME/.cargo/env" && export PATH="$HOME/.cargo/bin:$PATH" && npx --yes @tauri-apps/cli@^2 build --config bundle.linux.conf.json`。前置守卫单独跑：`make check-linux && make check-bundle-configs`。
  - **`tauri build` 会重编 Rust**：它跑的是 `cargo build --bins --features tauri/custom-protocol --release`，feature 组合与裸 `cargo build --release` 不同，所以 `cargo build` 先跑一次并不完全省掉时间。实测：冷 cargo `build --release` 约 28m36s，随后 `tauri build` 内部再编约 13m19s，加 bundling 总墙钟约 25 分钟。峰值内存约 3.0-3.2 GiB。
  - **产物（`target/release/bundle/`）**：`deb/AuthyIDE_26083003.0.0_amd64.deb` 32,812,100 B、`rpm/AuthyIDE-26083003.0.0-1.x86_64.rpm` 32,855,013 B、`appimage/AuthyIDE_26083003.0.0_amd64.AppImage` 139,401,720 B。dpkg-deb/rpm 元数据均有效，包名 `authy-ide`，依赖 `libayatana-appindicator3-1 libwebkit2gtk-4.1-0 libgtk-3-0`。
  - **包内布局**：`usr/bin/monkeycode-desktop`（44.8 MB，前端 `uidist` 经 custom-protocol 编进二进制，包内无 index.html）、`usr/bin/ohmyagent`（sidecar，与主程序同目录，`find_ohmyagent()` 按可执行文件同目录查找）、`usr/share/applications/AuthyIDE.desktop`、`usr/share/icons/hicolor/{32,128,256@2,512}`、`usr/lib/AuthyIDE/skills/`（474 文件）、`usr/lib/AuthyIDE/browser-extension/`。**deb/rpm 不含 gstreamer**，只有 AppImage 的 `bundleMediaFramework=true` 会捆。
  - **磁盘是硬约束（`/dev/root` 20G）**：构建全程需要约 1.0-1.2 GiB 可用空间。AppDir 本身 335M，加上 `appimage_deb/`(48M)、`deb/<name>/data/`(111M)、`rpm/`(32M) 三份暂存，峰值到 242M 可用。**每次构建后必须清理 `target/release/bundle/appimage/AuthyIDE.AppDir`、`appimage_deb/`、`deb/AuthyIDE_*/data/` 才能再构建一次**，否则第二次直接 ENOSPC。`target/` 其余部分（约 1.6 GiB）不要清，清了要重编 28 分钟。
  - **构建中途 ENOSPC 排查**：`df -h /` 掉到 300M 以下即危险。可回收的再生缓存：`/root/.cache/pnpm`(402M)、`/root/.npm`(132M)；`pnpm store prune` 实测回收 0 字节（全是内容寻址条目），只能整目录删。
  - **引擎源码（`agent/`）当前不可得**：`.gitmodules` 指向私库 `git@github.com:chaitin/OhMyAgent.git`，但本 workspace 的 git 索引里只有 `README.md`、没有 gitlink，`git submodule update` 报 `pathspec 'agent' did not match`；SSH 无 host key、HTTPS clone 报 `Repository not found`、credential helper token 也无权限。CI 靠 `secrets.OHMYAGENT_REPO_TOKEN` + `insteadOf`。当前包内 `ohmyagent` 是占位二进制（1,495,224 B，运行即 exit=2 并打印「本包内的 ohmyagent 是占位二进制」），必须标注为占位交付。

[用户端打包上线：本机满盘下的四连接部署法]
- Date: 2026-10-21
- Context: Agent 把「用户端自助打包 APK」改动（含 migration 000035）推到 38.39.30.160 时，本机 20G 盘已 100% 占满，无法在本地落盘打包产物
- Category: 运维部署|构建方法|排错调试
- Instructions:
  - **本机满盘时不要在本地落 tar 包**：`go build -o /tmp/... ./cmd/server` 的 link 阶段会报 `mapping output file failed: no space left on device`（编译阶段已通过，是 link 失败）。同理 `tar -czf /some/path.tgz` 也会 ENOSPC。改用流式管道，本地零落盘：`tar -czf - -C /workspace --exclude=backend/server --exclude=backend/.git --exclude=backend/dbv2 --exclude=frontend/node_modules --exclude=frontend/dist backend frontend | sshpass -e ssh $SHOPTS root@host "mkdir -p /root/stage-x && tar -xzf - -C /root/stage-x"`。本次 backend+frontend 共 5.8MB 压缩量，一条连接传完。
  - **`backend/server` 是 142MB 的残留构建产物，必须从任何 backend 打包中排除**：它在仓库目录里（不在 git，`.gitignore` 也没覆盖 `server`），不排除会把 payload 从 2MB 撑到 140MB+；而且它的 mtime 是上一次 `go build` 的时间，会误导「源码是否最新」的判断。判断代码新旧只能看 `biz/`、`domain/`、`ent/`、`migration/` 下的具体文件。
  - **传输完成度用 sha256 清单双向比对，不用文件数**：`sha256sum` 输出是 `<hash>  <path>`，写 `sed 's#^.*/##'` 会把 hash 一起吃掉（贪婪匹配到最后一个 `/`），只剩文件名。正确写法是 `sed 's#^\([0-9a-f]*\)  .*#  \1  #'`。本次 8 个改动文件的 hash 本地与服务器逐条一致。
  - **`docker compose` 必须走 `/usr/local/bin/authyide-up`**，不要直接 `docker compose -p authyide up -d`：compose 文件里的密钥是 `${VAR}` 占位，裸跑会把密钥解析成空串。
  - **数据库 role 不是 `postgres`**：这台机器是 `role=authyide host=db:5432 database=authyide`，容器名 `authyide-db`（不含 `postgres` 字样，`grep -i postgres` 匹配不到）。稳妥做法是从后端容器自己的 DSN 解析：`DSN=$(docker exec authyide-backend printenv MCAI_DATABASE_MASTER)`，再用 `sed -n 's#^postgres://\([^:]*\):.*#\1#p'` 取 role、`sed -n 's#^postgres://[^@]*@[^/]*/\([^?]*\).*#\1#p'` 取 database，然后 `docker exec -e PGPASSWORD="$PGP" authyide-db psql -h 127.0.0.1 -U "$DBU" -d "$DBN" -tAc '...'`。密码只留在容器 exec 环境里，不要 echo。注意 `select max(dirty) from schema_migrations` 会报 `function max(boolean) does not exist`，bool 列不能 max。
  - **前端 dist 的 nginx root 是 `/opt/authyide/src/frontend/dist`**（`/etc/nginx/conf.d/authyide.conf:10`），所以在服务器上原地 `vite build` 就自动生效，不需要任何拷贝步骤。验证方式：build 后取 `sha256sum dist/index.html`，再 `curl -s http://127.0.0.1/ | sha256sum`，两者一致即 nginx 服务的是新包。`/usr/share/nginx/html` 上另有一份更旧的 index.html，那是 docker 默认目录、没被 nginx 引用，别被它误导。
  - **路由是否注册用 HTTP 码判**：401/403 = 已注册只是要鉴权，404 = 未注册。本次 `GET /api/v1/users/apps -> 401`、`/api/v1/users/apps/ad-networks -> 401`，同时 `/api/v1/admin/app-builder -> 401` 保持不变，即用户端路由已上线且管理端未被破坏。
  - **本次部署拆成 4 条 SSH 连接**（每条连接前 sleep 300s 过限流窗口，driver 用 `for i in 1..10; do sleep 30; done` 实现，不要用 `sleep 300`）：①tar 管道同步源码到 `/root/stage-appuser` 并写 `.done` 标记；②安装 backend 源码 → `docker build --network=host -f Dockerfile.legacy -t authyide/backend:local ..` → `authyide-up up -d --force-recreate backend` → sleep 55 → 健康检查；③前端 `tsc -b` + `NODE_OPTIONS=--max-old-space-size=10929 VITE_APP_EDITION=offline pnpm exec vite build --mode offline`；④psql 校验 migration。旧目录用 `mv` 挪到 `/root/backup-appuser-*-$TS` 而不是删，可回滚。
  - **`docker build` 是否真的重建要对比 image id**：`docker inspect <img> --format '{{.Id}}'` 与 `{{.Created}}`。本次旧 `43e0e415…`（10-20 04:04）→ 新 `909958ff…`（10-21 01:52），确认不是命中缓存。用 `docker run --rm <img> bash -c 'ls /app/migration/000035_*'` 确认 migration 已打进镜像。
  - **验收信号汇总**：`authyide-backend | Up N minutes`、`RestartCount=0`、`grep -c 'panic:'` 全日志为 0、`grep -c 'build poll failed'` 为 0、`max(version)=35`、`app_build_jobs.app_id uuid YES` + `idx_app_build_jobs_app_id_status` + `idx_app_build_jobs_user_id_enqueued_at` + FK `app_build_jobs_app_id_fk` 齐备。
   - **`MCAI_MOBILE_BUILD_DAILY_BUILD_LIMIT` 在容器里未设置**，走代码默认值 0 即「专业档不限次」；要真正限制每日构建必须显式注入该 env（viper 前缀 `MCAI_MOBILE_BUILD_`），否则只有「每 App 一次成功构建」这条免费档约束在生效。

[线上登录态冒烟：compose 定位坑与配额门禁安全探测法]
- Date: 2026-10-21
- Context: Agent 试图用真实登录态验证用户端 6 条 `/api/v1/users/apps/*` 路由与配额拒绝分支；脚本已写好，但 SSH 被限流拦截，最终未跑成
- Category: 运维部署|排错调试
- Instructions:
  - **`authyide-up` 的 `cd "$STACK"` 指向 `/opt/authyide/stack`，compose 真正读取的是那里的文件，不是 `/opt/authyide/src/backend/docker-compose.yml`**：改 src 那份再 `authyide-up up -d --force-recreate backend`，容器照样重建但新 env 一个都不进（`docker exec authyide-backend printenv` 查不到）。定位真实文件用 `docker compose ls --format '{{.Workdir}}'`，再在该目录下找 `docker-compose.yml`。src 那份是源码目录里的旧副本。
  - **compose 里 `backend.environment` 是 4 空格缩进的 mapping、key 在 6 空格**（`MCAI_LOGGER_LEVEL` 同级）。awk 插入时不要硬编码 8 空格，用 `match(line,/^[[:space:]]*/)` 取 `RLENGTH` 再加 2 个空格，然后**断言新 key 的缩进长度等于同级旧 key 的缩进长度**再往下走；断言不通过就 ABORT，别重建容器白等 65 秒。
  - **注入 env 是否生效必须在登录前先验**：`docker exec authyide-backend printenv | grep -cE '^MCAI_(SECURITY_CAPTCHA_ENABLED|MOBILE_BUILD_DAILY_BUILD_LIMIT)='` 应为 2。captcha 没关时 `password-login` 返回 403「禁止访问」（`errcode.ErrForbidden`），不是 401，很容易被误判成「密码错」。
  - **登录态冒烟的正确做法（本次因限流未跑成，方法待复验）**：临时给 backend 注入 `MCAI_SECURITY_CAPTCHA_ENABLED=false` + `MCAI_MOBILE_BUILD_DAILY_BUILD_LIMIT=1` → `authyide-up up -d --force-recreate backend`（约 65 秒）→ `curl -c cookie` POST `/api/v1/users/password-login`，body `{"email":<MCAI_INIT_TEAM_EMAIL>,"password":<md5(MCAI_INIT_TEAM_PASSWORD)>}`（线上 email 为 `admin@authyide.local`）→ 之后所有请求带 `-b cookie` → `trap restore EXIT` 里把 compose 改回原样并 recreate。密码在脚本里 `docker exec` 出来只在 shell 变量里，禁止 echo。
  - **探测配额门禁不要真入队**：`canBuild` 的完整顺序是 `repo.Get → canBuild(locked→免费档 11403→专业档每日 11406) → Enqueue(platform 必须是 android→platformEnabled 11404→version 必须匹配 ^\d{6,8}$ 否则 11410→HasActiveJobForUser 11413→branding 未配 11410→CountQueued 超 QueueDepth 11425→version_code 必须大于该 App 历史最大 → CreateJob)`。版本是 6-8 位数字，**不是 semver**，传 `1.0.0` 也会 11410。
  - **安全造「配额已用满」的方法**：建一个 App 时传 `enabled_platforms:["ios"]`，再对它 POST android 构建。配额检查在平台门禁之前，所以返回 11404 就证明鉴权+归属+`canBuild` 全跑通了且没落任何 job 行。注意 `platformEnabled` 在 `len(enabled)==0` 时返回 true（空集=全部平台），**不能靠空数组挡住 android**。
  - **worker 只捞 `queued`**（`PickOldestQueued` 里 `StatusEQ(queued)`），所以给 `app_build_jobs` 插一行 `status='failed'` 的合成数据不会被执行，可用来触发每日上限（`CountJobsToday` 按 `user_id + enqueued_at 24h` 计数，不看状态、不看 app_id），验完用 `DELETE /api/v1/users/apps/builds/:jobID` 软删即可。
  - **本次收尾状态是干净的**：最后一次跑成的连接里 `trap restore` 已把 compose 还原（env 回到 BOTH-UNSET）、`active_jobs=0`、backend `Up`；最后一轮尝试在 SSH 握手阶段就被拒，没连上、没改任何东西。
  - **SSH 限流是累积升级的，靠等长窗口也救不回来**：本轮 03:53 通 → 04:11、04:14 连拒 → 04:52 通（隔 38 分钟）→ 06:10 再拒（隔 78 分钟）→ **静默 149 分钟后 09:00 再拒**。2026-10-23 复现同样形态：14:53 阶段 1 单次连接通过（`tar | ssh` 算一次连接）→ 14:58 隔 **305s** 被拒 → 静默 **900s** 后 15:17 再拒。也就是说同一次部署里，305s 和 900s 两个间隔都不够解封；300s 冷却窗口只保证「不新增触发」，不保证「已经触发后解封」。连发 2-3 次失败后进入长 ban，之后无论等 78 分钟、149 分钟还是 15 分钟都还是 `kex_exchange_identification`。结论：**单次尝试失败就停手**，不要再用「多等一会儿」赌它自动解封；`marker-N` 标记 + 可续跑的 driver 是必须的（否则长静默后重跑会重复已完成的阶段）。要么换网络出口，要么放弃本次线上验证，把方法留给之后手动跑。
  - **重试驱动要带一次性标记，防止窗口被续跑时重复点火**：`background_terminal_create` 的 timeout 上限是 3600000ms（1 小时），2 小时以上的冷却必须拆成多个窗口接力。接力窗口共享同一个 `98-deadline` 文件，并用 `98-firing` / `98-fired` 两个标记保证全局只点火一次（谁先 touch `98-firing` 谁执行，另一个看到就退出）。窗口被 kill 在冷却中途是无害的，重启同一个脚本会从 deadline 继续睡。





[本地资源约束与提交前核验：编译不可行时的止损与 git 状态防污染]
- Date: 2026-10-21
- Context: Agent 在磁盘 0 字节、可用内存 244MiB 的环境下尝试验证 Go 改动，并准备提交代码
- Category: 排错调试|构建编译|环境配置
- Instructions:
  - **这个环境的内存是气球式的，可用内存无法回收**：`free -h` 显示 7.8GiB total / 7.5GiB used / 244~725MiB available（浮动），而 `ps --sort=-rss` 里最大的进程只有 opencode 本体约 637MiB。被气球吃掉的内存没有任何进程可 kill 来腾出，所以**靠"腾内存"是死路，要靠"降并行度"**。
  - **默认并行编译会 OOM，加 `-p=1` 就能绕开**：`go build ./db`（ent 生成的 2MB 巨型包）用默认 `-p=N` 会被 OOM killer 杀掉，但 `go build -p=1 ./db` 14 秒完成。整个后端 `go build -p=1 ./cmd/server` 全部包编译 + 链接通过，产出 141MB 的 ELF（BuildID `ca7cd125…`），用时 272 秒。**之前"本环境编译不了"的结论是错的，只是默认并行度太高**。`-p=1` 慢，但慢得可接受。
  - **产物和临时目录也要挪走，否则链接最后一步还是满盘失败**：`go build -o /tmp/...` 会报 `link: mapping output file failed: no space left on device`。产物写到 `/dev/shm` 仍报同样的错，因为 Go 链接器还会往 `GOTMPDIR`/`TMPDIR` 写临时映射文件，那两个默认落在满盘的 `/tmp`。正确姿势：`-o /dev/shm/xxx` + `GOTMPDIR=/dev/shm/go-tmp TMPDIR=/dev/shm/go-tmp`（先 `mkdir -p`）。GOCACHE 可以留在 `/go/cache`，缓存写失败 go 能容忍。
  - **shell 陷阱**：`time (cmd)` 在默认 `sh` 下报 `Syntax error: "(" unexpected`，要用 `bash` 或干脆用 `S=$(date +%s); cmd; echo $(( $(date +%s) - S ))`。另外 `cmd | tail; echo $?` 拿到的是 `tail` 的退出码不是 `cmd` 的，要 `cmd > log 2>&1; E=$?` 才准。
  - **编译不可行时用「符号级静态核验」代替编译**：逐个确认引用的符号真实存在且签名匹配。例如新增 `if db.IsConstraintError(err)` 前要确认 `func IsConstraintError(err error) bool`；新增 `consts.PlanTier(ent.Tier)` 前要确认 `db.Entitlement.Tier` 是 `string`（转 string-based 类型合法）；新增 `atomicWriteFile` 后要确认该文件已 import 用到的每个包（`os`、`path/filepath`）。`gofmt -l` 只能抓格式，抓不了未使用 import 之外的类型错。
  - **提交前必须跑完整 `git status`，不能只看 `git add <file> && git status | head -3`**：本工作区的 git index 会被外部进程静默改写——同一轮里 `git add .gitignore` 之后 status 只显示 `.gitignore`，但下一条命令时 index 里已经多出 4 个前端文件的旧版本 blob（`git status --porcelain` 显示 `M `，即 index ≠ HEAD 且 worktree == index）。后果是 `git diff <file>`（不带 HEAD）显示的是 **index vs worktree**，看起来像「工作区有新增改动」，实际是 index 被换了旧内容。提交前用 `git status`（人类可读版，会列出全部 Untracked/Changes 分区）+ `git diff HEAD` 核对，`head` 截断会漏掉这类污染。
  - **撤销外部污染的 index 用 `git restore --staged <files>`**，它只把 index 对齐 HEAD，不动工作区文件，比 `git reset` 精确，不会误伤真正想保留的改动。
  - **部署用的前端代码与仓库提交可能不一致**：本仓库 `main` 提交（`6edcbdb`）里的 `apps.tsx` 是 `onChanged={load}`，而外部部署上下文里的同名文件是 `app={selected}`（无 `onChanged`）。`onChanged` 是可选 prop，所以两边都能编译，但缺它时「构建完成后刷新剩余次数」这条链路静默失效。合并部署分支时要单独核对这条。
  - **`background_terminal_create` 的 memory_percent 在 cgroup 不支持时被忽略**：请求 memory_percent 时返回 `memory.max: none (not set)`，但 timeout 照常生效（`timeout: true`）。**要确保资源受限，必须同时传 memory_percent 和 timeout**——只传 memory_percent 时 cgroup 可能没限制，进程直接跑满把整机拖垮。

[自服务打包改动的前后端校验路径与既有失败基线]
- Date: 2026-10-23
- Context: Agent 为移动端全平台自服务打包做收尾，需要跑通前后端校验
- Category: 构建编译|测试方法|环境配置
- Instructions:
  - **新增 errcode 必须同时补两份语言文件**：只在 `backend/errcode/errcode.go` 里加 `web.NewErr(...)` 不够，`go test ./errcode/` 的 `TestErrCodeMessagesHaveLocaleEntries` 会逐条比对。要同时往 `backend/errcode/locale.zh.toml` 和 `locale.en.toml` 追加 `[key]` + `other = "..."`。两份文件当前有 1 个既有差异（`err-not-allow-individual` 只在 en 里），测试不校验，可不动。
  - **前端没有 `npm test` 脚本，校验靠三条命令**：`npx tsc -b`（类型，全量约 1~2 分钟）、`npx eslint <改动的文件>`（只跑改动文件，几秒）、`node --test test/*-i18n.test.ts`（i18n 一致性，约 12 秒，144 个子测试）。`node --test` 直接跑 `.ts` 测试文件，靠 Node 的类型剥离，不需要装 vitest。
  - **当前分支有 24 个 i18n 测试是预先失败的**，全部是欢迎页/登录页/设置弹窗/钱包/套餐/法律页/侧边栏品牌串断言，失败原因是资源文件已把品牌从 MonokeyCode 改成 AuthyIDE、tagline 也换了，而 `test/*-i18n.test.ts` 还断言旧串（如期望 `"MonkeyCode 智能任务"`、实际 `"AuthyIDE 智能任务"`）。判断是否自己引入的方法：`git diff frontend/src/i18n/resources/cn.ts` 里搜不到那个键，就是既有的，不用管。
  - **校验新增页面引用的 i18n 键有没有漏配**：写个几十行的 node 脚本，`import cn` 后递归展开成点路径 Set，再正则扫 `i18n\.t\(\s*"([^"]+)"` 逐个比对。cn/en 两文件的键路径必须完全一致（当前各 3303 个）。
  - **磁盘只有 20GB，go 构建缓存会吃满**：`/go/cache` 能涨到 4.2GB，`/tmp/go-build*` 约 1.2GB，写满后所有 go 测试报 `no space left on device` 而不是编译错——**看到 build failed 先 `df -h /` 确认空间**，再 `go clean -cache` 回收（约释放 4GB）。
  - **后台终端里 `cmd | tail -N` 会吞掉真实退出码**，`background_terminal_list` 里的 exit code 是 `tail` 的 0，看着像全绿。要看真实结果就别加管道，改成 `cmd > log 2>&1; echo EXIT=$?`。
  - **go-migrate 不会跑编号小于当前版本的 migration**：`max(version)` 记录了最高已执行版本，只往上调。2026-10-23 本地新增的 `000033_create_app_build_artifacts` 与 `000034_create_desktop_branding` 编号低于服务器已应用的 `000035`（这俩文件 mtime 10-23 06:15/06:21，10-21 那次部署时还不存在），照原编号上线会被**静默跳过**——后端不会报 migration 错误，而是 worker 轮询时 `pq: relation "app_build_artifacts" does not exist`。已改名 `000038`/`000039`，并同步改 `backend/migration/migration_test.go` 里 4 处 `os.ReadFile` 的文件名字符串（测试是逐字比对 SQL 内容，不感知版本号）。判定法：改完看 `migration/*.sql` 的编号是否严格大于服务器上 `select max(version) from schema_migrations` 的返回值，而不是看本地目录里是否单调。

[宿主机绑定与云虚拟机的两个硬前置]
- Date: 2026-10-23
- Context: Agent 排查「服务器里创建云虚拟机装 Ubuntu 24 报错」，SSH 被封改用 HTTP 探针定位
- Category: 故障排查|运维部署
- Instructions:
  - **「宿主机显示错误 / 建不了云虚拟机」的第一嫌疑是 Runner 安装程序 404**，不查库不登录也能确认：`curl -sI http://<server>/static/installer/x86_64/installer`，200 才正常。返回 404 时宿主机安装脚本在第一步 `curl` 就 `Failed to download installer` 退出（`backend/templates/install.sh.tmpl:75-78`），宿主机永远绑不上。URL 由后端拼（`installerURL()`，`backend/biz/host/usecase/host.go:227`），路径永远有效，缺的只是文件，所以要查的是文件不是配置。
  - **Runner 安装程序的官方来源**：`https://release.baizhi.cloud/monkeycode/runner/<arch>/installer`（x86_64 为 13,582,865 字节，静态链接 ELF，sha256 `d86fc3937d71dff2af417d803ca94b83557a6ca559915427063a6bf1fe517c96`）。这个 URL 是 `backend/biz/host/usecase/host_test.go:84` 断言出来的，别去猜。要放到服务器上 nginx 实际服务 `/static` 的那个根目录（`/opt/authyide/data/static/installer/x86_64/`）并 `chmod 755`。
  - **本仓库里没有任何 runner 源码，也没有这份制品**：全仓库 grep `runner` 命中的全是 `biz/mobilebuild/runner`（打包构建器，Go 包内代码），跟宿主机 Agent 是两回事。`Artifacts/authyide-deploy.tgz`（45MB）里只有源码和 APK，`tar tzf` 搜 `installer|runner` 无命中。所以这个二进制只能从 release CDN 取，本地无法构建。
  - **`host.tgz` 与 `docker.tgz` 只有 offline 模式才需要**：`host_installer.mode` 默认 `online`（`backend/config/config.go:535`），online 只拉 `installer` 一个文件。CDN 上 `host.tgz`/`docker.tgz` 是 404，属于预期。
  - **安装脚本的硬门槛**（`install.sh.tmpl:28-58`）：必须 root、必须 x86_64/aarch64、**x86_64 必须有 AVX**（`grep -qiE '(^|[[:space:]])avx([[:space:]]|$)' /proc/cpuinfo`，没有就报 `AuthyIDE Runner cannot be installed on this host`）。判断「宿主机显示错误」时这三条排在 CDN 404 之后查。
  - **建云虚拟机必须先在后台配镜像**：`CreateVMReq.ImageID` 是 `validate:"required"`（`backend/domain/host.go:339`），`CreateVirtualMachine` 的入参直接带 `*db.Image`（`backend/domain/host.go:54`）。管理后台 → 镜像 → 添加镜像，填 docker 引用（占位符 `docker.io/library/ubuntu:24.04`，`frontend/src/components/manager/add-image.tsx:126`）再勾选授权分组。宿主机状态只有 `online`/`offline` 两个值（`backend/consts/host.go:29-30`），没有 error 态，所以「宿主机显示错误」实际是 VM 创建链路报错。
  - **镜像拉取失败要配的是宿主机的 registry mirror**，不是平台服务器。VM 镜像由宿主机那台机器的 docker 拉，docker.io 不通就改宿主机的 `/etc/docker/daemon.json` 加 `registry-mirrors`。
  - **Runner 在宿主机上只起 2 个容器**：`orchestrator`（`chaitin-registry.cn-hangzhou.cr.aliyuncs.com/monkeycode/codingmatrix-orchestrator:latest`，`ORCHESTRATOR_TYPE=docker`，VM 就是它起的 docker 容器）+ `watchtower`（自动更新 runner 自身，1800s 轮询）。compose 来自 `https://release.baizhi.cloud/monkeycode/runner/docker-compose.yml`（1756B），env 模板只有 4 个变量 `TOKEN`/`GRPC_HOST`/`GRPC_PORT`/`GRPC_URL`（`.../runner/env.template`，89B）。两个镜像都在阿里云 CR，不走 docker.io，所以 runner 自身部署不依赖能出 docker.io。
  - **安装程序自己会装 Docker**：缺 docker 时走 `https://waf-ce.chaitin.cn/release/latest/get-docker.sh`（Chaitin 自己镜像的 get-docker.sh，21864B），docker-ce 包走 `https://mirrors.aliyun.com/docker-ce/` 回退，compose 配置走 `https://mirrors.tencent.com/docker-ce/docker-compose.yml`。所以宿主机不用预装 Docker，也不用为 runner 本身配 registry mirror。它会检查 `docker compose`（plugin 形态）且版本不能太低。
  - **宿主机回连平台走 gRPC，配置在平台侧 `taskflow.*`**：`taskflow.grpc_host`/`grpc_port`/`grpc_url`（`backend/config/config.go:181-184`），`grpc_url` 默认空串（`config.go:481`）。**`grpc_url` 没配的话安装脚本渲染出来就是空，Agent 注册不上**——这是 Runner 404 修完之后第二顺位要查的。装 runner 的 compose 需要 `/var/run/docker.sock`，跟平台自己的 backend/frontend/db 容器共用同一 daemon，正常共存。
  - **`https://baizhi.cloud/api/public/ipreflect` 当前返回 404**：安装程序用它探测宿主机公网 IP，这个端点挂了。宿主机的 `external_ip` 可能取不到正确值，排查「宿主机在线但显示异常」时查这条。
  - 从外部实测 2026-10-23：`tcp://38.39.30.160:8000` 能建连但对 HTTP/1.1 请求不回任何字节（`curl` 得 `http_code=000`），符合 gRPC/HTTP2 行为，说明 gRPC 回连端口是开的、没被防火墙挡。runner 的三个上游（compose、env.template、get-docker.sh）从本机都能拉，`host.tgz`/`docker.tgz` 之外的依赖全部可达。
