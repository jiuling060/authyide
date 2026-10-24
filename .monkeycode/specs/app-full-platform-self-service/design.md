# 全端用户自助打包

## 目标

用户中心的应用可以自助打包**系统具备构建能力的全部端**，即 `android` 与 `desktop-linux`。
`ios` 与 `harmonyos` 仍是应用平台集合里的目标标记，构建层没有对应 runner（需要 Xcode、DevEco
工具链），本次不接构建链路。

## 现状缺口

1. 前端提交体写死 `platform: "android"`（`app-builds-panel.tsx`）。
2. 平台词汇两层不通：应用层 `desktop`，构建层 `desktop-linux`。
3. `entitlements.desktop_licensed` 字段存在但无人读取。
4. 自服务下载/发布不透传 channel，Content-Type 硬编码 APK 媒体类型。
5. 桌面品牌是管理员单例，用户产物会共用 `AuthyIDE` / `com.authyide.app`。
6. 发布写全局目录，多用户互相覆盖版本清单。

## 平台归一化

`mobilebuild/domain` 增加 `PlatformDesktop = "desktop"` 与 `NormalizePlatform`：

```go
func NormalizePlatform(platform string) string {
	switch strings.TrimSpace(platform) {
	case PlatformDesktop:
		return PlatformDesktopLinux
	default:
		return strings.TrimSpace(platform)
	}
}
```

`PlatformValid` 走归一化；`enqueueBuild` 入队前归一化 `req.Platform`；
`platformEnabled` 对 `enabled_platforms` 每项也归一化，使 `["web","desktop"]` 能匹配
`desktop-linux`。job 行与所有返回体一律存归一化后的构建层值。

## 数据模型

新增表 `app_desktop_branding`，一行一个应用。迁移编号 `000037`：

```sql
CREATE TABLE app_desktop_branding (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid() NOT NULL,
    app_id            uuid NOT NULL,
    product_name      text NOT NULL DEFAULT '',
    bundle_identifier text NOT NULL DEFAULT '',
    created_at        timestamp with time zone DEFAULT now() NOT NULL,
    updated_at        timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at        timestamp with time zone
);
CREATE UNIQUE INDEX uq_app_desktop_branding_app
    ON app_desktop_branding USING btree (app_id) WHERE (deleted_at IS NULL);
```

图标不新增槽位，复用 `apps.icon_key` 与既有 `UploadAppIcon` 通道，桌面与移动共用同一张图。
升级端点不落库，由后端按应用派生。

默认值：`product_name` 回落 `apps.name`；`bundle_identifier` 回落
`apps.package_id + ".desktop"`，保证始终是合法的反向域名。

## 发布路径按应用隔离

| 场景 | 产物目录 | 清单 |
|---|---|---|
| 管理员 移动 | `mobile/<version>/authyide.apk` | `app-version/android.json` |
| 管理员 桌面 | `desktop/<version>/<name>` | `desktop/latest-linux.json` |
| 用户 移动 | `mobile/<app_id>/<version>/authyide.apk` | `app-version/<app_id>/android.json` |
| 用户 桌面 | `desktop/<app_id>/<version>/<name>` | `desktop/<app_id>/latest-linux.json` |

清单 URL 随之派生：`<base><prefix>/app-version/<app_id>/android.json`、
`<base><prefix>/desktop/<app_id>/latest.json`（Tauri updater 追加 `-linux` 后落盘为
`latest-linux.json`）。

## 准入

`app.usecase.canBuild` 增加平台维度判定：请求平台归一化后为 `desktop-linux` 且
`entitlements.desktop_licensed` 为假，返回 `errcode.ErrAppDesktopUnlicensed`。

## 用户中心 API

```
GET  /api/v1/users/apps/:id/builds/platforms
     → [{platform, enabled, reason, updater_endpoint?}]
GET  /api/v1/users/apps/:id/builds/branding
     → {product_name, bundle_identifier}
PUT  /api/v1/users/apps/:id/builds/branding   body {product_name?, bundle_identifier?}
GET  /api/v1/users/apps/:id/builds/:jobID/download?channel=
POST /api/v1/users/apps/:id/builds/:jobID/publish?channel=
```

`/platforms` 聚合判定：模块启用 && 平台门控 && 应用平台集合含该平台 &&
（桌面时）桌面许可。`reason` 为不可用原因。

## 任务详情 DTO

`AppBuildJob` 增加：

```go
Artifacts []AppBuildArtifactInfo `json:"artifacts,omitempty"`

type AppBuildArtifactInfo struct {
	Channel     string `json:"channel"`
	FileName    string `json:"file_name"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"download_url"`
	Publishable bool   `json:"publishable"`
}
```

`DownloadBuild` / `PublishBuild` 增加 `channel` 参数；Content-Type 由后缀决定，
移动端 apk 与桌面端 appimage/deb/rpm 走同一映射函数。

## 构建注入

worker 的 desktop 分支：`job.AppID != uuid.Nil` 时读取该应用的桌面品牌，取到即用，
取不到回落管理员单例。注入 `ProductName` / `BundleID` / `UpdaterEndpoint` / `IconPath`
四项。桌面 runner 把图标重采样到 `icons/` 下 Linux 各尺寸槽位，
`tauri.build.conf.json` 覆盖 `productName` / `identifier` / `version` / updater endpoints。

## 不做

- `ios` / `harmonyos` 构建链路。
- 桌面升级包签名：`UpdaterSigningKeyPath` 未配置时清单 `signature` 为空并记告警，
  客户端拒绝更新，产物保留为版本记录与下载入口。
- 构建机依赖安装与端到端出包验证：引擎源码不可得，`make linux` 断在 `check-ohmy-src`。
