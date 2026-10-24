// Package runner: platform dispatch for host prerequisite probes.
package runner

import (
	"context"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

// PrereqProbe carries the paths the prerequisite probes check.
type PrereqProbe struct {
	AndroidSDK       string
	JavaHome         string
	DesktopWorkspace string
}

// MissingPrereqs reports what platform cannot run with on this host. An empty
// slice means the platform is ready.
func MissingPrereqs(ctx context.Context, platform string, probe PrereqProbe) []string {
	switch platform {
	case domain.PlatformAndroid:
		return (&androidRunner{cfg: AndroidConfig{
			AndroidSDK: probe.AndroidSDK,
			JavaHome:   probe.JavaHome,
		}}).Precheck(ctx)
	case domain.PlatformDesktopLinux:
		return (&desktopRunner{cfg: DesktopConfig{
			DesktopWorkspace: probe.DesktopWorkspace,
		}}).Precheck(ctx)
	}
	return nil
}
