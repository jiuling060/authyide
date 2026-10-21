# 自助打包（App Self-Service Packaging）需求文档

Feature Name: app-self-service-packaging
Updated: 2026-10-20
Status: draft

## Introduction

当前打包能力只开放给管理员：13 条路由全在 `/api/v1/admin/app-builder` 前缀下，
每条 handler 内重复 `requireAdmin()` 校验。终端用户在用户中心拿到 App 记录后
无法自行产出可安装产物，必须找管理员代打包并分发。

本特性把打包能力开放给所有登录用户，按配额控制构建次数。用户中心已有的
`apps` 与 `entitlements` 数据模型、以及 `biz/app` 已实现的配额与锁定判定逻辑
直接复用，本特性主要补三件事：任务表补 `app_id` 关联、路由从管理前缀迁到
用户前缀、前端在用户控制台暴露打包入口。

历史脉络：`000026` 曾创建 `app_build_jobs` 并带 `app_id` 外键；`000028` 以
「App packaging is not delivered in this release」为由删掉该表，但保留了
`apps` 的构建计数器与 `entitlements` 表；`000031` 以「this deployment has no
user-center app concept」为由重建该表、去掉了 `app_id`。本特性纠正
`000031` 的这一判断：用户中心的 App 概念一直存在于 `apps` 表并被 `biz/app`
使用。

## Glossary

- **App**: 用户在用户中心创建的跨平台应用记录，一行一条，归属单一用户，
  携带名称、包标识、图标与平台集合，见 `apps` 表。
- **权益 (Entitlement)**: 每个用户一行，描述档位、免费构建计数与桌面许可，
  见 `entitlements` 表。
- **配额 (Build Quota)**: 档位决定的构建次数上限。免费档每个 App 限一次
  成功构建；专业档限制由配置项决定。
- **锁定 (Locked)**: App 达到免费档构建上限后的冻结状态，禁止修改任何字段。
- **构建任务 (Build Job)**: 一次打包请求，见 `app_build_jobs` 表。
- **打包平台 (Build Platform)**: 构建目标形态，取值 `android` 或 `desktop-linux`。
- **品牌配置**: 管理员维护的实例级移动 APP 品牌，见 `app_branding` 表。
- **素材槽位**: 可上传图片的位置，白名单 7 个：logo-light、logo-dark、icon、
  icon-dark、adaptive-icon、splash、splash-dark。

## Requirements

### Requirement 1: App 与权益模型

**User Story:** AS 登录用户, I want 在用户中心持有自己的 App 记录与配额档位, so that 我可以自助决定为自己打包什么。

#### Acceptance Criteria

1. WHEN 用户读取自己的 App 列表, THE 系统 SHALL 返回每个 App 的名称、包标识、
   图标键、广告网络、平台集合、构建次数、锁定状态、最近构建时间与产物引用，
   并返回该用户的档位与宿主额度。
2. WHEN 用户创建 App 且包标识在该用户名下唯一, THE 系统 SHALL 创建 App 并
   将状态置为 `draft`、构建次数置为 0、锁定状态置为假。
3. IF 用户提交的包标识在该用户名下已存在, THE 系统 SHALL 拒绝创建并返回
   包标识冲突业务码。
4. IF 用户请求读取或修改的 App 不属于该用户, THE 系统 SHALL 返回 App 不存在业务码，
   不暴露 App 是否存在的差异。
5. WHEN 用户请求读取某个 App 且该用户尚无权益行, THE 系统 SHALL 先创建免费档权益行，
   再返回该 App。
6. IF App 处于锁定状态且用户提交修改, THE 系统 SHALL 拒绝并返回 App 已锁定业务码。
7. IF 用户提交的广告网络不在注册表 `packer/ad_networks.yaml` 内, THE 系统 SHALL
   拒绝并返回广告网络非法业务码。
8. IF 用户提交广告网络为 `none`, THE 系统 SHALL 清空广告应用标识。
9. IF 用户提交的平台集合含未定义值, THE 系统 SHALL 拒绝并返回平台非法业务码。
10. IF 用户提交的平台集合为空, THE 系统 SHALL 将平台集合置为只含 `web`。

### Requirement 2: 自助打包授权

**User Story:** AS 登录用户, I want 不需要管理员介入即可触发打包, so that 我不必等待别人代打。

#### Acceptance Criteria

1. WHEN 登录用户提交打包请求, THE 系统 SHALL 仅校验该用户身份与该用户对该 App 的所有权，
   不校验管理员角色。
2. WHEN 登录用户提交打包请求, THE 系统 SHALL 不要求任何分组授权或团队授权。
3. IF 请求者身份缺失, THE 系统 SHALL 返回未授权业务码。
4. WHEN 登录用户列出构建任务, THE 系统 SHALL 只返回该用户自己名下的任务，
   不返回其他用户的任务。
5. WHEN 登录用户请求下载某个构建任务的产物, THE 系统 SHALL 校验该任务属于该用户，
   属于则返回产物字节流，不属于则返回任务不存在业务码。
6. IF 登录用户请求读取、发布或删除不属于该用户的构建任务, THE 系统 SHALL 返回任务不存在业务码。
7. WHEN 登录用户删除自己的构建任务, THE 系统 SHALL 执行软删除并保留审计记录。

### Requirement 3: 配额判定

**User Story:** AS 平台运营方, I want 按档位限制每个用户的构建次数, so that 构建资源不被无限制占用。

#### Acceptance Criteria

1. WHEN 用户提交打包请求, THE 系统 SHALL 在创建任务前完成配额判定。
2. IF 用户档位为免费档且该 App 构建次数大于等于 1, THE 系统 SHALL 拒绝创建任务并
   返回免费构建额度已用完业务码。
3. IF 用户档位为专业档且该用户当日构建任务数大于等于配置上限, THE 系统 SHALL
   拒绝创建任务并返回当日构建额度已用完业务码。
4. IF 用户档位为专业档且该用户当日构建任务数小于配置上限, THE 系统 SHALL 允许创建任务。
5. IF 用户请求打包平台为 `desktop-linux` 且权益的桌面许可为假, THE 系统 SHALL
   拒绝并返回桌面未授权业务码。
6. IF App 处于锁定状态且用户提交打包请求, THE 系统 SHALL 拒绝并返回 App 已锁定业务码。
7. IF 该用户已有任一非终态构建任务, THE 系统 SHALL 拒绝创建新任务并返回构建繁忙业务码。
8. IF 当日构建上限配置值小于等于 0, THE 系统 SHALL 将专业档视为不限次构建。

### Requirement 4: 构建任务与计数

**User Story:** AS 登录用户, I want 构建成功后计数、失败后可重试, so that 我的额度只花在真正成功的构建上。

#### Acceptance Criteria

1. WHEN 用户提交打包请求且配额判定通过, THE 系统 SHALL 创建任务并关联 App 主键、
   用户主键、平台、目标版本、版本码与状态 `queued`。
2. WHEN 构建任务结束且退出码为 0, THE 系统 SHALL 在单个事务内将任务状态置为 `succeeded`、
   写入产物引用与时长、将该 App 构建次数加一、将权益免费构建计数加一、写入最近构建时间、
   平台与产物引用。
3. WHEN 构建任务结束且退出码非 0, THE 系统 SHALL 将任务状态置为 `failed`、写入错误信息，
   并将 App 构建次数与权益免费构建计数保持不变。
4. WHEN 构建任务结束且免费档 App 构建次数达到 1, THE 系统 SHALL 将 App 锁定状态置为真。
5. IF 构建任务结束时的计数更新影响行数为 0, THE 系统 SHALL 记日志并返回数据库操作业务码，
   不回滚任务状态。
6. WHEN 用户读取构建任务详情, THE 系统 SHALL 返回任务状态、当前步骤、日志尾部、
   错误信息、产物引用与时长。

### Requirement 5: 产物下载与发布

**User Story:** AS 登录用户, I want 构建成功后直接拿到安装包, so that 我可以立即分发。

#### Acceptance Criteria

1. WHEN 用户请求下载构建任务产物且任务状态为 `succeeded` 且产物文件存在, THE 系统 SHALL
   返回二进制流与正确 MIME 类型与 Content-Disposition 文件名。
2. IF 产物文件缺失且任务状态为 `succeeded`, THE 系统 SHALL 返回产物缺失业务码。
3. IF 任务状态不为 `succeeded`, THE 系统 SHALL 返回任务未成功业务码。
4. WHEN 用户请求原生发布且任务产物存在, THE 系统 SHALL 写入发布清单并返回发布状态。
5. IF 用户请求 OTA 发布且系统配置未启用 OTA, THE 系统 SHALL 拒绝并返回 OTA 未启用业务码。
6. IF 用户请求发布的版本不高于当前已发布版本, THE 系统 SHALL 拒绝并返回版本过旧业务码。
7. IF 用户请求发布时目标平台不在该 App 的平台集合内, THE 系统 SHALL 拒绝并返回平台非法业务码。

### Requirement 6: 品牌与素材来源

**User Story:** AS 登录用户, I want 构建产物携带自己的品牌, so that 我分发的应用带我的标识。

#### Acceptance Criteria

1. WHEN 构建任务开始, THE 系统 SHALL 读取该 App 记录的图标键与包标识作为该次构建的
   品牌输入。
2. WHEN 构建任务开始, THE 系统 SHALL 读取实例级品牌配置的名称、版本号与版本码作为
   默认值，并由该 App 的版本字段覆盖。
3. IF 该 App 未设置图标键, THE 系统 SHALL 使用实例级品牌配置的素材，素材缺失则回落内置默认素材。
4. IF 素材引用的对象存储对象读取失败, THE 系统 SHALL 将任务置为 `failed` 并写入错误信息。

### Requirement 7: 管理端打包保留

**User Story:** AS 管理员, I want 保留为整个实例打包的能力, so that 我仍可发布面向全部用户的默认客户端。

#### Acceptance Criteria

1. WHEN 管理员在管理后台提交打包请求, THE 系统 SHALL 沿用现有品牌配置与现有工作区，
   不关联任何 App 记录。
2. WHEN 管理员提交打包请求, THE 系统 SHALL 不受用户配额判定约束。
3. WHEN 管理员读取构建任务列表, THE 系统 SHALL 可读取全部任务。
4. WHEN 系统同时存在管理端任务与用户端任务, THE 系统 SHALL 用单一单执行协程串行处理，
   同一时刻仅一个任务处于运行态。
5. WHEN 管理端任务与用户端任务争用工作区, THE 系统 SHALL 为每次任务使用独立的工作区副本，
   避免素材互相覆盖。

### Requirement 8: 用户端打包页面

**User Story:** AS 登录用户, I want 在用户中心完成打包与下载, so that 我无需访问管理后台。

#### Acceptance Criteria

1. WHEN 用户在用户中心进入某个 App, THE 系统 SHALL 展示该平台支持矩阵、品牌与版本字段、
   素材槽位、构建历史与下载入口。
2. WHEN 页面加载, THE 系统 SHALL 同时显示该用户档位与该 App 剩余可构建次数。
3. IF 该 App 已锁定, THE 系统 SHALL 禁用版本与素材编辑控件并提示重新解锁条件。
4. IF 该用户已有进行中的构建, THE 系统 SHALL 禁用提交按钮并显示当前进行中的任务步骤。
5. WHEN 用户提交打包, THE 系统 SHALL 轮询任务状态并在状态变化时刷新构建历史与剩余次数。
6. WHEN 构建成功, THE 系统 SHALL 展示可点击下载链接与产物 sha256 摘要。
7. IF 构建失败, THE 系统 SHALL 展示日志尾部与重试入口。
8. WHEN 页面渲染中英文文案, THE 系统 SHALL 保证英文与中文资源键叶子完全一致。
9. WHEN 页面加载, THE 系统 SHALL 在面包屑中显示该 App 名称。

## Non-Goals

- 不实现多平台并行构建，仍为单执行协程串行。
- 不实现 App 的实时预览，见 `app-live-preview`。
- 不实现 iOS 与 HarmonyOS 打包，见 `app-builder` 规格 Non-Goals。
- 不引入付费订阅系统，档位取值沿用 `entitlements.tier` 现有 `free` / `pro`。
- 不实现 App 的分享与转移所有权。
- 不实现构建产物的多版本保留策略，沿用现有单槽语义。
- 不修改管理员管理端打包页面的字段与交互。
