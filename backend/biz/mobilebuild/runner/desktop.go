// Package runner: the desktop (Tauri) build pipeline.
package runner

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

// Defaults for the branches and locations the pipeline checks out.
const (
	defaultShellBranch     = "release"
	defaultEngineBranch    = "main"
	defaultExtensionBranch = "v20.8.0"

	// desktopEngineName is the sidecar binary name Tauri looks for under bin/.
	desktopEngineName  = "ohmyagent"
	desktopSidecarDir  = "binaries"
	desktopExtStaging  = "binaries/browser-extension"
	desktopRuntimeDir  = "binaries/runtime"
	desktopBundleName  = "tauri.build.conf.json"
	desktopUpdaterBase = "https://tauri.app/plugins/v2/api/updater"

	// desktopGitTime bounds the git and npm steps. They are interactive with
	// the network, so give them more room than a local command and less than
	// a full cargo build.
	desktopGitTime = 10 * time.Minute
)

// DesktopConfig is the desktop pipeline's configuration.
type DesktopConfig struct {
	SharedConfig

	// DesktopWorkspace is the Tauri shell checkout. The pipeline edits
	// bundle.linux.conf.json inside it, so it is never shared with another job.
	DesktopWorkspace string
	// UINextDir is the ui-next repository that becomes the shell's frontend.
	UINextDir string
	// EngineSrc is the ohmyagent source tree; a missing Makefile fails closed.
	EngineSrc string
	// ExtensionDir is the browser-extension repository.
	ExtensionDir string
	// SkillsDir is copied next to the engine when non-empty.
	SkillsDir string

	UINextBranch    string
	ExtensionBranch string
	EngineBranch    string
	ShellBranch     string

	// Channels selects the artifact targets to collect. Empty collects
	// appimage only, which is the only target that supports self-update.
	Channels []string
	// ProductName and BundleID override the shell's own product name and
	// identifier. Empty keeps the checked-in values.
	ProductName string
	BundleID    string
	Version     string
	// UpdaterEndpoint is written into the tauri updater plugin config. Empty
	// leaves the shell's own endpoint untouched and yields an unsigned
	// manifest, which the client treats as no-update-available.
	UpdaterEndpoint   string
	UpdaterSigningKey string
	// IconPath points at one source PNG. When set, the runner resamples it
	// into the icon set Tauri bundles for the installed targets and overrides
	// the shell's checked-in icons. Empty keeps the checked-in icons.
	IconPath string
	// SourceSnapshot is the session source archive to overlay onto the shell
	// checkout. Nil builds the checked-in shell source.
	SourceSnapshot *domain.SourceSnapshot
}

// desktopRunner runs one desktop pipeline. A runner is created per job.
type desktopRunner struct {
	baseRunner
	cfg     DesktopConfig
	objects domain.ObjectSource
	jobID   uuid.UUID
	// overlayPaths are the session files this job wrote into the shared shell
	// checkout; Run rolls them back before the next job checks out.
	overlayPaths []string
}

// NewDesktopRunner creates a desktop pipeline runner. objects may be nil when
// no session snapshot is overlaid.
func NewDesktopRunner(cfg DesktopConfig, objects domain.ObjectSource, logger *slog.Logger) *desktopRunner {
	if cfg.TailBytes <= 0 {
		cfg.TailBytes = DefaultTailBytes
	}
	if cfg.LogTail <= 0 {
		cfg.LogTail = DefaultLogTail
	}
	if cfg.BuildTimeout <= 0 {
		cfg.BuildTimeout = DefaultBuildTime
	}
	if cfg.CargoTimeout <= 0 {
		cfg.CargoTimeout = DefaultCargoTime
	}
	return &desktopRunner{
		baseRunner: baseRunnerOf(cfg.SharedConfig, logger),
		cfg:        cfg,
		objects:    objects,
	}
}

func (r *desktopRunner) Platform() string {
	return domain.PlatformDesktopLinux
}

// Precheck reports host prerequisites the desktop pipeline cannot run without.
// Called once per enabled platform by the status endpoint, so every probe is
// capped at two seconds.
func (r *desktopRunner) Precheck(ctx context.Context) []string {
	missing := make([]string, 0, 12)
	for _, name := range []string{"cargo", "rustc", "go", "patchelf", "rpm", "node", "npm"} {
		if _, err := exec.LookPath(name); err != nil {
			missing = append(missing, name)
		}
	}
	// The webkit2gtk/gtk3 headers are only visible through pkg-config, which
	// tauri-bundler shells out to during bundle.
	for _, pkg := range []string{"webkit2gtk-4.1", "gtk+-3.0"} {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := exec.CommandContext(cctx, "pkg-config", "--exists", pkg).Run()
		cancel()
		if err != nil {
			missing = append(missing, pkg)
		}
	}
	if r.cfg.DesktopWorkspace != "" {
		if _, err := os.Stat(filepath.Join(r.cfg.DesktopWorkspace, "Cargo.toml")); err != nil {
			missing = append(missing, "desktop-shell")
		}
	}
	return missing
}

// channels returns the artifact targets this job collects, dropping anything
// that is not a desktop channel.
func (r *desktopRunner) channels() []string {
	want := r.cfg.Channels
	if len(want) == 0 {
		want = domain.DesktopChannels
	}
	out := make([]string, 0, len(want))
	seen := map[string]bool{}
	for _, c := range want {
		if domain.ChannelValid(c) && !seen[c] {
			out = append(out, c)
			seen[c] = true
		}
	}
	if len(out) == 0 {
		out = []string{domain.ChannelAppImage}
	}
	return out
}

// env is the environment every desktop step runs with. CARGO_HOME and
// RUSTUP_HOME are inherited when set so a runner can point at a pre-warmed
// toolchain; without them rust picks up its own defaults.
func (r *desktopRunner) env() []string {
	pairs := make([][2]string, 0, 8)
	for _, key := range []string{"PATH", "CARGO_HOME", "RUSTUP_HOME"} {
		if v := os.Getenv(key); v != "" {
			pairs = append(pairs, [2]string{key, v})
		}
	}
	pairs = append(pairs,
		[2]string{"HOME", "/root"},
		[2]string{"CI", "1"},
		[2]string{"LANG", "C.UTF-8"},
		[2]string{"NO_COLOR", "1"},
		[2]string{"DEBIAN_FRONTEND", "noninteractive"},
		[2]string{"NPM_CONFIG_UPDATE_NOTIFIER", "false"},
	)
	return envKV(pairs...)
}

// Run executes every desktop pipeline step.
func (r *desktopRunner) Run(ctx context.Context, jobID uuid.UUID, onStep StepFunc, onTail TailFunc) (*RunResult, error) {
	started := time.Now()
	r.jobID = jobID
	if err := os.MkdirAll(r.cfg.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(r.cfg.LogDir, jobID.String()+".log")
	logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open build log: %w", err)
	}
	defer logF.Close()
	if _, err = fmt.Fprintf(logF, "=== desktop build %s started %s ===\n", jobID.String(), started.UTC().Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("write build log: %w", err)
	}

	cancelTail := r.startTailLoop(ctx, logPath, onTail)
	defer cancelTail()
	r.cleanupOlder(DefaultLogRetention, logPath)

	fail := func(step string, msg string) (*RunResult, error) {
		return nil, fmt.Errorf("%s: %s", step, msg)
	}
	// The shell checkout is shared between jobs, so the session files one job
	// overlays have to go again before the next job checks out.
	defer rollbackOverlay(r.overlayPaths)

	for _, s := range r.steps() {
		if err := r.step(ctx, s.name, onStep, logF, s.fn); err != nil {
			return fail(s.name, err.Error())
		}
	}

	found, err := r.collectArtifacts()
	if err != nil {
		return nil, err
	}
	return &RunResult{Artifacts: found, LogPath: logPath}, nil
}

// desktopStep is one stage of the desktop pipeline.
type desktopStep struct {
	name string
	fn   func(context.Context, io.Writer) error
}

// steps lists the pipeline stages in execution order. overlay-source runs only
// for jobs bound to a session snapshot, after the shell checkout so the reset
// cannot wipe it and before bundle so the overlaid source is what gets built.
func (r *desktopRunner) steps() []desktopStep {
	steps := []desktopStep{
		{domain.StepUINext, r.stepUINext},
		{domain.StepEngine, r.stepEngine},
		{domain.StepBrowserExtension, r.stepExtension},
		{domain.StepShell, r.stepShell},
	}
	if r.cfg.SourceSnapshot != nil {
		steps = append(steps, desktopStep{domain.StepOverlaySource, r.overlaySource})
	}
	return append(steps,
		desktopStep{domain.StepBundle, r.stepBundle},
		desktopStep{domain.StepVerify, r.verify},
	)
}

// overlaySource overlays the bound session snapshot onto the shell checkout.
// Entries under the sidecar directory are dropped: that tree is staged by this
// runner, so a session file there would break the bundle instead of the build.
func (r *desktopRunner) overlaySource(ctx context.Context, log io.Writer) error {
	overlay := newSourceOverlay(r.objects, r.cfg.DesktopWorkspace, r.cfg.SourceSnapshot,
		r.cfg.LogDir, desktopSidecarDir)
	paths, err := overlay.Apply(ctx, log)
	r.overlayPaths = paths
	return err
}

// stepUINext builds the ui-next frontend. node_modules is left in place, so a
// second job on the same runner skips the install.
func (r *desktopRunner) stepUINext(ctx context.Context, log io.Writer) error {
	d := r.cfg.UINextDir
	if d == "" {
		return fmt.Errorf("ui-next directory not configured")
	}
	if err := gitCheckout(ctx, d, defaultIfEmpty(r.cfg.UINextBranch, defaultShellBranch), desktopGitTime, r.env(), log); err != nil {
		return fmt.Errorf("ui-next checkout: %w", err)
	}
	if err := runStep(ctx, d, r.env(), "npm", []string{"ci"}, r.cfg.BuildTimeout, log); err != nil {
		return fmt.Errorf("npm ci: %w", err)
	}
	if err := runStep(ctx, d, r.env(), "npm", []string{"run", "build"}, r.cfg.BuildTimeout, log); err != nil {
		return fmt.Errorf("npm run build: %w", err)
	}
	return nil
}

// stepEngine builds the sidecar. The engine repo is compiled, not vendored: a
// version drift between the engine commit and the shell would be shipped to
// users, so the branch is pinned by the request.
func (r *desktopRunner) stepEngine(ctx context.Context, log io.Writer) error {
	d := r.cfg.EngineSrc
	if d == "" {
		return fmt.Errorf("engine source not configured")
	}
	if _, err := os.Stat(filepath.Join(d, "Makefile")); err != nil {
		return fmt.Errorf("engine %s: missing Makefile, %w", d, err)
	}
	if err := gitCheckout(ctx, d, defaultIfEmpty(r.cfg.EngineBranch, defaultEngineBranch), desktopGitTime, r.env(), log); err != nil {
		return fmt.Errorf("engine checkout: %w", err)
	}
	// The first build on a clean runner compiles the whole workspace in tens of
	// minutes, so this step gets its own budget.
	if err := runStep(ctx, d, r.env(), "make", []string{"linux"}, r.cfg.CargoTimeout, log); err != nil {
		return fmt.Errorf("make linux: %w", err)
	}
	if _, err := findEngineBinary(d); err != nil {
		return fmt.Errorf("engine build produced no binary: %w", err)
	}
	return nil
}

// stepExtension stages the browser extension zip next to the sidecar so
// tauri-bundler picks it up as a bundled resource.
func (r *desktopRunner) stepExtension(ctx context.Context, log io.Writer) error {
	d := r.cfg.ExtensionDir
	if d == "" {
		return fmt.Errorf("browser extension source not configured")
	}
	if err := gitCheckout(ctx, d, defaultIfEmpty(r.cfg.ExtensionBranch, defaultExtensionBranch), desktopGitTime, r.env(), log); err != nil {
		return fmt.Errorf("extension checkout: %w", err)
	}
	zipPath, err := findExtensionZip(d)
	if err != nil {
		return fmt.Errorf("extension artifact: %w", err)
	}
	staging := filepath.Join(r.cfg.DesktopWorkspace, desktopExtStaging)
	if err = os.MkdirAll(staging, 0o755); err != nil {
		return fmt.Errorf("mkdir extension staging: %w", err)
	}
	if err = copyFile(zipPath, filepath.Join(staging, filepath.Base(zipPath))); err != nil {
		return fmt.Errorf("stage extension zip: %w", err)
	}
	// The skills tree ships next to the engine; the shell looks for it there.
	if r.cfg.SkillsDir != "" {
		if err := copyDir(r.cfg.SkillsDir, filepath.Join(r.cfg.DesktopWorkspace, desktopRuntimeDir, "skills")); err != nil {
			return fmt.Errorf("stage skills: %w", err)
		}
	}
	if _, err = fmt.Fprintf(log, "[extension] staged %s\n", filepath.Base(zipPath)); err != nil {
		return err
	}
	return nil
}

// stepShell compiles the Tauri shell. cargo check runs before the real bundle
// so a broken shell fails in seconds instead of after a full link.
func (r *desktopRunner) stepShell(ctx context.Context, log io.Writer) error {
	d := r.cfg.DesktopWorkspace
	if _, err := os.Stat(filepath.Join(d, "Cargo.toml")); err != nil {
		return fmt.Errorf("desktop shell %s: missing Cargo.toml", d)
	}
	if err := gitCheckout(ctx, d, defaultIfEmpty(r.cfg.ShellBranch, defaultShellBranch), desktopGitTime, r.env(), log); err != nil {
		return fmt.Errorf("shell checkout: %w", err)
	}
	if err := runStep(ctx, d, r.env(), "cargo", []string{"check", "--locked"}, r.cfg.CargoTimeout, log); err != nil {
		return fmt.Errorf("cargo check: %w", err)
	}
	return nil
}

// stepBundle runs tauri build with the config fragment this job needs. The
// checked-in tauri.conf.json keeps bundle.active false so that cargo check
// cannot accidentally try to bundle a development checkout.
func (r *desktopRunner) stepBundle(ctx context.Context, log io.Writer) error {
	d := r.cfg.DesktopWorkspace
	channels := r.channels()
	spec := desktopOverlaySpec{
		ProductName:     r.cfg.ProductName,
		BundleID:        r.cfg.BundleID,
		Version:         r.cfg.Version,
		Channels:        channels,
		UpdaterEndpoint: r.cfg.UpdaterEndpoint,
	}
	if r.cfg.IconPath != "" {
		set, err := r.materializeIcons(d, r.jobID)
		if err != nil {
			return err
		}
		spec.IconFiles = set
	}
	overlay := desktopTauriOverlay(spec)
	data, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tauri overlay: %w", err)
	}
	data = append(data, '\n')
	if err = atomicWriteFile(filepath.Join(d, desktopBundleName), data, 0o644); err != nil {
		return fmt.Errorf("write tauri overlay: %w", err)
	}
	if _, err = fmt.Fprintf(log, "[bundle] targets=%v productName=%q identifier=%q version=%q\n",
		channels, r.cfg.ProductName, r.cfg.BundleID, r.cfg.Version); err != nil {
		return err
	}
	if err = runStep(ctx, d, r.env(), "cargo",
		[]string{"build", "--no-default-features", "--config", desktopBundleName},
		r.cfg.CargoTimeout, log); err != nil {
		return fmt.Errorf("tauri build: %w", err)
	}
	return nil
}

// desktopIconSet is the icon set Tauri bundles for one job. The png names match
// the layout the `tauri icon` CLI emits, so a build and a hand-authored icon
// set are interchangeable.
const (
	desktopIcon32      = "32x32.png"
	desktopIcon128     = "128x128.png"
	desktopIcon256     = "128x128@2x.png"
	desktopIconLarge   = "icon.png"
	desktopIconWindows = "icon.ico"
)

// materializeIcons resamples one source PNG into the icon set the bundle step
// points at and returns the absolute file paths. The set lands in a job-scoped
// directory, because the shell checkout is rebuilt in place and a shared icons/
// tree would race between concurrent desktop jobs.
func (r *desktopRunner) materializeIcons(workspace string, jobID uuid.UUID) ([]string, error) {
	base, err := loadRGBA(r.cfg.IconPath)
	if err != nil {
		return nil, fmt.Errorf("read app icon %s: %w", r.cfg.IconPath, err)
	}
	dir := filepath.Join(workspace, iconDirName(jobID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create icon dir: %w", err)
	}
	r.removeStaleIconDirs(workspace, dir)
	sizes := []struct {
		name string
		side int
	}{
		{desktopIcon32, 32},
		{desktopIcon128, 128},
		{desktopIcon256, 256},
		{desktopIconLarge, 512},
	}
	files := make([]string, 0, len(sizes)+1)
	for _, s := range sizes {
		p := filepath.Join(dir, s.name)
		if err = writePNG(p, resizeRGBA(base, s.side), 0o644); err != nil {
			return nil, fmt.Errorf("write icon %s: %w", s.name, err)
		}
		files = append(files, p)
	}
	ico := filepath.Join(dir, desktopIconWindows)
	if err = writeICO(ico, resizeRGBA(base, 256), 0o644); err != nil {
		return nil, fmt.Errorf("write icon %s: %w", desktopIconWindows, err)
	}
	files = append(files, ico)
	return files, nil
}

// iconDirName is the per-job icon directory under the shell checkout.
func iconDirName(jobID uuid.UUID) string {
	if jobID == uuid.Nil {
		return ".icons"
	}
	return ".icons-" + jobID.String()
}

// removeStaleIconDirs drops the icon sets of other jobs. Only the current job's
// directory survives, which keeps the shared checkout from accumulating one per
// build.
func (r *desktopRunner) removeStaleIconDirs(workspace, keep string) {
	keepBase := filepath.Base(keep)
	entries, err := os.ReadDir(workspace)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), ".icons") || e.Name() == keepBase {
			continue
		}
		if err := os.RemoveAll(filepath.Join(workspace, e.Name())); err != nil {
			r.logger.Warn("remove stale desktop icon dir", "dir", e.Name(), "error", err)
		}
	}
}

// loadRGBA decodes one icon file into an image whose alpha channel survives the
// resample.
func loadRGBA(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	dst := image.NewNRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, src.Bounds().Min, draw.Src)
	return dst, nil
}

// resizeRGBA scales src to side x side with bilinear interpolation and returns
// a NRGBA image. Non-square sources are cropped to the smaller dimension first,
// so a label never stretches.
func resizeRGBA(src image.Image, side int) *image.NRGBA {
	if side <= 0 {
		return nil
	}
	b := src.Bounds()
	if b.Dx() != b.Dy() {
		c := min(b.Dx(), b.Dy())
		b = image.Rect(b.Min.X+(b.Dx()-c)/2, b.Min.Y+(b.Dy()-c)/2, b.Min.X+(b.Dx()+c)/2, b.Min.Y+(b.Dy()+c)/2)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	if b.Dx() == side && b.Dy() == side {
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}
	scaleBilinear(dst, src, b)
	return dst
}

// scaleBilinear maps sr onto dst's bounds with bilinear interpolation, blending
// in premultiplied space so a transparent edge does not smear a color halo. The
// standard library ships nearest-neighbour scaling only, so this is a short
// local implementation rather than a new dependency.
func scaleBilinear(dst *image.NRGBA, src image.Image, sr image.Rectangle) {
	dr := dst.Bounds()
	sx, sy := float64(sr.Dx())/float64(dr.Dx()), float64(sr.Dy())/float64(dr.Dy())
	for dy := dr.Min.Y; dy < dr.Max.Y; dy++ {
		y0, ty := sampleAxis((float64(dy-dr.Min.Y)+0.5)*sy-0.5, float64(sr.Min.Y), float64(sr.Max.Y))
		y1 := y0 + 1
		for dx := dr.Min.X; dx < dr.Max.X; dx++ {
			x0, tx := sampleAxis((float64(dx-dr.Min.X)+0.5)*sx-0.5, float64(sr.Min.X), float64(sr.Max.X))
			x1 := x0 + 1
			top := mixColor(src.At(sr.Min.X+x0, sr.Min.Y+y0), src.At(sr.Min.X+x1, sr.Min.Y+y0), tx)
			bot := mixColor(src.At(sr.Min.X+x0, sr.Min.Y+y1), src.At(sr.Min.X+x1, sr.Min.Y+y1), tx)
			dst.SetNRGBA(dx, dy, mixColor(top, bot, ty))
		}
	}
}

// sampleAxis maps one destination row or column to the two source samples it
// blends, clamping at the source edge. lo and hi are source coordinates in
// pixels.
func sampleAxis(f, lo, hi float64) (int, float64) {
	i := int(math.Floor(f))
	if float64(i) < lo {
		return int(lo), 0
	}
	if float64(i)+1 >= hi {
		return int(hi - 1), 0
	}
	return i, f - float64(i)
}

// mixColor interpolates two pixels in premultiplied alpha space, so a blend
// along a transparent edge does not smear a color halo.
func mixColor(a, b color.Color, t float64) color.NRGBA {
	ar, ag, ab, aa := premultiplied(a)
	br, bg, bb, ba := premultiplied(b)
	ma := aa + (ba-aa)*t
	if ma <= 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{
		R: clipByte(ar+(br-ar)*t, ma),
		G: clipByte(ag+(bg-ag)*t, ma),
		B: clipByte(ab+(bb-ab)*t, ma),
		A: clipByte(ma, 1),
	}
}

// premultiplied turns a straight-alpha pixel into the R, G, B and A channel
// fractions used by mixColor.
func premultiplied(c color.Color) (float64, float64, float64, float64) {
	r, g, b, a := c.RGBA()
	// RGBA reports the channels premultiplied by alpha and scaled by 257, so a
	// plain / 65535 already gives the premultiplied fractions. Multiplying by
	// the alpha fraction a second time dims every blended pixel, which turns a
	// red edge into a washed-out pink one.
	af := float64(a) / 65535.0
	if af <= 0 {
		return 0, 0, 0, 0
	}
	return float64(r) / 65535.0, float64(g) / 65535.0, float64(b) / 65535.0, af
}

// clipByte maps a premultiplied channel over a 0-1 alpha back to an 8-bit byte.
func clipByte(ch, alpha float64) uint8 {
	f := ch / alpha
	if f <= 0 {
		return 0
	}
	if f > 1 {
		return 255
	}
	return uint8(f*255 + 0.5)
}

// writePNG writes one icon at file size side.
func writePNG(path string, img *image.NRGBA, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writeICO writes one PNG-encoded ICO entry. PNG compression inside an ICO is
// understood by every target this pipeline ships to.
func writeICO(path string, img *image.NRGBA, mode os.FileMode) error {
	buf := &bytes.Buffer{}
	if err := png.Encode(buf, img); err != nil {
		return err
	}
	encoded := buf.Bytes()
	if len(encoded) > 0xFFFF {
		return fmt.Errorf("encoded png is too large for an ico entry: %d bytes", len(encoded))
	}
	w := img.Bounds().Dx()
	if w > 256 {
		w = 256
	}
	// The spec encodes a 256 px image as 0 in the entry width and height bytes.
	wid := w
	if wid == 256 {
		wid = 0
	}

	// ICONDIR: reserved(2), type(2), image count(2). ICONDIRENTRY: width(1),
	// height(1), palette(1), reserved(1), planes(2), bit count(2), bytes in
	// image(4), data offset(4).
	var order binary.ByteOrder = binary.LittleEndian
	hdr := &bytes.Buffer{}
	_, _ = hdr.Write([]byte{0, 0, 1, 0, 1, 0})
	_, _ = hdr.Write([]byte{byte(wid % 256), byte(wid / 256), 0, 0})
	_, _ = hdr.Write([]byte{1, 0, 32, 0})
	var u32 [4]byte
	order.PutUint32(u32[:], uint32(len(encoded)))
	_, _ = hdr.Write(u32[:])
	// The payload follows this four-byte offset field, so its position is the
	// header length after this write, not before it.
	order.PutUint32(u32[:], uint32(hdr.Len()+4))
	_, _ = hdr.Write(u32[:])

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(hdr.Bytes()); err != nil {
		return err
	}
	_, err = f.Write(encoded)
	return err
}

// verify confirms the collected artifacts are non-empty and hashes them before
// anything is published.
func (r *desktopRunner) verify(ctx context.Context, log io.Writer) error {
	found, err := r.collectArtifacts()
	if err != nil {
		return err
	}
	for _, a := range found {
		if _, err = fmt.Fprintf(log, "[verify] %s %s sha256=%s size=%d\n", a.Channel, a.FileName, a.SHA256, a.Size); err != nil {
			return err
		}
	}
	return nil
}

func (r *desktopRunner) collectArtifacts() ([]Artifact, error) {
	bundleDir := filepath.Join(r.cfg.DesktopWorkspace, "target", "release", "bundle")
	var out []Artifact
	for _, ch := range r.channels() {
		dir := filepath.Join(bundleDir, ch)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("bundle dir %s: %w", dir, err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			if !desktopArtifactSuffix(ch, name) {
				continue
			}
			p := filepath.Join(dir, name)
			fi, err := os.Stat(p)
			if err != nil || fi.Size() == 0 {
				continue
			}
			sum, err := fileSHA256(p)
			if err != nil {
				return nil, fmt.Errorf("hash %s: %w", name, err)
			}
			out = append(out, Artifact{
				Channel:  ch,
				FileName: name,
				Path:     p,
				SHA256:   sum,
				Size:     fi.Size(),
			})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no artifacts found under %s for channels %v", bundleDir, r.channels())
	}
	return out, nil
}

func desktopArtifactSuffix(channel, name string) bool {
	switch channel {
	case domain.ChannelAppImage:
		return strings.HasSuffix(name, ".appimage")
	case domain.ChannelDeb:
		return strings.HasSuffix(name, ".deb")
	case domain.ChannelRpm:
		return strings.HasSuffix(name, ".rpm")
	}
	return false
}

// desktopOverlaySpec is what the tauri config fragment is built from.
type desktopOverlaySpec struct {
	ProductName     string
	BundleID        string
	Version         string
	Channels        []string
	UpdaterEndpoint string
	// IconFiles is the icon set to bundle. Empty keeps the shell's own icons.
	IconFiles []string
}

// desktopTauriOverlay is the tauri.conf.json fragment one desktop job writes.
// Only the keys that differ from the checked-in config are set, so the base
// file stays the single source of windows, permissions and capabilities.
func desktopTauriOverlay(spec desktopOverlaySpec) map[string]any {
	targets := make([]string, 0, len(spec.Channels))
	for _, ch := range spec.Channels {
		targets = append(targets, ch)
	}
	bundle := map[string]any{
		"active":  true,
		"targets": targets,
		"externalBin": []string{
			filepath.Join(desktopSidecarDir, desktopEngineName),
		},
		"resources": map[string]string{
			filepath.Join(desktopExtStaging, "extension.zip"): "browser-extension",
			filepath.Join(desktopRuntimeDir, "skills"):        "skills",
		},
		"linux": map[string]any{
			"deb": map[string]any{
				"recommends": []string{"gstreamer1.0-plugins-good", "gstreamer1.0-libav"},
				"section":    "devel",
			},
			"rpm": map[string]any{
				"depends":    []string{"webkit2gtk4.1", "gtk3"},
				"recommends": []string{"libappindicator-gtk3", "gstreamer1-plugins-good", "gstreamer1-libav"},
			},
			"appimage": map[string]any{
				"bundleMediaFramework": true,
			},
		},
	}
	if len(spec.IconFiles) > 0 {
		bundle["icon"] = spec.IconFiles
	}
	overlay := map[string]any{
		"bundle": bundle,
	}
	if spec.ProductName != "" {
		overlay["productName"] = spec.ProductName
	}
	if spec.BundleID != "" {
		overlay["identifier"] = spec.BundleID
	}
	if spec.Version != "" {
		overlay["version"] = spec.Version
	}
	if spec.UpdaterEndpoint != "" {
		overlay["plugins"] = map[string]any{
			"updater": map[string]any{
				"plugins":   []string{desktopUpdaterBase},
				"endpoints": []string{spec.UpdaterEndpoint},
			},
		}
	}
	return overlay
}

// gitCheckout checks out branch in dir. When HEAD already resolves to branch
// the tree is fast-forwarded; otherwise it is reset to the branch first, so a
// dirty checkout cannot leak into the next build.
func gitCheckout(ctx context.Context, dir, branch string, timeout time.Duration, env []string, log io.Writer) error {
	if branch == "" {
		return fmt.Errorf("empty git branch")
	}
	current, err := runOutput(ctx, dir, env, "git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, timeout)
	if err != nil {
		return fmt.Errorf("git rev-parse: %w", err)
	}
	if strings.TrimSpace(current) == branch {
		return runStep(ctx, dir, env, "git", []string{"pull", "--ff-only"}, timeout, log)
	}
	if err = runStep(ctx, dir, env, "git", []string{"checkout", "--force", branch}, timeout, log); err != nil {
		return fmt.Errorf("git checkout %s: %w", branch, err)
	}
	return runStep(ctx, dir, env, "git", []string{"pull", "--ff-only"}, timeout, log)
}

// runOutput runs a command and returns its stdout.
func runOutput(ctx context.Context, dir string, env []string, name string, args []string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(stepCtx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// findEngineBinary locates the linux engine binary after `make linux`. The
// engine repo's output layout is not part of our contract, so the well-known
// paths are probed first and a bounded walk is the fallback.
func findEngineBinary(root string) (string, error) {
	candidates := []string{
		filepath.Join("desktop-engine-binaries", "ohmyagent"),
		filepath.Join("desktop-engine-binaries", "ohmyagent-linux-x86_64"),
		filepath.Join("dist", desktopEngineName),
		filepath.Join("target", "release", desktopEngineName),
		filepath.Join("bin", desktopEngineName),
	}
	for _, rel := range candidates {
		p := filepath.Join(root, rel)
		fi, err := os.Stat(p)
		if err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
			return p, nil
		}
	}
	found := ""
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if found != "" {
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if n == ".git" || n == "node_modules" || n == "target" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.Contains(d.Name(), desktopEngineName) {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return nil
		}
		found = p
		return filepath.SkipAll
	})
	if found == "" {
		return "", fmt.Errorf("no %s binary under %s", desktopEngineName, root)
	}
	return found, nil
}

// findExtensionZip locates the built browser extension archive.
func findExtensionZip(root string) (string, error) {
	preferred := []string{
		filepath.Join("dist", "extension.zip"),
		filepath.Join("dist", "browser-extension.zip"),
		"extension.zip",
	}
	for _, rel := range preferred {
		p := filepath.Join(root, rel)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Size() > 0 {
			return p, nil
		}
	}
	var found []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d.IsDir() {
				n := d.Name()
				if n == ".git" || n == "node_modules" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".zip") {
			found = append(found, p)
		}
		return nil
	})
	if len(found) == 0 {
		return "", fmt.Errorf("no extension archive under %s", root)
	}
	sort.Strings(found)
	return found[0], nil
}

// copyDir copies every regular file under src into dst, keeping the relative
// path. Symlinks are skipped: they point outside the source tree.
func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
}

func defaultIfEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
