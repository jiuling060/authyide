// Package errcode 提供统一的错误码定义和国际化支持
package errcode

import (
	"embed"
	"net/http"

	"github.com/GoYoko/web"
)

// LocalFS 嵌入本地化文件系统
//
//go:embed locale.*.toml
var LocalFS embed.FS

// 框架层面的错误
var (
	ErrUnauthorized = web.NewErr(http.StatusUnauthorized, http.StatusUnauthorized, "err-unauthorized")
	ErrForbidden    = web.NewErr(http.StatusForbidden, http.StatusForbidden, "err-forbidden")
	ErrBadRequest   = web.NewErr(http.StatusBadRequest, http.StatusBadRequest, "err-bad-request")
)

// 业务层面的错误
var (
	// 通用的错误
	ErrInternalServer    = web.NewErr(http.StatusOK, 10000, "err-internal-server")
	ErrPermision         = web.NewErr(http.StatusOK, 10001, "err-permision-denied")
	ErrNotFound          = web.NewErr(http.StatusOK, 10002, "err-not-found")
	ErrDuplicate         = web.NewErr(http.StatusOK, 10003, "err-duplicate")
	ErrDatabaseQuery     = web.NewErr(http.StatusOK, 10004, "err-database-query")
	ErrDatabaseOperation = web.NewErr(http.StatusOK, 10005, "err-database-operation-failed")
	ErrHTTPRequest       = web.NewErr(http.StatusOK, 10006, "err-http-request-failed")
	ErrHasInvalidEntry   = web.NewErr(http.StatusOK, 10007, "err-has-invalid-entry")
	ErrStreamDisconnect  = web.NewErr(http.StatusOK, 10008, "err-stream-disconnect")

	// 业务模块特有的 错误码

	// 文件管理
	ErrFilePermisionDenied = web.NewErr(http.StatusOK, 10100, "err-file-permision-denied")
	ErrFileOp              = web.NewErr(http.StatusOK, 10101, "err-file-op")
	ErrFileTooLarge        = web.NewErr(http.StatusOK, 10102, "err-file-too-large")

	// 主机管理
	ErrVMIDRequired          = web.NewErr(http.StatusOK, 10200, "err-vm-id-required")
	ErrVMNotBelongToUser     = web.NewErr(http.StatusOK, 10201, "err-vm-not-belong-to-user")
	ErrPermisionDenied       = web.NewErr(http.StatusOK, 10202, "err-permision-denied")
	ErrInvalidInstallToken   = web.NewErr(http.StatusOK, 10203, "err-invalid-token")
	ErrPublicHostNotFound    = web.NewErr(http.StatusOK, 10204, "err-public-host-not-found")
	ErrHostOffline           = web.NewErr(http.StatusOK, 10205, "err-host-offline")
	ErrVMExpired             = web.NewErr(http.StatusOK, 10206, "err-vm-expired")
	ErrApplyPortFailed       = web.NewErr(http.StatusOK, 10207, "err-apply-port-failed")
	ErrRecyclePortFailed     = web.NewErr(http.StatusOK, 10208, "err-recycle-port-failed")
	ErrPublicHostCannotRenew = web.NewErr(http.StatusOK, 10209, "err-public-host-cannot-renew")
	ErrPublicHostBeyondLimit = web.NewErr(http.StatusOK, 10230, "err-public-host-beyond-limit")
	ErrVmRemoved             = web.NewErr(http.StatusOK, 10231, "err-vm-removed")
	ErrVmBeyondExpireTime    = web.NewErr(http.StatusOK, 10232, "err-vm-beyond-expire-time")

	// 模型配置管理
	ErrInvalidAPIKey    = web.NewErr(http.StatusOK, 10300, "err-model-id-required")
	ErrInvalidParameter = web.NewErr(http.StatusOK, 10301, "err-invalid-parameter")
	ErrModelNotFound    = web.NewErr(http.StatusOK, 10302, "err-model-not-found")
	ErrImageNotFound    = web.NewErr(http.StatusOK, 10303, "err-image-not-found")

	// 项目管理
	ErrInvalidPlatform            = web.NewErr(http.StatusOK, 10400, "err-invalid-platform")
	ErrInvalidToken               = web.NewErr(http.StatusOK, 10401, "err-invalid-token")
	ErrInvalidCollaborarator      = web.NewErr(http.StatusOK, 10402, "err-invalid-collaborarator")
	ErrUpdateProjectFailed        = web.NewErr(http.StatusOK, 10403, "err-update-project-failed")
	ErrGitOperation               = web.NewErr(http.StatusOK, 10404, "err-git-operation")
	ErrProjectGitIdentityRequired = web.NewErr(http.StatusOK, 10405, "err-project-git-identity-required")
	ErrRepoAlreadyLinked          = web.NewErr(http.StatusOK, 10406, "err-repo-already-linked")
	ErrGitIdentityInUseByProject  = web.NewErr(http.StatusOK, 10407, "err-git-identity-in-use-by-project")
	ErrForbiddenBaseURL           = web.NewErr(http.StatusOK, 10408, "err-forbidden-base-url")
	ErrCodeupOAuthNotSupported    = web.NewErr(http.StatusOK, 10409, "err-codeup-oauth-not-supported")

	// 团队管理
	ErrTeamMemberLimitExceeded = web.NewErr(http.StatusOK, 10500, "err-team-member-limit-exceeded")
	ErrInvalidPassword         = web.NewErr(http.StatusOK, 10501, "err-invalid-password")
	ErrSMSFailed               = web.NewErr(http.StatusOK, 10502, "err-sms-failed")
	ErrUserAlreadyExists       = web.NewErr(http.StatusOK, 10503, "err-user-already-exists")
	ErrChangePasswordFailed    = web.NewErr(http.StatusOK, 10504, "err-change-password-failed")
	ErrPasswordHashFailed      = web.NewErr(http.StatusOK, 10505, "err-password-hash-failed")
	ErrPasswordLength          = web.NewErr(http.StatusOK, 10506, "err-password-length")
	ErrTeamAdminLimitExceeded  = web.NewErr(http.StatusOK, 10507, "err-team-admin-limit-exceeded")
	ErrTeamUserDeleted         = web.NewErr(http.StatusOK, 10508, "err-team-user-deleted")

	// 用户管理
	ErrIdentityAlreadyBound          = web.NewErr(http.StatusOK, 10601, "err-identity-already-bound")
	ErrInvalidState                  = web.NewErr(http.StatusOK, 10602, "err-invalid-state")
	ErrNotLoggedIn                   = web.NewErr(http.StatusOK, 10603, "err-not-logged-in")
	ErrUserBlocked                   = web.NewErr(http.StatusOK, 10605, "err-user-blocked")
	ErrLoginFailed                   = web.NewErr(http.StatusOK, 10606, "err-login-failed")
	ErrInvalidCoupon                 = web.NewErr(http.StatusOK, 10607, "err-invalid-coupon")
	ErrResetPasswordFailed           = web.NewErr(http.StatusOK, 10608, "err-reset-password-failed")
	ErrWalletInsufficient            = web.NewErr(http.StatusOK, 10609, "err-wallet-insufficient")
	ErrAccountOverdraft              = web.NewErr(http.StatusOK, 10610, "err-account-overdraft")
	ErrEmailVerifyFailed             = web.NewErr(http.StatusOK, 10611, "err-email-verify-failed")
	ErrEmailAlreadyBound             = web.NewErr(http.StatusOK, 10612, "err-email-already-bound")
	ErrEmailTaken                    = web.NewErr(http.StatusOK, 10613, "err-email-taken")
	ErrEmailRequired                 = web.NewErr(http.StatusOK, 10614, "err-email-required")
	ErrEmailNotBound                 = web.NewErr(http.StatusOK, 10615, "err-email-not-bound")
	ErrEnterpriseResetPasswordDenied = web.NewErr(http.StatusOK, 10616, "err-enterprise-reset-password-denied")
	ErrOIDCDisabled                  = web.NewErr(http.StatusOK, 10617, "err-oidc-disabled")
	ErrOIDCConfigInvalid             = web.NewErr(http.StatusOK, 10618, "err-oidc-config-invalid")
	ErrOIDCStateInvalid              = web.NewErr(http.StatusOK, 10619, "err-oidc-state-invalid")
	ErrOIDCTokenInvalid              = web.NewErr(http.StatusOK, 10620, "err-oidc-token-invalid")
	ErrOIDCEmailRequired             = web.NewErr(http.StatusOK, 10621, "err-oidc-email-required")
	ErrOIDCEmailNotVerified          = web.NewErr(http.StatusOK, 10622, "err-oidc-email-not-verified")
	ErrOIDCEmailDomainDenied         = web.NewErr(http.StatusOK, 10623, "err-oidc-email-domain-denied")
	ErrOIDCTeamMemberRequired        = web.NewErr(http.StatusOK, 10624, "err-oidc-team-member-required")
	ErrPasswordLoginDisabled         = web.NewErr(http.StatusOK, 10625, "err-password-login-disabled")
	ErrOAuthLoginProviderDisabled    = web.NewErr(http.StatusOK, 10626, "err-oauth-login-provider-disabled")
	ErrOAuthLoginStateInvalid        = web.NewErr(http.StatusOK, 10627, "err-oauth-login-state-invalid")
	ErrOAuthLoginEmailRequired       = web.NewErr(http.StatusOK, 10628, "err-oauth-login-email-required")
	ErrOAuthLoginEmailUnverified     = web.NewErr(http.StatusOK, 10629, "err-oauth-login-email-unverified")
	ErrOAuthLoginRoleDenied          = web.NewErr(http.StatusOK, 10630, "err-oauth-login-role-denied")
	ErrOAuthLoginFailed              = web.NewErr(http.StatusOK, 10631, "err-oauth-login-failed")
	ErrOAuthLoginRedirectInvalid     = web.NewErr(http.StatusOK, 10632, "err-oauth-login-redirect-invalid")
	ErrRegisterEmailInvalid          = web.NewErr(http.StatusOK, 10633, "err-register-email-invalid")
	ErrRegisterPasswordInvalid       = web.NewErr(http.StatusOK, 10634, "err-register-password-invalid")
	ErrRegisterEmailTaken            = web.NewErr(http.StatusOK, 10635, "err-register-email-taken")
	ErrRegisterTokenInvalid          = web.NewErr(http.StatusOK, 10636, "err-register-token-invalid")
	ErrRegisterEmailUnverified       = web.NewErr(http.StatusOK, 10637, "err-register-email-unverified")
	ErrRegisterAlreadyVerified       = web.NewErr(http.StatusOK, 10638, "err-register-already-verified")

	// captcha 模块
	ErrCreateCaptchaFailed = web.NewErr(http.StatusOK, 10700, "err-create-captcha-failed")
	ErrRedeemCaptchaFailed = web.NewErr(http.StatusOK, 10701, "err-redeem-captcha-failed")
	ErrCaptchaVerifyFailed = web.NewErr(http.StatusOK, 10702, "err-captcha-verify-failed")

	ErrDepositFailed = web.NewErr(http.StatusOK, 10801, "err-deposit-failed")

	// 任务管理
	ErrTaskCannotDelete               = web.NewErr(http.StatusOK, 10810, "err-task-cannot-delete")
	ErrTaskConcurrencyLimit           = web.NewErr(http.StatusOK, 10811, "err-task-concurrency-limit")
	ErrModelAccessDenied              = web.NewErr(http.StatusOK, 10812, "err-model-access-denied")
	ErrTaskRoundsDirectionUnsupported = web.NewErr(http.StatusOK, 10813, "err-task-rounds-direction-unsupported")

	// 知识库索引管理
	ErrKBNilTree       = web.NewErr(http.StatusOK, 10900, "err-kb-nil-tree")
	ErrKBEmptyPath     = web.NewErr(http.StatusOK, 10901, "err-kb-empty-path")
	ErrKBPathNotFound  = web.NewErr(http.StatusOK, 10902, "err-kb-path-not-found")
	ErrKBNotFile       = web.NewErr(http.StatusOK, 10903, "err-kb-not-file")
	ErrKBNotDir        = web.NewErr(http.StatusOK, 10904, "err-kb-not-dir")
	ErrKBDirNotEmpty   = web.NewErr(http.StatusOK, 10905, "err-kb-dir-not-empty")
	ErrKBAlreadyExists = web.NewErr(http.StatusOK, 10906, "err-kb-already-exists")

	// 微信公众号
	ErrWechatMPNotBound = web.NewErr(http.StatusOK, 11200, "err-wechat-mp-not-bound")

	// License 管理
	ErrLicenseInvalid         = web.NewErr(http.StatusOK, 11300, "err-license-invalid")
	ErrLicenseMissing         = web.NewErr(http.StatusOK, 11301, "err-license-missing")
	ErrLicenseExpired         = web.NewErr(http.StatusOK, 11302, "err-license-expired")
	ErrLicenseNotReady        = web.NewErr(http.StatusOK, 11303, "err-license-not-ready")
	ErrLicenseMachineMismatch = web.NewErr(http.StatusOK, 11304, "err-license-machine-mismatch")

	// 用户中心 App
	ErrAppNotFound             = web.NewErr(http.StatusOK, 11400, "err-app-not-found")
	ErrAppLocked               = web.NewErr(http.StatusOK, 11401, "err-app-locked")
	ErrAppPackageIDConflict    = web.NewErr(http.StatusOK, 11402, "err-app-package-id-conflict")
	ErrAppBuildQuotaExceeded   = web.NewErr(http.StatusOK, 11403, "err-app-build-quota-exceeded")
	ErrAppInvalidPlatform      = web.NewErr(http.StatusOK, 11404, "err-app-invalid-platform")
	ErrAppAdNetworkInvalid     = web.NewErr(http.StatusOK, 11405, "err-app-ad-network-invalid")
	ErrBuildDailyQuotaExceeded = web.NewErr(http.StatusOK, 11406, "err-build-daily-quota-exceeded")
	ErrAppDesktopUnlicensed    = web.NewErr(http.StatusOK, 11407, "err-app-desktop-unlicensed")
	ErrMemberInviteUnavailable = web.NewErr(http.StatusOK, 11408, "err-member-invite-unavailable")

	// 移动端 APP 在线打包
	ErrBrandingInvalid      = web.NewErr(http.StatusOK, 11410, "err-branding-invalid")
	ErrAssetInvalid         = web.NewErr(http.StatusOK, 11411, "err-asset-invalid")
	ErrAssetSlotInvalid     = web.NewErr(http.StatusOK, 11412, "err-asset-slot-invalid")
	ErrBuildBusy            = web.NewErr(http.StatusConflict, 11413, "err-build-busy")
	ErrBuildNotFound        = web.NewErr(http.StatusOK, 11414, "err-build-not-found")
	ErrBuildArtifactMissing = web.NewErr(http.StatusOK, 11415, "err-build-artifact-missing")
	ErrReleaseVersionTooOld = web.NewErr(http.StatusOK, 11416, "err-release-version-too-old")
	ErrBuildNotSucceeded    = web.NewErr(http.StatusOK, 11417, "err-build-not-succeeded")
	ErrOtaDisabled          = web.NewErr(http.StatusOK, 11418, "err-ota-disabled")
	ErrWorkspaceInvalid     = web.NewErr(http.StatusOK, 11419, "err-workspace-invalid")
	// 桌面端打包
	ErrPlatformDisabled        = web.NewErr(http.StatusOK, 11420, "err-platform-disabled")
	ErrPlatformInvalid         = web.NewErr(http.StatusOK, 11421, "err-platform-invalid")
	ErrChannelInvalid          = web.NewErr(http.StatusOK, 11422, "err-channel-invalid")
	ErrChannelNotPublishable   = web.NewErr(http.StatusOK, 11423, "err-channel-not-publishable")
	ErrResourceInsufficient    = web.NewErr(http.StatusOK, 11424, "err-resource-insufficient")
	ErrBuildQueueFull          = web.NewErr(http.StatusOK, 11425, "err-build-queue-full")
	ErrBuildFailed          = web.NewErr(http.StatusOK, 11426, "err-build-failed")

	// 会话源码快照
	ErrSnapshotEmpty            = web.NewErr(http.StatusOK, 11430, "err-snapshot-empty")
	ErrSnapshotSyncFailed       = web.NewErr(http.StatusOK, 11431, "err-snapshot-sync-failed")
	ErrSnapshotTooLarge         = web.NewErr(http.StatusOK, 11432, "err-snapshot-too-large")
	ErrSnapshotNotFound         = web.NewErr(http.StatusOK, 11433, "err-snapshot-not-found")
	ErrSnapshotStale            = web.NewErr(http.StatusOK, 11434, "err-snapshot-stale")
	ErrSnapshotCorrupt          = web.NewErr(http.StatusOK, 11435, "err-snapshot-corrupt")
	ErrAppSourceSnapshotUnbound = web.NewErr(http.StatusOK, 11436, "err-app-source-snapshot-unbound")
)
