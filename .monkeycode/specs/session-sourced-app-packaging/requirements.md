# 会话源码打包（Session-Sourced App Packaging）需求文档

Feature Name: session-sourced-app-packaging
Updated: 2026-10-22
Status: draft

## Introduction

用户中心已能自助创建 App 并触发打包，但打包机使用的永远是实例级全局模板
`/opt/authyide/src/mobile`（即 AuthyIDE 客户端自身）：`EnqueueBuildReq` 只带
`platform` / `version` / `version_code` / `app_id`，`worker.workspaceFor` 每次
`cp -al` 一份模板，`runnerConfig` 没有任何文件注入入口。会话里写好的代码无法
进入打包机，会话与产物之间只有管理员手工搬运这一条路。

本特性补三件事：会话内新增「同步源码」动作，把当前会话工作区打成一份源码快照
存入对象存储；App 可以绑定一份快照；打包机在模板工作区上递归覆盖该快照后再执行
既有流水线。记账沿用既有 App 实体与配额，不新增任务表、不新增积分通道。

覆盖语义为递归覆盖：快照文件与模板同名时快照优先，模板独有的文件与目录保留。
模板仍承载 `app.json`、`android/`、`metro.config.js`、`babel.config.js`、
`plugins/` 等构建配置，会话代码须遵循模板约定的工程结构。

能力缺口：当前后端不存在 VM 到对象存储的源码导出通道，附件链是反向的
（后端签 presigned URL 供 VM 下载）。快照能力需要新增这一条通道，可复用既有
`biz/uploader` 的分前缀 presigned 上传机制，新增一个 `snapshot` 上传用途即可。

历史脉络：`app-self-service-packaging` 的 R7 AC1 要求管理端打包「沿用现有品牌
配置与现有工作区」，本特性不修改管理端行为；R7 AC1 描述的「沿用现有工作区」在
用户端被「模板 + 快照覆盖」替代。既有 `app-self-service-packaging` 的 R6（品牌与
素材来源）与 R3（配额判定）在本特性中原样生效。

## Glossary

- **会话 (Session)**: 用户从某个客户端发起、承载连续交互及其模型与工具调用的
  工作上下文。
- **会话工作区**: 该会话在虚拟机内的源码目录，即用户在该会话中编写代码的落点。
- **源码快照 (Source Snapshot)**: 会话工作区某一刻的文件集合，打包后存入对象
  存储的一份不可变副本，可被一个或多个 App 引用。
- **快照同步**: 在会话内发起、把当前会话工作区打成快照并上传到对象存储的动作。
- **快照过期 (Stale)**: 会话工作区在快照创建后又发生过变更的状态。
- **App**: 用户在用户中心创建的跨平台应用记录，一行一条，归属单一用户，见
  `apps` 表。
- **打包 (Build)**: 以 App 记录为目标、由打包机执行 `expo prebuild` 与
  `assembleRelease` 产出可安装产物的过程，见 `app_build_jobs` 表。
- **模板工作区**: 实例级全局 Expo 工程 `/opt/authyide/src/mobile`，是打包的
  基线工作区，携带品牌、版本号、prebuild 配置与构建脚本。
- **递归覆盖 (Overlay)**: 以模板工作区为基底，将快照中的文件按相对路径写入工作区
  副本；相对路径相同的条目由快照覆盖模板，模板独有的条目保留。
- **配额 (Build Quota)**: 档位决定的构建次数上限，见
  `app-self-service-packaging` 的 R3。

## Requirements

### Requirement 1: 会话源码快照

**User Story:** AS 登录用户, I want 在会话内把当前代码同步成一份快照, so that
我打包时用的是会话里此刻的代码，并且这份代码不依赖会话存活。

#### Acceptance Criteria

1. WHEN 用户在某个会话内发起快照同步, THE 系统 SHALL 递归收集该会话工作区的源码文件
   并打包为一份快照存入对象存储，快照条目保存会话主键、快照主键、归属用户主键、
   文件数、字节数、sha256 摘要、创建时间与状态。
2. WHEN 快照同步完成, THE 系统 SHALL 将快照状态置为成功，并让该会话下更早的快照
   标记为已被取代，只保留最新一份作为该会话的当前快照。
3. IF 会话工作区为空, THE 系统 SHALL 拒绝创建快照并返回快照为空业务码。
4. IF 会话工作区不可读或打包过程失败, THE 系统 SHALL 将本次同步标记为失败并返回
   快照同步失败业务码，不产生可被 App 绑定的快照记录。
5. IF 会话工作区打包字节数超过配置上限, THE 系统 SHALL 拒绝创建并返回快照超限业务码。
6. WHEN 同一会话重复发起快照同步, THE 系统 SHALL 创建新的快照记录，
   不覆盖历史快照。
7. WHEN 快照上传完成, THE 系统 SHALL 记录快照在对象存储中的对象键，
   供后续构建任务读取。

### Requirement 2: App 绑定源码快照

**User Story:** AS 登录用户, I want 把我某个会话的快照绑定到自己的 App 上, so that
该 App 打包时用的是这份代码。

#### Acceptance Criteria

1. WHEN 用户为 App 设置源码快照, THE 系统 SHALL 校验该快照归属该用户，
   通过后记录 App 的快照主键与该快照的会话主键。
2. IF 用户为 App 设置的快照归属其他用户, THE 系统 SHALL 返回快照不存在业务码。
3. WHEN 用户清除 App 的源码快照, THE 系统 SHALL 清空该 App 的快照主键，
   后续该 App 的打包回落到模板工作区。
4. IF App 处于锁定状态且用户修改快照绑定, THE 系统 SHALL 拒绝并返回 App 已锁定业务码。
5. WHEN 用户读取某个 App, THE 系统 SHALL 返回该 App 当前绑定的快照主键、
   来源会话标题、快照 sha256 摘要、快照创建时间与快照过期状态。
6. IF App 未绑定源码快照, THE 系统 SHALL 返回空的快照主键，
   并在打包页明确标识该 App 使用模板工作区。

### Requirement 3: 打包来源与递归覆盖

**User Story:** AS 登录用户, I want 打包时把我的会话代码叠加到模板上, so that
我分发的应用承载我在会话里写的代码，同时保留模板的构建与品牌能力。

#### Acceptance Criteria

1. WHEN 构建任务创建, THE 系统 SHALL 记录该次构建依据的快照主键与来源会话主键，
   作为产物溯源信息。
2. WHEN 构建任务开始且该 App 绑定了源码快照, THE 系统 SHALL 以模板工作区副本为基底，
   将快照文件按相对路径递归覆盖到副本；相对路径相同的条目以快照为准，
   模板独有的条目保留。
3. WHEN 构建任务开始且该 App 未绑定源码快照, THE 系统 SHALL 直接使用模板工作区副本，
   行为与既有规格一致。
4. WHEN 覆盖步骤完成, THE 系统 SHALL 再依次执行品牌注入、配置重写、
   `expo prebuild` 与 `assembleRelease`，使 App 的名称、包标识与版本号覆盖会话
   携带的同名配置。
5. IF 快照在对象存储中读取失败, THE 系统 SHALL 将任务置为 failed 并写入错误信息。
6. IF 快照内存在绝对路径或包含 `..` 的条目, THE 系统 SHALL 中止覆盖并将任务置为
   failed，不写出该快照覆盖出的工作区内容。
7. IF 覆盖后的工作区缺少 `app.json` 或 `android/gradlew`, THE 系统 SHALL 将任务置为
   failed 并写入错误信息。
8. WHEN 构建任务结束, THE 系统 SHALL 清理该次任务的工作区副本，
   不向后续任务残留覆盖结果。

### Requirement 4: 会话侧同步入口与状态

**User Story:** AS 登录用户, I want 在会话里知道打包用的是哪份代码, so that
我不会把旧代码打进产物。

#### Acceptance Criteria

1. WHEN 用户打开某会话的打包入口, THE 系统 SHALL 显示该会话当前快照的来源会话标题、
   创建时间、文件数与 sha256 摘要；该会话没有快照时展示未同步状态。
2. WHEN 用户点击同步源码, THE 系统 SHALL 显示同步进行状态，
   并在完成后刷新快照信息与 App 页面的快照显示。
3. IF 快照同步失败, THE 系统 SHALL 展示失败原因并提供重试入口。
4. IF 会话的最后活动时间晚于其当前快照的创建时间, THE 系统 SHALL 将该快照标记为
   过期。
5. WHEN 用户为绑定过期快照的 App 提交打包, THE 系统 SHALL 禁用提交入口并提示先
   在会话内同步源码。
6. WHEN 用户在会话内查看已绑定该会话快照的 App, THE 系统 SHALL 列出这些 App，
   并在快照过期时给出对应提示。

### Requirement 5: 归属与隔离

**User Story:** AS 平台运营方, I want 快照按用户隔离, so that 用户无法打包他人的
会话代码。

#### Acceptance Criteria

1. WHEN 用户发起快照同步, THE 系统 SHALL 校验该会话归属该用户。
2. IF 用户请求读取、同步或绑定其他用户会话的快照, THE 系统 SHALL 返回会话不存在
   业务码。
3. WHEN 构建任务开始, THE 系统 SHALL 校验该 App 绑定的快照归属该 App 的归属用户，
   不一致则将任务置为 failed。
4. WHEN 用户列出可绑定的快照, THE 系统 SHALL 只返回该用户名下会话产生的快照。
5. WHEN 用户浏览会话文件区域的普通文件列表, THE 系统 SHALL 只返回用户上传的文件，
   快照对象保存在独立前缀下，不列入该列表，也不接受普通删除请求。

### Requirement 6: 打包记账沿用 App 实体

**User Story:** AS 登录用户, I want 会话源码打包和模板打包共用同一套额度, so that
我只需要关心一个 App 的构建次数。

#### Acceptance Criteria

1. WHEN 用户为绑定了源码快照的 App 提交打包, THE 系统 SHALL 与既有 App 打包一致地
   执行配额判定，按该 App 的构建计数判定是否拒绝。
2. WHEN 绑定源码快照的构建任务成功, THE 系统 SHALL 将该 App 的构建计数加一，
   并按既有规则更新档位免费构建计数。
3. IF 免费档 App 因源码打包成功达到构建计数上限, THE 系统 SHALL 将该 App 置为锁定。
4. WHEN 绑定源码快照的构建任务失败, THE 系统 SHALL 保持该 App 的构建计数与
   档位免费构建计数不变。

### Requirement 7: 用户中心打包页展示

**User Story:** AS 登录用户, I want 在打包前明确看到这次要打包的是哪份代码, so that
我提交的版本可追溯。

#### Acceptance Criteria

1. WHEN 用户在用户中心进入某个 App, THE 系统 SHALL 展示该 App 的源码来源标识：
   绑定快照时显示会话标题、创建时间、文件数与摘要，未绑定快照时显示使用模板工作区。
2. IF 该 App 绑定的快照已过期, THE 系统 SHALL 禁用提交按钮并提示先在会话内同步源码。
3. WHEN 用户提交打包并等待任务结束, THE 系统 SHALL 在任务详情中显示本次构建所依据的
   快照主键与来源会话标题。
4. IF 该 App 已锁定, THE 系统 SHALL 禁用快照绑定控件，
   展示与既有规格一致的重新解锁条件。

## Assumptions

- 会话工作区内的工程结构与模板约定一致，可直接叠加；不满足时构建失败并把日志尾部
  展示给用户。
- 对象存储在实例中可用且配额充足，快照作为普通对象长期保留。
- 会话工作区打包字节数的配置上限沿用既有对象存储大小上限配置项，
  本特性只读取该值，不新增独立上限。
- 快照同步由会话侧执行文件收集与上传，后端只负责签发上传凭证与登记快照记录。

## Non-Goals

- 不实现 VM 到打包机的实时文件拉取通道。
- 不实现多平台并行构建，仍为单执行协程串行。
- 不实现 iOS 与 HarmonyOS 打包。
- 不实现快照版本管理，保留历史但不做回滚与差异对比。
- 不实现会话代码与模板约定的静态校验，以构建结果作为唯一反馈。
- 不实现多个 App 共享同一份代码的不同打包形态。
- 不修改管理员管理端打包页面与打包行为。
- 不修改品牌与素材注入逻辑，仍沿用 `app-self-service-packaging` 的 R6。
- 不新增配额档位或积分通道，复用既有 `entitlements` 判定。

## Open Questions

本轮已无未决项，四项原待确认问题按以下决策定案：

1. 快照大小上限：读取既有 `ObjectStorage.MaxSize`，与上传器、技能包、扩展包共用同一
   数值，不新增配置项、不改 compose。超限返回快照超限业务码。
2. 历史快照生命周期：随会话永久保留，系统不执行自动清理。R1 AC6 记录的被取代快照
   保留在库中供溯源，不物理删除。
3. 快照对象可见性：快照存于对象存储的独立前缀，不列入会话文件区域的普通文件列表，
   不接受普通删除请求。用户释放引用的唯一途径是清除 App 的快照绑定（R2 AC3），
   被取代快照的物理回收不在本特性范围。
4. `app.json` 覆盖语义：会话携带的 `app.json` 会被覆盖，App 的名称、包标识、
   版本号以 App 记录为准（R3 AC4）。品牌控制权归 App 实体，会话文件不覆盖品牌字段。
