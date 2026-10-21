package consts

// PlanTier 套餐档位。free 表示未开通任何付费套餐。
type PlanTier string

const (
	PlanTierFree  PlanTier = "free"
	PlanTierBasic PlanTier = "basic"
	PlanTierPro   PlanTier = "pro"
	PlanTierUltra PlanTier = "ultra"
)

// PlanHostQuota 各套餐允许同时在线的宿主机（会话虚拟机）上限。
// 免费 1 台，付费从 2 台起步，依次翻倍。
var PlanHostQuota = map[PlanTier]int{
	PlanTierFree:  1,
	PlanTierBasic: 2,
	PlanTierPro:   4,
	PlanTierUltra: 8,
}

// HostQuotaForTier 返回套餐对应的主机上限，未知或空档位按免费档处理。
func HostQuotaForTier(tier PlanTier) int {
	if n, ok := PlanHostQuota[tier]; ok {
		return n
	}
	return PlanHostQuota[PlanTierFree]
}

// FreeAppBuildsPerApp 免费档每个 App 允许自助打包成功的次数。
const FreeAppBuildsPerApp = 1
