package usecase

import (
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/chaitin/MonkeyCode/backend/domain"
)

// adNetworksFileDefault is the deployed location of the ad network registry.
// The packer directory is mounted into the backend container at this path.
const adNetworksFileDefault = "/opt/authyide/src/packer/ad_networks.yaml"

// adNetworksFileEnv overrides the registry location, useful for local dev
// where the repo is mounted somewhere else.
const adNetworksFileEnv = "MCAI_AD_NETWORKS_FILE"

// adNetworksFallback is used when the registry cannot be read. It mirrors
// packer/ad_networks.yaml so the user center still works after a misconfiguration
// instead of returning an empty picker; the logger warns on every fallback.
var adNetworksFallback = []domain.AdNetworkOption{
	{ID: "none", DisplayName: "不展示广告", Platforms: []string{"web", "android", "ios", "harmonyos", "desktop"}},
	{ID: "admob", DisplayName: "Google AdMob", Vendor: "Google", Platforms: []string{"android", "ios", "web"}},
	{ID: "pangle", DisplayName: "穿山甲 Pangle", Vendor: "ByteDance", Platforms: []string{"android", "ios", "harmonyos"}},
	{ID: "gdt", DisplayName: "腾讯优量汇", Vendor: "Tencent", Platforms: []string{"android", "ios"}},
	{ID: "unity", DisplayName: "Unity Ads", Vendor: "Unity", Platforms: []string{"android", "ios"}},
	{ID: "applovin", DisplayName: "AppLovin MAX", Vendor: "AppLovin", Platforms: []string{"android", "ios"}},
}

// adNetworksFile is the yaml shape of packer/ad_networks.yaml. Only the fields
// the user center renders are declared; unknown keys are ignored by yaml.v3.
type adNetworksFile struct {
	Networks []struct {
		ID          string   `yaml:"id"`
		DisplayName string   `yaml:"displayName"`
		Vendor      string   `yaml:"vendor"`
		Platforms   []string `yaml:"platforms"`
	} `yaml:"networks"`
}

// loadAdNetworks reads the ad network registry. A missing or malformed file
// never breaks the user center: the caller gets the fallback list and the
// logger records the problem.
func loadAdNetworks(logger *slog.Logger) []domain.AdNetworkOption {
	path := os.Getenv(adNetworksFileEnv)
	if path == "" {
		path = adNetworksFileDefault
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		logger.Warn("failed to read ad networks registry, using built-in fallback",
			"error", err, "path", path)
		return adNetworksFallback
	}

	var file adNetworksFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		logger.Warn("failed to parse ad networks registry, using built-in fallback",
			"error", err, "path", path)
		return adNetworksFallback
	}

	options := make([]domain.AdNetworkOption, 0, len(file.Networks))
	for _, network := range file.Networks {
		options = append(options, domain.AdNetworkOption{
			ID:          network.ID,
			DisplayName: network.DisplayName,
			Vendor:      network.Vendor,
			Platforms:   network.Platforms,
		})
	}
	if len(options) == 0 {
		logger.Warn("ad networks registry is empty, using built-in fallback", "path", path)
		return adNetworksFallback
	}
	return options
}

// adNetworkSet builds the validation set from the registry ids.
func adNetworkSet(options []domain.AdNetworkOption) map[string]bool {
	set := make(map[string]bool, len(options))
	for _, option := range options {
		set[option.ID] = true
	}
	return set
}
