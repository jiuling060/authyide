package domain

import "testing"

func TestPlatformValid(t *testing.T) {
	for _, platform := range BuildPlatforms {
		if !PlatformValid(platform) {
			t.Fatalf("%s must be buildable", platform)
		}
	}
	for _, platform := range []string{"", "ios", "android-linux"} {
		if PlatformValid(platform) {
			t.Fatalf("%q must not be buildable", platform)
		}
	}
}

func TestChannelValid(t *testing.T) {
	for _, channel := range ArtifactChannels {
		if !ChannelValid(channel) {
			t.Fatalf("%s must be a known channel", channel)
		}
	}
	for _, channel := range []string{"", "APPIMAGE", "msi", "apk.tar.gz"} {
		if ChannelValid(channel) {
			t.Fatalf("%q must not be a known channel", channel)
		}
	}
}

func TestChannelPublishableIsAppImageOnly(t *testing.T) {
	for _, channel := range ArtifactChannels {
		want := channel == ChannelAppImage
		if got := ChannelPublishable(channel); got != want {
			t.Fatalf("ChannelPublishable(%s) = %v, want %v", channel, got, want)
		}
	}
}

func TestPrimaryChannelOf(t *testing.T) {
	cases := map[string]string{
		PlatformAndroid:      ChannelAPK,
		PlatformDesktopLinux: ChannelAppImage,
		"":                   "",
		"ios":                "",
	}
	for platform, want := range cases {
		if got := PrimaryChannelOf(platform); got != want {
			t.Fatalf("PrimaryChannelOf(%q) = %q, want %q", platform, got, want)
		}
	}
}

// TestReleaseTrainPaths pins the on-disk layout of both release trains. The
// admin-console train keeps the instance-wide paths and a self-service app adds
// its own app id as one directory, so the two never overwrite each other.
func TestReleaseTrainPaths(t *testing.T) {
	const appID = "550e8400-e29b-41d4-a716-446655440000"

	if got := MobileManifestRelPath("", PlatformAndroid); got != "/app-version/android.json" {
		t.Fatalf("instance mobile manifest = %q", got)
	}
	if got := MobileManifestRelPath(appID, PlatformAndroid); got != "/app-version/"+appID+"/android.json" {
		t.Fatalf("app mobile manifest = %q", got)
	}
	if got := MobileArtifactRelPath("", "1.2.3"); got != "/mobile/1.2.3/"+MobileArtifactName {
		t.Fatalf("instance apk = %q", got)
	}
	if got := MobileArtifactRelPath(appID, "1.2.3"); got != "/mobile/"+appID+"/1.2.3/"+MobileArtifactName {
		t.Fatalf("app apk = %q", got)
	}
	if got := DesktopManifestRelPath(""); got != "/desktop/"+StaticDesktopManifest {
		t.Fatalf("instance desktop manifest = %q", got)
	}
	if got := DesktopManifestRelPath(appID); got != "/desktop/"+appID+"/"+StaticDesktopManifest {
		t.Fatalf("app desktop manifest = %q", got)
	}
	if got := DesktopArtifactRelPath("", "26083003", "AuthyIDE.AppImage"); got != "/desktop/26083003/AuthyIDE.AppImage" {
		t.Fatalf("instance desktop artifact = %q", got)
	}
	if got := DesktopArtifactRelPath(appID, "26083003", "AuthyIDE.AppImage"); got != "/desktop/"+appID+"/26083003/AuthyIDE.AppImage" {
		t.Fatalf("app desktop artifact = %q", got)
	}

	// The updater endpoint and the on-disk manifest describe the same train:
	// the client polls /latest.json, the worker writes latest-linux.json.
	want := "https://cdn.example.test/static/desktop/" + appID + "/" + DesktopEndpointLeaf
	if got := DesktopUpdaterEndpoint("https://cdn.example.test/", "/static/", appID); got != want {
		t.Fatalf("updater endpoint = %q, want %q", got, want)
	}
	if got := DesktopUpdaterEndpoint("https://cdn.example.test", "/static", ""); got != "https://cdn.example.test/static/desktop/"+DesktopEndpointLeaf {
		t.Fatalf("instance updater endpoint = %q", got)
	}
}

func TestJoinStaticTrimsSlashes(t *testing.T) {
	if got := JoinStatic("https://a.b/", "/static/", "/desktop/", "app", "/latest.json"); got != "https://a.b/static/desktop/app/latest.json" {
		t.Fatalf("JoinStatic = %q", got)
	}
	if got := JoinStatic("", "", "x", "y"); got != "/x/y" {
		t.Fatalf("JoinStatic relative = %q", got)
	}
}

func TestDefaultDesktopBranding(t *testing.T) {
	app := &AppBranding{Name: "FieldNotes", PackageID: "com.example.fieldnotes"}
	b := DefaultDesktopBranding(app)
	if b.ProductName != "FieldNotes" {
		t.Fatalf("product name = %q", b.ProductName)
	}
	if b.BundleIdentifier != "com.example.fieldnotes"+DesktopAppSuffix {
		t.Fatalf("bundle id = %q", b.BundleIdentifier)
	}
}
