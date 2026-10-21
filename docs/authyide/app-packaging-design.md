# AuthyIDE — APP metadata, packaging and licensing

Status: design (implementation pending toolchains — see "Prerequisites").

## Goal

Turn AuthyIDE from "builds websites" into "builds apps": a user configures their
app in the user center (name, icon, optional ad network), **previews it live
while it is being developed**, and the platform produces installable artifacts
for Web / Android / iOS / HarmonyOS, plus a paid Desktop tier, from one Flutter
codebase.

**Scope note on ads**: AuthyIDE itself serves no ads and provides no
platform-level ad network. The ad network is an optional attribute of the *user's
generated app* — the user brings their own network account (AdMob / 穿山甲 /
优量汇 ...) and AuthyIDE only wires it into the built app.

## Data model (backend)

Reuse the existing ent + domain layering. New entities:

```
App
  id            uuid
  owner_id      uuid (user)
  name          string        # shown on device
  package_id    string        # com.example.app (immutable forever once set)
  icon_key      string        # object storage key of the uploaded icon
  build_count   int           # successful builds so far
  locked        bool          # true once a free-tier build succeeds
  created_at / updated_at

AppAdConfig                      # 1:1 with App — the USER APP's own ads, optional
  app_id              uuid
  network             string     # registry id: none|admob|pangle|gdt|unity|applovin
  app_id_external     string     # the user's account id at that network
  placements          json       # {banner, interstitial, rewarded}
  enabled_platforms   []string   # subset of web|android|ios|harmony|desktop

Entitlement                      # per user
  user_id            uuid
  tier               string     # free | pro
  free_builds_used   int        # free tier: allowed exactly 1 successful build
  desktop_licensed   bool

AppBuildJob
  id            uuid
  app_id        uuid
  platforms     []string
  status        pending|running|succeeded|failed
  artifacts     json        # {platform: download_url}
  log_key       string
  created_at / finished_at

License                          # reuse private-deployment license module
  id            uuid
  user_id       uuid
  product       string      # desktop
  machine_id    string
  signature     string      # ed25519, bound to machine_id
  issued_at / expires_at(null = perpetual)
```

## API

| Method | Path                                | Purpose                              |
| ------ | ----------------------------------- | ------------------------------------ |
| GET    | `/api/settings/branding`            | product name / icon / theme (global) |
| GET    | `/api/user/apps`                    | list apps (incl. `locked`, `build_count`) |
| POST   | `/api/user/apps`                    | create app (name, icon upload)       |
| PUT    | `/api/user/apps/{id}`               | edit app — rejected with 409 when locked |
| PUT    | `/api/user/apps/{id}/ads`           | set the user app's ad network + placements |
| POST   | `/api/user/apps/{id}/builds`        | request build (platforms)            |
| GET    | `/api/user/apps/{id}/builds/{job}`  | status + artifact URLs               |
| POST   | `/api/user/apps/{id}/preview`       | start a live preview session         |
| GET    | `/api/user/apps/{id}/preview`       | preview status + `preview_url`       |
| DELETE | `/api/user/apps/{id}/preview`       | stop the preview session             |
| POST   | `/api/user/apps/{id}/code/tasks`    | AI code modification (Pro only)      |
| GET    | `/api/user/apps/{id}/code/tasks`    | code-change task list / history      |
| POST   | `/api/user/apps/{id}/code/revert`   | revert a code change                 |
| POST   | `/api/user/licenses/desktop`        | issue desktop license (paid)         |

`GET /api/settings/ad-networks` returns `packer/ad_networks.yaml` verbatim so
the user-center picker and the Flutter runtime share one capability table. This
only drives the *user's app*; AuthyIDE serves no ads of its own.

Validation before enqueueing a build:

- every requested (network, platform) pair is supported, else reject with a
  clear message;
- **free tier** with `free_builds_used >= 1` is rejected with `402` and a
  message to upgrade; the free tier allows exactly **one successful build**;
- **AI code modification** (`.../code/tasks`) requires Pro, else `402`;
- Desktop requires an active `desktop` entitlement.

## Free tier and app immutability

The free plan allows **one packaging run**. The rules:

- Before the first build, a free user may create and freely edit the app
  (name, icon, package id, optional ads).
- On the first **successful** build, the app is frozen: `App.locked = true` and
  `Entitlement.free_builds_used = 1`.
- While locked, `PUT /api/user/apps/{id}` and the ads endpoint return `409`; the
  user center renders the editor read-only with an upgrade prompt.
- Pro tier removes the lock: unlimited builds, edits always allowed, and each
  edit can trigger a rebuild.
- **AI code modification is Pro-only** (see below); the free tier cannot change
  the app's code at all.

A failed build does not consume the free quota and does not lock the app.

## User center (frontend)

New page under the user console (`frontend/src/pages/console/user/apps/`):

- app list + create;
- **editor**, the three fields the user center is responsible for:
  1. **APP name** — device display name;
  2. **icon** — upload; cropped to the sizes in `tools/generate_brand_icons.py`;
  3. **the user app's ad network** — picker filtered by target platform, plus
     the user's own network account id and placements (`none` = no ads);
  - also package id (locked once set) and target platforms;
- **code tab (Pro only)** — AI modification / update of the app's source (see
  below);
- **preview tab** — live preview of the app under development (see below);
- **builds tab** — request build, watch status, download artifacts;
- **locked state**: for a free user whose build succeeded, the editor above is
  read-only with an upgrade prompt and the "build" action is hidden. Preview
  stays available because it only views the frozen configuration.

## AI code modification (Pro only)

The user center can change the app's **code**, not just its metadata, by talking
to the AI — the same agent that builds the app in the first place.

- A code-editing input takes a natural-language request ("add a dark mode
  toggle", "change the home layout to a 2-column grid") and runs an AuthyIDE
  agent task against the app's Flutter source in the user's dev workspace.
- Requires **Pro**: free tier gets the editing UI as a disabled, upgrade-gated
  control; the API returns `402` for free users.
- It consumes the normal session / credits accounting, since it is a real agent
  run.
- Every change is applied to the app's source; the preview refresh (below) shows
  the result immediately, and a rebuild packages it.
- Changes are tracked per app so the user can see history and revert.

| Method | Path                              | Purpose                            |
| ------ | --------------------------------- | ---------------------------------- |
| POST   | `/api/user/apps/{id}/code/tasks`  | start an AI code-modification task |
| GET    | `/api/user/apps/{id}/code/tasks`  | task list / history                |
| POST   | `/api/user/apps/{id}/code/revert` | revert a change                    |

## Live preview during development

The user can preview the app they are building before packaging it.

- On demand, the platform scaffolds (or reuses) the app's Flutter web target and
  runs `flutter run -d web-server` inside the user's dev workspace; the dev
  server port is reverse-proxied by the platform and returned as `preview_url`.
- The user center embeds `preview_url` in an iframe with a phone / tablet /
  desktop device frame, so the same code is inspected at real viewport sizes.
- Preview is re-rendered from the current source on each change, so edits to
  name / icon / ad config are visible immediately without a full package.
- Sessions auto-teardown after an idle timeout; `DELETE .../preview` stops it
  early.
- Platform limits shown honestly: preview renders the web target, so native-only
  ad networks (Pangle/GDT/Unity/AppLovin) show no ads in preview even though they
  are embedded in the native build. AdMob-web is the only network previewable.
- Preview requires no build quota and works for both free and Pro tiers; for a
  locked free app it shows the frozen configuration.

## Branding is runtime-configured

The product name is already modelled as "全局设置 → 产品名称". Wire the
`branding` settings into `frontend/src/utils/config.tsx` so the shell name and
icon follow server settings instead of hardcoded strings. This file currently
holds only skill paths; it should gain the branding accessor.

## Prerequisites (external, must be provided)

1. **Apple Developer account + a macOS build runner** — iOS cannot be built on
   Linux. This is a hard requirement.
2. **Google Play / Apple accounts** — for store publishing (if desired).
3. **HarmonyOS toolchain** — the OpenHarmony Flutter fork and DevEco SDK; upstream
   Flutter has no `ohos` target.
4. **Ad network accounts** — AdMob / 穿山甲 / 优量汇 etc. app ids and placements.
5. **Object storage** — for icons and build artifacts.
6. **Worker fleet** — one Linux pool + one macOS pool.
7. **License decision for the fork itself** — see below.

## Licensing / legal (blocking decision)

Upstream MonkeyCode is **AGPL-3.0**. Monetising it (ad revenue, paid desktop)
while keeping the derived source closed is not permitted by AGPL: network users
must be offered the corresponding source. Options:

1. keep the whole fork AGPL-3.0 and publish source (simplest, no cost);
2. obtain a commercial/dual license from the upstream vendor;
3. replace the AGPL components.

Products built *by* users of AuthyIDE are separate from the platform's own
license and are unaffected.

## Not yet implemented here

Backend Go handlers/ent schemas, the user-center React page, the build worker
daemon, and the macOS/HarmonyOS runners are specified but not coded, because
this environment has no Go, Node build, Flutter, or JDK toolchain and very
limited RAM. They are the next implementation phase.
