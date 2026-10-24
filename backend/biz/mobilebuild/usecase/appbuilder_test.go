package usecase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

func TestValidateDesktopBranding(t *testing.T) {
	base := &domain.DesktopBranding{
		ProductName:      "AuthyIDE",
		BundleIdentifier: "com.authyide.app",
	}
	cases := []struct {
		name string
		b    *domain.DesktopBranding
		want bool
	}{
		{"valid", base, true},
		{"blank name", withProduct("", base), false},
		{"long name", withProduct(strings.Repeat("a", 101), base), false},
		{"max name", withProduct(strings.Repeat("a", 100), base), true},
		{"blank bundle id", withBundle("", base), false},
		{"single segment", withBundle("authyide", base), false},
		{"leading dot", withBundle(".authyide", base), false},
		{"numeric segment", withBundle("com.1authyide", base), false},
		{"long bundle id", withBundle(strings.Repeat("com.", 80)+"app", base), false},
		{"bad endpoint", withEndpoint("not a url", base), false},
		{"empty endpoint", withEndpoint("", base), true},
		{"http endpoint", withEndpoint("http://example.test/desktop/latest.json", base), true},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateDesktopBranding(tc.b); (err == nil) != tc.want {
				t.Fatalf("validateDesktopBranding(%+v) error = %v, want valid=%v", tc.b, err, tc.want)
			}
		})
	}
}

func withProduct(name string, b *domain.DesktopBranding) *domain.DesktopBranding {
	out := *b
	out.ProductName = name
	return &out
}

func withBundle(bundleID string, b *domain.DesktopBranding) *domain.DesktopBranding {
	out := *b
	out.BundleIdentifier = bundleID
	return &out
}

func withEndpoint(endpoint string, b *domain.DesktopBranding) *domain.DesktopBranding {
	out := *b
	out.UpdaterEndpoint = endpoint
	return &out
}

func TestDesktopManifestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), desktopManifestName)

	if m, err := readDesktopManifestFile(path); err != nil {
		t.Fatalf("read missing manifest: %v", err)
	} else if m != nil {
		t.Fatalf("a missing manifest must read nil, got %+v", m)
	}

	want := domain.DesktopReleaseManifest{
		Version: "26083003",
		Notes:   "first desktop release",
		PubDate: "2026-10-23T00:00:00Z",
		URL:     "https://example.test/desktop/26083003/AuthyIDE.AppImage",
	}
	if err := writeDesktopManifestAtomic(path, want); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	got, err := readDesktopManifestFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if *got != want {
		t.Fatalf("manifest = %+v, want %+v", got, want)
	}

	if err := os.WriteFile(path, []byte(`{"notes":"incomplete"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write incomplete manifest: %v", err)
	}
	if m, err := readDesktopManifestFile(path); err != nil {
		t.Fatalf("read incomplete manifest: %v", err)
	} else if m != nil {
		t.Fatalf("an incomplete manifest must read nil, got %+v", m)
	}

	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write broken manifest: %v", err)
	}
	if _, err := readDesktopManifestFile(path); err == nil {
		t.Fatal("a broken manifest must not read cleanly")
	}
}

func TestVersionGreater(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"26083004", "26083003", true},
		{"26083003", "26083003", false},
		{"99", "100", false},
		{"26083004", "99", true},
		{"260830040", "26083003", true},
	}
	for _, tc := range cases {
		if got := versionGreater(tc.candidate, tc.current); got != tc.want {
			t.Fatalf("versionGreater(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}
