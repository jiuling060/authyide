package v1

import "testing"

func TestArtifactContentType(t *testing.T) {
	cases := map[string]string{
		"authyide.apk":       "application/vnd.android.package-archive",
		"AuthyIDE.deb":       "application/vnd.debian.binary-package",
		"AuthyIDE.rpm":       "application/x-rpm",
		"AuthyIDE.AppImage":  "application/octet-stream",
		"AuthyIDE_APP_IMAGE": "application/octet-stream",
		"":                   "application/octet-stream",
	}
	for name, want := range cases {
		if got := artifactContentType(name); got != want {
			t.Fatalf("artifactContentType(%q) = %q, want %q", name, got, want)
		}
	}
}
