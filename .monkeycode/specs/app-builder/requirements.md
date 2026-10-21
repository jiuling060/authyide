# Requirements Document

## Introduction

管理员在管理后台的打包页面完成一次客户端构建，平台可选移动端（Android）或桌面端（Linux）。

移动端路径：上传品牌素材、配置 APP 名称与版本号、选择服务端地址，触发服务端构建。系统在单槽内串行执行 `gradle assembleRelease`，把 APK 发布为「原生更新」，移动端启动与回前台时自动检测新版本并引导下载安装。仅 JS 层改动另走 expo-updates OTA 通道。

桌面端路径：触发服务端构建后，系统在打包机上串行执行 `ui-next` 构建、ohmyagent 引擎 sidecar 编译、浏览器扩展构建、`tauri build`，产出 deb / rpm / AppImage 三个安装包，其中 AppImage 按 Tauri updater 协议写出升级清单，使已安装客户端获得自更新能力。

两条路径共用打包槽位、任务状态机、日志尾部、权限收敛与审计约束；差异集中在平台维度、产物模型与发布协议。

本功能恢复 `migration/000028_remove_app_build_and_ad_placements` 中删除的打包能力，并补上被它删掉的产物发布链路。

## Glossary

- **品牌配置 (Branding Profile)**: 一个实例内唯一的一组 APP 元数据，包含名称、版本号、versionCode、服务端地址、OTA 发布地址。
- **品牌素材 (Branding Asset)**: 品牌配置下按槽位管理的图片文件，槽位为 `logo-light`、`logo-dark`、`icon`、`icon-dark`、`adaptive-icon`、`splash`、`splash-dark`。
- **桌面品牌 (Desktop Branding)**: 桌面客户端独立的一组元数据，包含产品名、bundle identifier、updater 端点与图标素材键，与移动端品牌不共享字段。
- **打包平台 (Build Platform)**: 构建目标形态，取值 `android` 或 `desktop-linux`。
- **打包工作区 (Build Workspace)**: 构建时使用的子项目目录。移动端包含 `app.json`、`assets/` 与 `android/`；桌面端包含 `tauri.conf.json`、`ui-next/`、`src/` 与图标源。
- **打包机 (Build Host)**: 具备构建工具链的 Linux 主机，是打包任务的唯一执行位置。移动端需 JDK 与 Android SDK；桌面端需 Rust、Node、Go 与 WebView 系统依赖。
- **打包槽位 (Build Slot)**: 同一时刻最多容纳一个打包任务的互斥单元，跨平台共享。
- **打包任务 (Build Job)**: 一次完整的构建执行记录，带状态机与日志尾部。
- **桌面产物 (Desktop Artifact)**: 一次桌面打包产出的安装包文件，同一任务可产出多个，按分发渠道（`deb` / `rpm` / `appimage`）区分。
- **引擎 Sidecar (Engine Sidecar)**: 随桌面包分发的 ohmyagent 引擎二进制，命名约定为 `binaries/ohmyagent-<triple>`，由打包配置 `bundle.externalBin` 声明。
- **打包配置 Overlay (Bundle Overlay)**: 文件名形如 `bundle.<platform>.conf.json` 的 Tauri 配置，只经 `--config` 显式传入，声明 `bundle.active`、`externalBin` 与 `resources`。
- **原生发布 (Native Release)**: 把某个成功任务的 APK 复制为版本化静态产物，并写出版本平台清单文件，使移动端原生更新检测生效。
- **桌面升级发布 (Desktop Update Release)**: 把某个成功任务的 AppImage 复制为版本化静态产物，并按 Tauri updater 协议写出升级清单。
- **OTA 发布 (OTA Release)**: 把移动端子项目导出为 JS 包，通过 expo-updates 协议下发给已装新包的客户端。
- **原生更新通道 (Native Update Channel)**: `<静态根>/app-version/<platform>.json`，内容为 `{version, url}`，供移动端比对已装版本。
- **桌面升级清单 (Desktop Update Manifest)**: `<静态根>/desktop/latest-<platform>.json`，Tauri updater 协议格式，供桌面客户端轮询新版本。

## Requirements

### Requirement 1: 平台能力与前置检查

**User Story:** AS 管理员, I want 在打包前先看到各平台是否可用及原因, so that 不会点一个必然失败的构建按钮。

#### Acceptance Criteria

1. WHEN 管理员查询打包能力状态, THE 系统 SHALL 分别返回移动端与桌面端是否可用，以及各自的产物渠道列表与槽位白名单。
2. WHEN 判断平台可用性, THE 系统 SHALL 检查该平台开关、打包工作区、引擎源码目录与全部工具链命令同时满足，任一缺失即判定不可用。
3. IF 平台判定为不可用, THE 系统 SHALL 在状态响应中列出缺失项名称，前端据此隐藏该平台入口并展示原因。
4. IF 管理员对判定为不可用的平台提交打包请求, THE 系统 SHALL 拒绝并返回前置缺失业务码，且不创建打包任务。

### Requirement 2: 移动端品牌配置读写

**User Story:** AS 管理员, I want 配置 APP 名称、版本号与 versionCode 以及服务端地址, so that 打包产物与当前实例的身份一致。

#### Acceptance Criteria

1. WHEN 管理员读取移动端品牌配置, THE 系统 SHALL 返回名称、版本号、versionCode、服务端地址、OTA 发布地址五个字段，若从未设置过 SHALL 返回从移动端子项目当前 `app.json` 解析出的默认值。
2. WHEN 管理员提交移动端品牌配置且名称非空、版本号匹配 `^\d{6,8}$`、versionCode 为正整数, THE 系统 SHALL 持久化该配置并返回保存后的完整配置。
3. IF 提交的版本号为空或版本号含非数字字符, THE 系统 SHALL 拒绝保存并返回参数错误业务码，且不改动已存配置。
4. IF 提交的 versionCode 小于已发布过的最大 versionCode, THE 系统 SHALL 拒绝保存并返回参数错误业务码。
5. WHILE 移动端品牌配置已保存, THE 系统 SHALL 把配置写入 `Branding Profile` 表，且该实例内只存在一行。

### Requirement 3: 桌面品牌配置读写

**User Story:** AS 管理员, I want 单独配置桌面客户端的产品名与 bundle identifier, so that 桌面包与移动端包的身份可以独立演进。

#### Acceptance Criteria

1. WHEN 管理员读取桌面品牌配置, THE 系统 SHALL 返回产品名、bundle identifier、updater 端点、图标素材键四个字段，若从未设置过 SHALL 返回从桌面子项目当前 `tauri.conf.json` 解析出的默认值。
2. WHEN 管理员提交桌面品牌配置且产品名非空, THE 系统 SHALL 持久化该配置并返回保存后的完整配置。
3. IF 提交的 bundle identifier 不符合反向域名格式, THE 系统 SHALL 拒绝保存并返回参数错误业务码。
4. WHILE 桌面品牌配置已保存, THE 系统 SHALL 把配置写入 `Desktop Branding` 表，且该实例内只存在一行。
5. WHEN 管理员配置桌面品牌, THE 系统 SHALL 不改动移动端品牌配置的任何字段。

### Requirement 4: 品牌素材上传

**User Story:** AS 管理员, I want 按槽位上传 logo、桌面图标与启动图, so that 打包出来的 APP 使用自己的品牌。

#### Acceptance Criteria

1. WHEN 管理员上传某个槽位的图片文件, THE 系统 SHALL 校验其为 PNG 且文件不超过 4 MiB，校验通过 SHALL 存为对象存储对象并更新该槽位的素材引用。
2. IF 上传的文件不是 PNG 或超过 4 MiB, THE 系统 SHALL 拒绝并返回参数错误业务码，且不改动该槽位现有素材。
3. WHEN 管理员读取素材清单, THE 系统 SHALL 返回全部槽位及其素材是否存在、最后上传时间、可访问 URL。
4. IF 某槽位从未上传素材, THE 系统 SHALL 在清单中标记该槽位为空，且打包时使用子项目的内置默认素材。
5. IF 管理员请求删除某槽位素材, THE 系统 SHALL 清除该槽位引用，使打包回落到内置默认素材。
6. WHEN 管理员上传桌面图标源素材, THE 系统 SHALL 校验其为 1024×1024 的 PNG，该素材在桌面打包时由构建流程派生各尺寸图标。

### Requirement 5: 移动端在线打包

**User Story:** AS 管理员, I want 一键触发服务端 APK 构建, so that 不需要本地装 JDK 和 Android SDK。

#### Acceptance Criteria

1. WHEN 管理员提交平台为 `android` 的打包请求并指定版本号, THE 系统 SHALL 创建状态为 `queued` 的打包任务并返回任务 id。
2. WHEN 打包任务被槽位拾取, THE 系统 SHALL 依次执行素材注入、`app.json` 重写、`expo prebuild`、`gradle assembleRelease`，任一步骤退出码非零 SHALL 把任务置为 `failed` 并记录该步骤名与日志尾部。
3. WHILE 打包任务运行中, THE 系统 SHALL 把退出码、产物路径、产物字节数写入任务记录。
4. WHEN 打包任务成功, THE 系统 SHALL 把任务状态置为 `succeeded` 并写入产物字节数与耗时。
5. IF 打包超时超过配置的上限, THE 系统 SHALL 终止子进程并把任务置为 `failed`，错误信息标注为超时。
6. IF 打包失败, THE 系统 SHALL 保留失败任务的完整记录，管理员可对同一配置重新提交新任务。

### Requirement 6: 桌面在线打包

**User Story:** AS 管理员, I want 一次触发完成桌面客户端全部构建步骤, so that 不需要在打包机上手工执行命令序列。

#### Acceptance Criteria

1. WHEN 管理员提交平台为 `desktop-linux` 的打包请求并指定版本号, THE 系统 SHALL 创建状态为 `queued` 的打包任务并返回任务 id。
2. WHEN 打包任务被槽位拾取, THE 系统 SHALL 依次执行前置检查、`ui-next` 构建、引擎 sidecar 编译、浏览器扩展构建、`tauri build`，任一步骤退出码非零 SHALL 把任务置为 `failed` 并记录该步骤名与日志尾部。
3. WHEN 引擎 sidecar 编译步骤开始, THE 系统 SHALL 把引擎源码的 git 短提交哈希注入产物的版本符号，使客户端关于页显示的引擎版本与实际参与构建的检出一致。
4. IF 打包工作区、引擎源码目录或 Node / Rust / Go 任一工具链不可用, THE 系统 SHALL 在入队前判定为配置错误并拒绝任务，不占用打包槽位。
5. WHEN 打包产物生成完毕, THE 系统 SHALL 校验三个渠道的安装包均存在且非空，缺一即判为失败并标注缺失的渠道。
6. WHEN 桌面打包任务成功, THE 系统 SHALL 把每个产物记录为独立的产物条目，条目含渠道名、文件名、字节数与 sha256。
7. IF 某次桌面打包在部分产物生成后失败, THE 系统 SHALL 记录已产出的部分产物并标记任务整体为失败，且不覆盖此前同一版本号的成功产物。

### Requirement 7: 打包槽位互斥

**User Story:** AS 系统, I want 同一时刻只执行一个打包任务, so that 不同时段的构建不互抢打包机内存与磁盘。

#### Acceptance Criteria

1. WHILE 已有任意平台的打包任务处于 `running` 状态, THE 系统 SHALL 拒绝新的打包请求并返回「构建中」业务码，HTTP 状态码为 409。
2. WHEN 新请求被拒绝, THE 系统 SHALL 在响应中返回当前运行中任务的 id、平台与开始时间。
3. WHEN 管理员提交第二个请求而第一个任务正在排队中, THE 系统 SHALL 同样返回 409，不建立重复排队任务。
4. IF 打包进程意外退出而任务状态未更新, THE 系统 SHALL 在服务启动时把孤儿 `running` 任务置为 `failed`，错误标注为进程中断。

### Requirement 8: 打包机资源约束

**User Story:** AS 系统, I want 在打包前确认磁盘与内存余量, so that 不会在构建接近完成时因资源耗尽而失败。

#### Acceptance Criteria

1. WHEN 打包任务被槽位拾取, THE 系统 SHALL 检查打包机可用磁盘与可用内存，低于配置阈值即拒绝执行并标注为资源不足。
2. WHILE 打包任务运行中, THE 系统 SHALL 按配置的轮询周期刷新任务日志尾部与已运行秒数。
3. IF 打包超时超过配置上限, THE 系统 SHALL 终止子进程树并把任务置为 `failed`，错误信息标注为超时与配置上限值。
4. WHEN 打包任务结束, THE 系统 SHALL 保留该次构建的完整日志文件，供管理员通过任务详情获取路径。

### Requirement 9: 构建进度可见

**User Story:** AS 管理员, I want 看到任务状态与最新日志, so that 我知道还要等多久、失败在哪一步。

#### Acceptance Criteria

1. WHEN 管理员查询任务详情, THE 系统 SHALL 返回状态、当前步骤名、已运行秒数、日志尾部（截断至配置上限）、错误信息与起止时间。
2. WHILE 任务运行中, THE 系统 SHALL 至少每 5 秒刷新一次日志尾部。
3. WHEN 管理员查询任务列表, THE 系统 SHALL 按创建时间倒序返回任务的状态、平台、目标版本号、产物字节数与耗时。
4. IF 任务处于 `queued`, THE 系统 SHALL 返回排队中的前置任务数。

### Requirement 10: 产物下载

**User Story:** AS 管理员, I want 下载已构建的安装包, so that 我可以离线分发给用户。

#### Acceptance Criteria

1. WHEN 管理员请求下载某成功任务的产物, THE 系统 SHALL 按该产物的 MIME 类型流式返回，`Content-Disposition` 为 `attachment` 并带版本化文件名。
2. IF 任务未成功或产物文件缺失, THE 系统 SHALL 返回产物不可用业务码。
3. IF 管理员请求的渠道在该任务产物中不存在, THE 系统 SHALL 返回产物不可用业务码。
4. IF 管理员未指定渠道且该任务平台仅有单一主产物, THE 系统 SHALL 自动选择该主产物。
5. IF 管理员未指定渠道但该任务有多个产物, THE 系统 SHALL 返回参数错误业务码并要求显式指定渠道。
6. WHEN 产物已通过发布对外可见, THE 系统 SHALL 同时提供公开静态下载地址，且两者内容 sha256 一致。

### Requirement 11: 移动端原生发布

**User Story:** AS 管理员, I want 把构建产物发布为原生更新, so that 已装旧版的用户自动收到新版本提示。

#### Acceptance Criteria

1. WHEN 管理员发布某成功任务, THE 系统 SHALL 把 APK 复制到版本化静态路径 `<静态根>/mobile/<version>/authyide.apk`，并写入 `<静态根>/app-version/android.json`，内容为 `{"version": <version>, "url": <公开下载 URL>}`。
2. IF 待发布版本号小于或等于清单文件当前记录的版本号, THE 系统 SHALL 拒绝发布并返回版本过旧业务码。
3. WHEN 发布成功, THE 系统 SHALL 记录发布时间与产物 sha256 到任务记录，并标记该任务为已发布。
4. IF 清单文件写入失败, THE 系统 SHALL 回滚已复制的版本化产物，不留下半成品。
5. WHEN 管理员查询发布状态, THE 系统 SHALL 返回各平台清单当前的版本号与下载 URL、最后发布时间。

### Requirement 12: 桌面升级发布

**User Story:** AS 管理员, I want 把桌面产物发布为自更新源, so that 用户不需要手动下载新版安装包。

#### Acceptance Criteria

1. WHEN 管理员发布某成功任务的 `appimage` 渠道产物, THE 系统 SHALL 把该产物复制到版本化静态路径，并写出桌面升级清单，清单含版本号、产物 sha256 与下载 URL。
2. IF 待发布版本号小于或等于清单当前记录的版本号, THE 系统 SHALL 拒绝发布并返回版本过旧业务码。
3. WHEN 发布成功, THE 系统 SHALL 记录发布时间与产物 sha256 到任务记录并标记该任务为已发布。
4. IF 清单写入失败, THE 系统 SHALL 回滚已复制的版本化产物，不留下半成品。
5. IF 管理员请求发布 `deb` 或 `rpm` 渠道产物, THE 系统 SHALL 返回渠道不可发布业务码，并说明该渠道由包管理器负责升级。
6. IF 未配置签名密钥, THE 系统 SHALL 仍写出清单但在发布状态中标记为未签名，使前端能提示该实例不具备客户端自更新能力。

### Requirement 13: OTA 发布

**User Story:** AS 管理员, I want 单独发布一次 OTA, so that 只改 JS 时无需等整包构建。

#### Acceptance Criteria

1. WHEN 管理员触发 OTA 发布, THE 系统 SHALL 在工作区执行 `expo export` 并把产物推送到 expo-updates 协议端点。
2. IF OTA 发布未启用或发布地址为空, THE 系统 SHALL 返回 OTA 未启用业务码，前端据此隐藏该入口。
3. IF `expo export` 失败, THE 系统 SHALL 记录失败原因，且不改动已上线的原生发布清单。
4. WHEN OTA 发布成功, THE 系统 SHALL 记录导出时间、runtimeVersion 指纹与产物字节数。

### Requirement 14: 权限收敛

**User Story:** AS 系统, I want 打包能力仅管理员可用, so that 普通成员无法触发构建或读取产物。

#### Acceptance Criteria

1. WHEN 用户角色不是 `admin` 访问任一打包接口, THE 系统 SHALL 返回未授权错误。
2. WHILE 用户角色为 `admin`, THE 系统 SHALL 允许访问全部打包接口。
3. WHEN 未登录访问打包接口, THE 系统 SHALL 返回认证错误。
4. THE 系统 SHALL 对打包与发布操作记录审计日志，包含操作者、任务 id 与平台。

### Requirement 15: 移动端原生更新检测

**User Story:** AS 移动端用户, I want 自动收到新版本提示, so that 我不需要手动找安装包。

#### Acceptance Criteria

1. WHEN 移动端启动或回到前台, THE 移动端 SHALL 请求 `<updatesServer>/app-version/<platform>.json` 并比对已装版本号。
2. IF 清单版本号大于已装版本号, THE 移动端 SHALL 展示新版本提示并提供跳转下载或商店的入口。
3. IF 清单请求失败或网络不可达, THE 移动端 SHALL 静默跳过，不弹错误提示，且不阻塞主流程。
4. WHILE `extra.updatesServer` 未配置, THE 移动端 SHALL 不做原生更新检测。
5. WHEN 用户确认更新, THE 移动端 SHALL 打开清单中记录的 URL，并在跳转后回到应用。

### Requirement 16: 管理后台打包页面

**User Story:** AS 管理员, I want 在一个页面完成配置、打包、查看、发布, so that 不用为每个客户端切入口。

#### Acceptance Criteria

1. WHEN 管理员打开打包页面, THE 前端 SHALL 展示品牌配置表单、素材上传区、构建按钮、任务列表与当前发布状态五个区块。
2. WHEN 管理员打开打包页面, THE 前端 SHALL 在构建表单与任务列表处区分平台，并对不可用的平台展示缺失的前置项。
3. WHILE 有打包任务运行中, THE 前端 SHALL 自动轮询任务详情并展示当前步骤与进度指示，构建按钮置灰。
4. WHEN 移动端打包成功, THE 前端 SHALL 在任务条目上展示产物大小、耗时、下载与发布两个动作。
5. WHEN 桌面打包成功, THE 前端 SHALL 按渠道分别展示产物大小、耗时与下载动作，且仅对 `appimage` 渠道展示发布动作。
6. IF 管理员不是 `admin`, THE 前端 SHALL 不展示该导航入口。
7. WHEN 页面加载, THE 前端 SHALL 展示各平台清单当前的版本号与发布时间。

### Requirement 17: 构建失败可诊断

**User Story:** AS 管理员, I want 失败时知道具体原因, so that 我能修正后重试。

#### Acceptance Criteria

1. WHEN 打包任务失败, THE 系统 SHALL 记录失败的步骤名、子进程退出码与日志尾部。
2. IF 日志尾部不足以定位问题, THE 管理员 SHALL 能通过任务详情获取完整日志文件路径。
3. WHEN 打包任务因超时失败, THE 系统 SHALL 在错误信息中标注超时与配置的上限值。
4. IF 工作区缺少构建入口或工具链不可用, THE 系统 SHALL 在入队前即判定为配置错误并拒绝该任务，不占用槽位。

## Non-Goals

- 不实现 iOS 打包与 App Store 提审流程。
- 不实现桌面端 `macos` 与 `windows` 打包；两者分别需要 Apple 签名与公证凭据、Windows 环境或交叉编译加签名凭据，属独立交付。
- 不实现代码签名、公证与吊销流程；本次产物为未签名内部构建。
- 不由本功能负责桌面引擎源码的获取与版本升级；引擎源码作为打包机前置输入提供。
- 不替代 App Store / Play Console 与商店分发，只提供直链下载。
- 不实现签名证书管理，沿用移动端子项目现有的 release 签名配置。
- 不实现构建产物按用户分发的授权访问；发布清单与安装包均为公开静态资源。
- 不实现多实例、多品牌、多租户打包；每个实例各有一个移动端品牌与一个桌面品牌。
