package runner

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// solid makes an opaque single-colour source image.
func solidRGBA(side int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// wantIconPixel is the opaque source colour the icon tests sample back.
var wantIconPixel = color.NRGBA{R: 10, G: 120, B: 240, A: 255}

func TestResizeSolidKeepsColour(t *testing.T) {
	wantSolid := color.NRGBA{R: 200, G: 100, B: 40, A: 255}
	src := solidRGBA(9, wantSolid)
	for _, side := range []int{32, 128, 512} {
		got := resizeRGBA(src, side)
		b := got.Bounds()
		if b.Dx() != side || b.Dy() != side {
			t.Fatalf("resizeRGBA(%d) bounds = %v", side, b)
		}
		if c := got.NRGBAAt(0, 0); c != wantSolid {
			t.Fatalf("corner pixel of a solid source changed: %+v", c)
		}
		if c := got.NRGBAAt(side/2, side/2); c != wantSolid {
			t.Fatalf("centre pixel of a solid source changed: %+v", c)
		}
	}
}

// ramp checks that bilinear interpolation lands between the two neighbouring
// source pixels instead of snapping to one of them, which is the failure mode
// of nearest-neighbour scaling.
func TestResizeInterpolates(t *testing.T) {
	// A 2x2 gradient: the left column is opaque red, the right column is
	// fully transparent. Blending across it must fade the red channel.
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		src.SetNRGBA(0, y, color.NRGBA{255, 0, 0, 255})
		src.SetNRGBA(1, y, color.NRGBA{255, 0, 0, 0})
	}
	got := resizeRGBA(src, 8)

	edge := got.NRGBAAt(0, 0)
	if edge.A != 255 {
		t.Fatalf("left edge alpha = %d, want 255", edge.A)
	}
	inner := got.NRGBAAt(3, 0)
	if inner.A >= 255 {
		t.Fatalf("interior alpha = %d, the blend must fade below opaque", inner.A)
	}
	// Blending in premultiplied space keeps the visible colour red wherever
	// alpha is still above zero.
	if inner.R != 255 || inner.G != 0 || inner.B != 0 {
		t.Fatalf("interior colour = %+v, the blend must stay red", inner)
	}
}

// TestResizeCropsNonSquare guards against a stretched label: a wide source must
// be centred-cropped to the smaller dimension before scaling.
func TestResizeCropsNonSquare(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			if x < 2 {
				src.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
			} else if x > 5 {
				src.SetNRGBA(x, y, color.NRGBA{0, 0, 255, 255})
			} else {
				src.SetNRGBA(x, y, color.NRGBA{0, 255, 0, 255})
			}
		}
	}
	got := resizeRGBA(src, 16)
	c := got.NRGBAAt(8, 8)
	if c.G != 255 || c.R != 0 || c.B != 0 {
		t.Fatalf("centre of a cropped source = %+v, want the green middle band", c)
	}
}

func TestMaterializeIcons(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "icon.png")
	if err := writePNG(src, solidRGBA(200, wantIconPixel), 0o644); err != nil {
		t.Fatalf("write source icon: %v", err)
	}

	r := NewDesktopRunner(DesktopConfig{IconPath: src}, nil, slog.Default())
	jobID := uuid.New()
	files, err := r.materializeIcons(dir, jobID)
	if err != nil {
		t.Fatalf("materializeIcons: %v", err)
	}
	if len(files) != 5 {
		t.Fatalf("icon set has %d files, want 5: %v", len(files), files)
	}

	want := map[string]int{
		desktopIcon32:      32,
		desktopIcon128:     128,
		desktopIcon256:     256,
		desktopIconLarge:   512,
		desktopIconWindows: 256,
	}
	seen := map[string]bool{}
	for _, p := range files {
		name := filepath.Base(p)
		if _, ok := want[name]; !ok {
			t.Fatalf("unexpected icon file %q", name)
		}
		seen[name] = true
		if !filepath.IsAbs(p) {
			t.Fatalf("icon path %q must be absolute for tauri.conf.json", p)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if len(raw) == 0 {
			t.Fatalf("%s is empty", p)
		}
		if name == desktopIconWindows {
			// Go's image package has no ico decoder, so only the directory
			// header is checked here; TestWriteICO covers the entry in depth.
			if len(raw) < 22 || binary.LittleEndian.Uint16(raw[0:2]) != 0 ||
				binary.LittleEndian.Uint16(raw[2:4]) != 1 || binary.LittleEndian.Uint16(raw[4:6]) != 1 {
				t.Fatalf("%s is not a one-image ico: %v", p, raw[:6])
			}
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", p, err)
		}
		b := img.Bounds()
		if b.Dx() != want[name] || b.Dy() != want[name] {
			t.Fatalf("%s is %dx%d, want %dx%d", name, b.Dx(), b.Dy(), want[name], want[name])
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("icon set is missing %s", name)
		}
	}

	// The icon set is job scoped, so a second job does not inherit the first
	// one's assets and the first one's directory is removed.
	other := uuid.New()
	if _, err := r.materializeIcons(dir, other); err != nil {
		t.Fatalf("materializeIcons second job: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, iconDirName(jobID))); !os.IsNotExist(err) {
		t.Fatalf("the first job's icon dir must be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, iconDirName(other))); err != nil {
		t.Fatalf("the current job's icon dir must survive: %v", err)
	}
}

func TestMaterializeIconsRejectsMissingSource(t *testing.T) {
	r := NewDesktopRunner(DesktopConfig{IconPath: filepath.Join(t.TempDir(), "absent.png")}, nil, slog.Default())
	if _, err := r.materializeIcons(t.TempDir(), uuid.New()); err == nil {
		t.Fatal("a missing icon source must fail the build")
	}
}

func TestWriteICO(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "icon.ico")
	img := solidRGBA(256, wantIconPixel)
	if err := writeICO(path, img, 0o644); err != nil {
		t.Fatalf("writeICO: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ico: %v", err)
	}

	// ICONDIR: reserved(2), type(2), image count(2).
	if binary.LittleEndian.Uint16(raw[0:2]) != 0 ||
		binary.LittleEndian.Uint16(raw[2:4]) != 1 ||
		binary.LittleEndian.Uint16(raw[4:6]) != 1 {
		t.Fatalf("icon dir header = %v", raw[:6])
	}
	// ICONDIRENTRY: width(1), height(1), palette(1), reserved(1), planes(2),
	// bit count(2), bytes in res(4), image offset(4).
	e := raw[6:]
	if e[0] != 0 || e[1] != 0 || e[2] != 0 || e[3] != 0 {
		t.Fatalf("256 px must be encoded as 0 0: %v", e[:4])
	}
	if binary.LittleEndian.Uint16(e[4:6]) != 1 || binary.LittleEndian.Uint16(e[6:8]) != 32 {
		t.Fatalf("planes/bit count = %v", e[4:8])
	}
	size := binary.LittleEndian.Uint32(e[8:12])
	off := binary.LittleEndian.Uint32(e[12:16])
	if int(off) != 22 {
		t.Fatalf("image offset = %d, want 22", off)
	}
	if int(size) != len(raw)-22 {
		t.Fatalf("entry size = %d, want %d", size, len(raw)-22)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reopen ico: %v", err)
	}
	defer f.Close()
	if _, err := f.Seek(22, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	decoded, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("the ico payload must decode as a png: %v", err)
	}
	b := decoded.Bounds()
	if b.Dx() != 256 || b.Dy() != 256 {
		t.Fatalf("ico payload is %dx%d", b.Dx(), b.Dy())
	}
	if !pixelEquals(decoded.At(0, 0), wantIconPixel) {
		t.Fatalf("ico pixel = %+v", decoded.At(0, 0))
	}
}

// pixelEquals compares the source colour with what the png decoder hands back,
// which is a premultiplied RGBA when the image is fully opaque.
func pixelEquals(got, want color.Color) bool {
	gr, gg, gb, ga := got.RGBA()
	wr, wg, wb, wa := want.RGBA()
	return gr == wr && gg == wg && gb == wb && ga == wa
}

func TestDesktopOverlayIconFiles(t *testing.T) {
	overlay := desktopTauriOverlay(desktopOverlaySpec{IconFiles: []string{"/a/32x32.png", "/a/icon.ico"}})
	bundle, ok := overlay["bundle"].(map[string]any)
	if !ok {
		t.Fatal("overlay has no bundle section")
	}
	icons, found := bundle["icon"]
	if !found {
		t.Fatal("an icon set must override the shell's checked-in icons")
	}
	list, isList := icons.([]string)
	if !isList || len(list) != 2 {
		t.Fatalf("icon set = %v", icons)
	}
	if len(overlay) == 0 {
		t.Fatal("empty overlay")
	}

	empty := desktopTauriOverlay(desktopOverlaySpec{})
	eb, ok := empty["bundle"].(map[string]any)
	if !ok {
		t.Fatal("overlay has no bundle section")
	}
	if _, present := eb["icon"]; present {
		t.Fatal("without an app icon the overlay must keep the shell's icons")
	}
}
