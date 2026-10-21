# APP 实时预览（App Live Preview）需求文档

Feature Name: app-live-preview
Updated: 2026-10-20
Status: draft

## Introduction

用户在用户中心配置 App 的名称、包标识与图标后，目前没有任何方式看到品牌落地后的
实际效果。`app-builder.tsx` 中唯一带「预览」字样的是上传素材的缩略图，
`frontend/src` 下 `LivePreview` 与「实时预览」零命中。用户必须完整跑一次
打包、下载 APK、装到真机才能看到自己的图标装进 App 的效果。

本特性在浏览器内用 iframe 承载 App 的 Web 渲染，用户改完品牌配置就能看到效果，
无需打包、无需真机。

移动端工程已具备 Web 渲染基础：`mobile/package.json` 已声明
`react-native-web ~0.21.0` 与 `react-dom 19.2.0`，`mobile/app.json`
的 `web` 段已存在（`bundler: metro`）。唯一的阻碍是 `output` 取值为
`single`，该模式面向原生容器内嵌，无法作为静态资源对外服务，
需要一份 Web 专用配置覆盖为 `static`。

## Glossary

- **预览包 (Preview Bundle)**: 一次 `expo export --platform web` 的静态导出
  结果，绑定一个资源指纹，见 `app_preview_bundles` 表。
- **资源指纹 (Asset Hash)**: 由 App 的图标键、名称、包标识、实例品牌版本与
  素材集合共同计算的 sha256，指纹不变则复用已有预览包。
- **手机框 (Phone Frame)**: 前端用 CSS 绘制的设备外观容器，尺寸 390×844，
  内部放置 iframe。
- **能力层 (Capability Layer)**: 声明每个原生模块在 Web 平台是否可用的清单，
  构建时收集，写入预览包元数据。
- **单飞 (Single-flight)**: 同一时刻只允许一个预览构建在跑，第二个请求
  复用第一个的执行结果。
- **打包平台 (Build Platform)**: 取值 `android` 或 `desktop-linux`，
  Web 不属于打包平台，只属于预览平台。

## Requirements

### Requirement 1: 预览渲染

**User Story:** AS 登录用户, I want 在浏览器里看到自己的 App 跑起来的样子, so that 我不必打包就能验证品牌效果。

#### Acceptance Criteria

1. WHEN 用户在用户中心进入某个 App, THE 系统 SHALL 在手机框容器中渲染该 App 的
   Web 预览。
2. WHEN 手机框渲染完成, THE 系统 SHALL 使用 iframe 承载预览包入口页，
   iframe 的 src 指向后端同源静态路径。
3. WHILE 预览包构建中, THE 系统 SHALL 在手机框位置显示构建进度与当前步骤，
   并在构建完成后自动加载 iframe。
4. IF 预览包构建失败, THE 系统 SHALL 在手机框位置显示失败原因与重试入口，
   不重试自动。
5. WHEN 预览包构建失败且错误信息指向 Web 平台无法渲染的模块, THE 系统 SHALL
   在失败提示中列出这些模块名。
6. WHEN 手机框渲染 iframe, THE 系统 SHALL 设置 sandbox 属性且关闭 popups 与
   popups-modals，保留 same-origin 与 allow-scripts。
7. WHEN 手机框加载 iframe, THE 系统 SHALL 使用响应式比例保持 390:844 的
   视口尺寸，允许用户调整手机框缩放比例。
8. WHEN 用户在 iframe 内点击返回或切换页面, THE 系统 SHALL 保持手机框外层的
   配置面板不被关闭。

### Requirement 2: 按需构建与缓存

**User Story:** AS 平台, I want 只有需要时才构建预览包，并且同配置复用, so that 构建资源不被预览消耗。

#### Acceptance Criteria

1. WHEN 用户请求预览 URL, THE 系统 SHALL 按 App 的资源指纹查询现有预览包。
2. WHEN 指纹命中且状态为 `succeeded` 且构建时间距今小于 24 小时, THE 系统 SHALL
   返回既有入口 URL，不创建构建任务。
3. WHEN 指纹未命中或既有预览包已过期, THE 系统 SHALL 创建预览构建任务并返回任务状态。
4. WHEN 预览构建任务成功, THE 系统 SHALL 将导出目录写入静态存储并写入预览包记录。
5. WHEN 预览构建任务失败, THE 系统 SHALL 记录错误信息并保留任务供重试。
6. IF 同一 App 已有预览构建任务在运行, THE 系统 SHALL 返回当前运行中的任务状态，
   不创建第二个任务。
7. IF 不同用户同时请求不同 App 的预览, THE 系统 SHALL 串行执行预览构建，
   不占用打包任务的工作区副本。
8. IF 预览构建超过 5 分钟, THE 系统 SHALL 终止构建、清理临时目录并将任务置为 `failed`。
9. IF 预览导出目录写入静态存储失败, THE 系统 SHALL 将任务置为 `failed` 并返回错误。
10. WHEN 用户删除某个 App, THE 系统 SHALL 级联删除该 App 的全部预览包记录与静态文件。

### Requirement 3: Web 平台能力层

**User Story:** AS App 作者, I want 知道哪些原生能力在 Web 预览里用不了, so that 我不会误以为线上效果就是这个。

#### Acceptance Criteria

1. WHEN 预览构建执行, THE 系统 SHALL 收集工程声明的原生模块清单及其 Web 平台
   可用性标记。
2. WHEN 预览包成功, THE 系统 SHALL 将不可用模块名列表写入预览包元数据。
3. WHEN 用户打开预览, THE 系统 SHALL 在手机框下方列出不可用模块清单。
4. WHEN 预览页面渲染到不可用能力, THE 系统 SHALL 渲染为带说明文字的占位块，
   说明文字指明该能力仅在原生平台可用。
5. IF 预览页面发生不可恢复的运行时错误, THE 系统 SHALL 在手机框内显示错误信息
   与堆栈首行，而不是白屏。
6. WHEN 预览页面需要原生深链唤起, THE 系统 SHALL 改用查询参数传递目标路由，
   不注册自定义 scheme。
7. WHEN 预览页面发起网络请求, THE 系统 SHALL 走后端同源路径，
   请求体带当前用户标识以通过既有鉴权中间件。

### Requirement 4: 移动端 Web 支持改造

**User Story:** AS App 作者, I want 我的 App 能导出为静态 Web 站点, so that 预览有东西可渲染。

#### Acceptance Criteria

1. WHEN 预览构建执行, THE 系统 SHALL 使用 Web 专用配置文件，该文件将 web.output
   覆盖为 `static`。
2. WHEN Web 专用配置文件被用于预览构建, THE 系统 SHALL 不影响原生打包流程使用的
   主配置文件。
3. WHEN 预览构建执行, THE 系统 SHALL 复用现有工程已声明的 `react-native-web` 与
   `react-dom` 依赖，不新增运行时依赖。
4. WHEN 预览构建执行, THE 系统 SHALL 只导出 Web 平台产物，
   不导出原生平台产物。
5. WHEN 导出产物生成, THE 系统 SHALL 将入口页作为静态站点根路径可访问。
6. IF 导出过程报告缺少 Web 依赖, THE 系统 SHALL 将任务置为 `failed` 并在错误信息中
   列出缺失的依赖名。

### Requirement 5: 预览入口与边界

**User Story:** AS 登录用户, I want 清楚地知道预览是什么以及它不能替代什么, so that 我不会把它当成品验证。

#### Acceptance Criteria

1. WHEN 页面展示预览入口, THE 系统 SHALL 标注预览为 Web 渲染形态，
   并标注该形态不覆盖原生能力。
2. WHEN 用户在预览页操作, THE 系统 SHALL 禁用与打包发布相关的全部按钮，
   预览页不提供打包入口。
3. WHEN 用户切换 App, THE 系统 SHALL 重新计算资源指纹并按需重新构建。
4. IF 该用户的档位不允许构建任何 App, THE 系统 SHALL 仍允许预览，
   预览不消耗构建额度。
5. IF App 已锁定, THE 系统 SHALL 仍允许预览现有配置效果，
   但不允许修改配置。
6. WHEN 预览构建占用构建机的磁盘, THE 系统 SHALL 在完成或失败后清理导出临时目录，
   仅保留静态存储中的正式产物。

## Non-Goals

- 不启动常驻 Metro 开发服务器，预览构建是按需触发的一次性导出。
- 不提供真机扫码预览，见 Expo Go 方案，本特性只走 Web 形态。
- 不支持原生平台（Android / iOS）预览，Web 是唯一的预览平台。
- 不支持在预览内做端到端的功能测试，能力层不完整的模块只渲染占位。
- 不实现预览的分享链接与公开访问，预览仅限归属用户本人。
- 不实现预览的多设备尺寸矩阵，只提供单一 390×844 手机框。
- 不实现预览截图导出。
- 不实现桌面端预览，见 `app-builder-desktop-runner`。
