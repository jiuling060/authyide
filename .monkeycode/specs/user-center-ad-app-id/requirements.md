# 用户中心管理广告应用 ID 需求

## 简介

AuthyIDE 用户中心的每个应用增加"广告配置"能力：用户填入自己从第三方广告平台注册到的应用 ID，平台只负责代存并在后续打包构建时把该 ID 嵌入生成的应用。平台本身不提供广告投放、结算、审核或 SDK 托管。

与之相对，上一版引入的"广告位映射"（按平台 × banner/interstitial/rewarded/native 填写广告位 ID）和应用打包构建管线（Flutter worker、app_build_jobs 表、产物上传）均不在本版本交付范围内，一并移除。

**入口位置**：用户中心设置对话框新增「广告」Tab，与 identities/models/images/hosts/vms/notifications 平齐。在该 Tab 内先选择应用，再编辑该应用的广告网络和应用 ID。原应用编辑页（`/console/apps`）的「广告」Tab 和「打包」Tab 一并移除。

## 术语

- **广告网络**：用户自己的广告平台账号所属的平台。可选值为 `none`（不接入广告）、`admob`（Google AdMob）、`pangle`（穿山甲）、`gdt`（腾讯广告）、`unity`（Unity Ads）、`applovin`（AppLovin）。集合必须与 `packer/ad_networks.yaml` 保持一致。
- **应用 ID**：用户在广告网络后台为自己的应用注册到的唯一标识（例如 AdMob 的 `ca-app-pub-xxxx~yyyy`）。属于用户自己的账号，平台不校验其有效性。
- **已锁定应用**：免费额度已用尽、转为只读的应用。用户无法修改其任何配置。
- **打包构建管线**：根据应用配置调用 Flutter 生成可安装产物（APK、IPA、EXE 等）的完整链路，包含后端任务表、打包 worker、对象存储产物和前端打包记录页。

## 需求

### 需求 1：广告平台与应用 ID 配置

**用户故事：** 作为 AuthyIDE 用户，我希望在用户中心设置页选择某个应用并为其选择广告平台、填入应用 ID，以便该应用接入我自己的广告账号。

#### 验收标准

1. WHEN 用户在用户中心设置对话框切换到「广告」Tab，AuthyIDE SHALL 展示应用选择下拉框、广告平台选择器和应用 ID 输入框。
2. WHEN 用户未选择任何应用，AuthyIDE SHALL 隐藏广告平台选择器和应用 ID 输入框，并展示"请选择一个应用"的提示。
3. WHEN 用户选择一个应用，AuthyIDE SHALL 加载该应用当前的广告网络和应用 ID，并渲染到对应控件中。
4. WHEN 用户选择「不接入广告」，AuthyIDE SHALL 隐藏应用 ID 输入框，并将应用 ID 输入框的当前值丢弃。
5. WHEN 用户从下拉选择器选择一个广告平台，AuthyIDE SHALL 展示应用 ID 输入框，并在输入框下方显示该平台的应用 ID 获取提示文案。
6. WHEN 用户在应用 ID 输入框中输入内容，AuthyIDE SHALL 实时保留输入内容，直到用户保存或切换应用。
7. WHEN 用户在应用 ID 输入框中输入内容后删除为空白，AuthyIDE SHALL 将空白视为"未填写应用 ID"。
8. WHILE 所选应用处于已锁定状态，AuthyIDE SHALL 禁用广告平台选择器和应用 ID 输入框，并在区域顶部展示锁定原因提示。
9. WHEN 用户保存应用配置，AuthyIDE SHALL 将当前应用 ID、当前选择的广告网络和应用 ID 一并提交到后端。

### 需求 2：广告 ID 的校验、持久化与回显

**用户故事：** 作为 AuthyIDE 用户，我希望我填写的应用 ID 能被正确保存并在下次打开时原样回显，同时平台不会因无效 ID 拒绝保存。

#### 验收标准

1. WHEN 后端接收到应用更新请求且请求体包含 `ad_network`，AuthyIDE SHALL 校验 `ad_network` 的值属于广告网络集合，否则返回 `400`。
2. WHEN 后端接收到应用更新请求且请求体包含 `ad_app_id`，AuthyIDE SHALL 将 `ad_app_id` 原样持久化到当前应用，不校验其格式或真实性。
3. WHEN `ad_network` 为 `none`，AuthyIDE SHALL 将当前应用的 `ad_app_id` 设置为空值。
4. WHEN 请求体未携带 `ad_app_id` 字段，AuthyIDE SHALL 保留当前应用已有的 `ad_app_id` 值。
5. WHEN 后端返回应用详情，AuthyIDE SHALL 在响应体中携带 `ad_network` 和 `ad_app_id` 字段。
6. WHEN 应用详情中的 `ad_app_id` 为空值，前端 SHALL 渲染为空字符串而非 `null`。
7. WHEN 保存请求因网络或后端错误失败，AuthyIDE SHALL 保留用户已输入的广告网络和应用 ID 值，不恢复到旧值。
8. WHEN 用户切换所选应用，AuthyIDE SHALL 丢弃未保存的编辑内容，并加载新选应用的实际配置。

### 需求 3：移除广告位映射能力

**用户故事：** 作为 AuthyIDE 开发者，我希望系统不再包含广告位映射相关字段和界面，以免用户误填或维护死代码。

#### 验收标准

1. WHEN 用户保存应用配置，AuthyIDE SHALL 不在请求体中发送 `ad_placements` 字段。
2. WHEN 后端接收到应用更新请求，AuthyIDE SHALL 忽略请求体中的 `ad_placements` 字段，不将其写入数据库。
3. WHEN 后端返回应用详情，AuthyIDE SHALL 不在响应体中携带 `ad_placements` 字段。
4. WHEN 执行数据库迁移，AuthyIDE SHALL 从 `apps` 表移除 `ad_placements` 列（若该列存在）。
5. WHEN 用户中心设置对话框的「广告」Tab 渲染完成，AuthyIDE SHALL 不展示按平台分组的广告位 ID 输入网格。
6. WHEN 用户在应用 ID 输入框中输入内容，AuthyIDE SHALL 将该内容视为整个应用的单一应用 ID，而非某个广告位的 ID。

### 需求 4：移除应用打包构建管线

**用户故事：** 作为 AuthyIDE 开发者，我希望移除本版本不交付的应用打包构建管线，以免用户看到不可用的入口并减少维护负担。

#### 验收标准

1. WHEN 用户打开应用详情页，AuthyIDE SHALL 只展示一个 Tab，不展示「广告」Tab 和「打包」Tab。
2. WHEN 用户打开应用详情页，AuthyIDE SHALL 不展示打包记录列表、打包发起控件和产物下载链接。
3. WHEN 后端接收到针对应用的构建请求，AuthyIDE SHALL 返回 `404`。
4. WHEN 后端启动时，AuthyIDE SHALL 不启动应用打包 worker 进程。
5. WHEN 执行数据库迁移，AuthyIDE SHALL 移除 `app_build_jobs` 表（若该表存在）。
6. WHEN 执行 `go build ./...`，代码仓库中 SHALL 不存在对 `app_build_jobs` 表、`cmd/appworker` 和 `pkg/appbuild` 包的引用。
7. WHEN 执行前端类型检查，代码仓库中 SHALL 不存在对构建状态、构建日志和产物字段的引用。
8. WHEN 执行 `go test ./...` 和前端测试，移除后的代码 SHALL 通过全部现有测试，不引入新的测试失败。

### 需求 5：广告网络列表的单一权威源

**用户故事：** 作为 AuthyIDE 开发者，我希望广告平台选项只在一处定义，以免前端下拉框与后端校验列表不一致。

#### 验收标准

1. WHEN 后端启动，AuthyIDE SHALL 读取 `packer/ad_networks.yaml` 中的网络列表并作为广告网络集合的权威定义。
2. WHEN 前端渲染应用选择或广告平台下拉框，AuthyIDE SHALL 通过后端获取广告网络列表，不在前端硬编码。
3. WHEN `packer/ad_networks.yaml` 中新增一个网络，AuthyIDE SHALL 在重启后端后使其自动出现在前端下拉框中，无需修改前端代码。
4. WHEN 后端接收到不在列表中的 `ad_network` 值，AuthyIDE SHALL 返回 `400`。

## 非需求（明确不做）

- 平台不提供广告投放、计费、结算、审核能力。
- 平台不托管广告 SDK，也不验证用户填写的应用 ID 是否真实有效。
- 本版本不交付 Flutter 打包构建能力，因此广告 ID 只在用户配置层面存在，暂不实际嵌入任何产物。
- 不提供广告位级别的 ID 配置（banner/interstitial/rewarded/native × 各平台）。
- 不限制每个用户可填写的广告网络数量，每个应用只接入一个广告网络。
- 应用 ID 按应用归属，不提供用户级别的共享广告 ID。