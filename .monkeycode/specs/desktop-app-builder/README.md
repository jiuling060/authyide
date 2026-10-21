# desktop-app-builder 已合并进 app-builder

桌面端打包是移动端打包能力的平台扩展，共用打包槽位、任务状态机、日志尾部、权限收敛与审计约束。为避免两份 spec 之间出现权威冲突，本目录的需求与设计已整体合并到单一权威文档：

- 需求：`.monkeycode/specs/app-builder/requirements.md`
  - Requirement 1 平台能力与前置检查
  - Requirement 3 桌面品牌配置读写
  - Requirement 6 桌面在线打包
  - Requirement 8 打包机资源约束
  - Requirement 10 产物下载（渠道维度）
  - Requirement 12 桌面升级发布
  - Requirement 16 管理后台打包页面（平台维度）
- 设计：`.monkeycode/specs/app-builder/design.md`
  - Runner 抽象（`BuildRunner` 接口 + `android.go` / `desktop.go` / `registry.go`）
  - 桌面端流水线（五步 + `beforeBuildCommand` 覆盖配置）
  - 数据模型（`000033_create_app_build_artifacts`、`000034_create_desktop_branding`）
  - 配置项（桌面端 9 个新键）
  - 实测基线（三件套产物清单、包内布局、耗时、磁盘约束、引擎源码状态）
  - 错误处理（11420-11424）

后续桌面端打包的需求变更与设计演进，一律在 `app-builder/` 目录下进行。
