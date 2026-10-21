import { useCallback, useEffect, useRef, useState } from "react";
import {
  Boxes,
  Download,
  Image as ImageIcon,
  Palette,
  RefreshCw,
  Rocket,
  Smartphone,
  Terminal,
  Trash2,
  Upload,
} from "lucide-react";
import { toast } from "sonner";

import { Api } from "@/api/Api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { Separator } from "@/components/ui/separator";
import { Spinner } from "@/components/ui/spinner";
import { useTranslation } from "react-i18next";

const VERSION_RE = /^\d{6,8}$/;
const POLL_INTERVAL_MS = 5000;

type AssetSlotInfo = {
  slot: string;
  key?: string;
  url?: string;
  bytes: number;
  set: boolean;
};

type BuilderStatus = {
  enabled: boolean;
  workspace: string;
  platforms: string[];
  asset_slots: AssetSlotInfo[];
  build_steps: string[];
  version_pattern: string;
  max_asset_bytes: number;
  gradle_timeout: string;
  next_version_code: number;
};

type Branding = {
  name: string;
  version: string;
  version_code: number;
  server_url: string;
  updates_server?: string;
  assets: AssetSlotInfo[];
};

type BuildJob = {
  id: string;
  platform: string;
  target_version: string;
  version_code: number;
  status: string;
  current_step?: string;
  error?: string;
  exit_code?: number;
  log_tail?: string;
  artifact_path?: string;
  artifact_sha256?: string;
  artifact_size: number;
  duration_ms: number;
  enqueued_at: string;
  started_at?: string;
  finished_at?: string;
  published_at?: string;
  artifact_download_url?: string;
};

type ReleaseManifest = { version: string; url: string };

type ReleaseStatus = {
  platform: string;
  enabled: boolean;
  manifest?: ReleaseManifest;
  artifact_url?: string;
  artifact_present: boolean;
  artifact_sha256?: string;
};

type OtaStatus = {
  enabled: boolean;
  url?: string;
  manifest?: ReleaseManifest;
};

type Releases = {
  native?: ReleaseStatus[];
  ota?: OtaStatus;
};

type Envelope = {
  code?: number;
  message?: string;
  data?: any;
};

function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === "object") {
    const body = error as { message?: string; data?: { message?: string } };
    if (body.data?.message) return body.data.message;
    if (body.message) return body.message;
  }
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return "-";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`;
}

function formatTime(value?: string): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const locale = navigator.language?.toLowerCase().startsWith("zh") ? "zh-CN" : "en-US";
  return date.toLocaleString(locale, { hour12: false }).replace(/\//g, "-");
}

function formatDuration(ms?: number): string {
  if (!ms || ms <= 0) return "-";
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
}

function shortSha(sha?: string): string {
  if (!sha) return "-";
  return sha.length > 12 ? `${sha.slice(0, 12)}…` : sha;
}

export default function TeamManagerAppBuilder() {
  const { t } = useTranslation();
  const api = new Api();

  const [status, setStatus] = useState<BuilderStatus | null>(null);
  const [branding, setBranding] = useState<Branding | null>(null);
  const [jobs, setJobs] = useState<BuildJob[]>([]);
  const [releases, setReleases] = useState<Releases | null>(null);

  const [statusLoading, setStatusLoading] = useState(true);
  const [brandingName, setBrandingName] = useState("");
  const [brandingVersion, setBrandingVersion] = useState("");
  const [brandingVersionCode, setBrandingVersionCode] = useState("1");
  const [brandingServerUrl, setBrandingServerUrl] = useState("");
  const [brandingUpdatesServer, setBrandingUpdatesServer] = useState("");
  const [savingBranding, setSavingBranding] = useState(false);

  const [buildVersion, setBuildVersion] = useState("");
  const [buildVersionCode, setBuildVersionCode] = useState("1");
  const [startingBuild, setStartingBuild] = useState(false);
  const [busyJobId, setBusyJobId] = useState<string | null>(null);

  const [expandedJobId, setExpandedJobId] = useState<string | null>(null);
  const [localPreviews, setLocalPreviews] = useState<Record<string, string>>({});
  const [busyAssetSlot, setBusyAssetSlot] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const pendingAssetSlot = useRef<string | null>(null);

  const activeJob = jobs.find((job) => job.status === "queued" || job.status === "running");
  const activeJobId = activeJob?.id ?? null;
  const currentJob = jobs.find((job) => job.id === expandedJobId) ?? null;
  const androidRelease = releases?.native?.find((item) => item.platform === "android");

  const loadStatus = useCallback(async () => {
    try {
      const resp = await api.api.v1AdminAppBuilderStatusList();
      if (resp.ok && resp.data?.code === 0) {
        const data = resp.data.data as BuilderStatus;
        setStatus(data);
        setBuildVersionCode((value) =>
          value && value !== "1" ? value : String(data.next_version_code || 1)
        );
      } else {
        setStatus(null);
      }
    } catch {
      setStatus(null);
    } finally {
      setStatusLoading(false);
    }
  }, [api]);

  const loadBranding = useCallback(async () => {
    try {
      const resp = await api.api.v1AdminAppBuilderBrandingList();
      if (resp.ok && resp.data?.code === 0) {
        const data = resp.data.data as Branding;
        setBranding(data);
        setBrandingName(data.name || "");
        setBrandingVersion(data.version || "");
        setBrandingVersionCode(String(data.version_code || 1));
        setBrandingServerUrl(data.server_url || "");
        setBrandingUpdatesServer(data.updates_server || "");
        setBuildVersion((value) => value || data.version || "");
      }
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.branding.loadFailed", { message: "" })));
    }
  }, [api, t]);

  const loadJobs = useCallback(async () => {
    try {
      const resp = await api.api.v1AdminAppBuilderBuildsList({ limit: 30 });
      if (resp.ok && resp.data?.code === 0) {
        setJobs((resp.data.data as BuildJob[]) ?? []);
      }
    } catch {
      // The caller keeps the previous list and retries on the next tick.
    }
  }, [api]);

  const loadReleases = useCallback(async () => {
    try {
      const resp = await api.api.v1AdminAppBuilderReleasesList();
      if (resp.ok && resp.data?.code === 0) {
        setReleases(resp.data.data as Releases);
      }
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.releases.loadFailed", { message: "" })));
    }
  }, [api, t]);

  useEffect(() => {
    void loadStatus();
    void loadBranding();
    void loadJobs();
    void loadReleases();
  }, [loadStatus, loadBranding, loadJobs, loadReleases]);

  useEffect(() => {
    if (!activeJobId) return;
    const timer = window.setInterval(() => {
      void loadJobs();
      void loadReleases();
    }, POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [activeJobId, loadJobs, loadReleases]);

  const handleSaveBranding = async () => {
    if (!brandingName.trim()) {
      toast.error(t("managerAppBuilder.branding.nameRequired"));
      return;
    }
    if (!VERSION_RE.test(brandingVersion.trim())) {
      toast.error(t("managerAppBuilder.branding.versionInvalid"));
      return;
    }
    const versionCode = Number(brandingVersionCode);
    if (!Number.isInteger(versionCode) || versionCode <= 0) {
      toast.error(t("managerAppBuilder.branding.versionCodeInvalid"));
      return;
    }
    if (!brandingServerUrl.trim()) {
      toast.error(t("managerAppBuilder.branding.serverUrlRequired"));
      return;
    }
    setSavingBranding(true);
    try {
      const resp = await api.api.v1AdminAppBuilderBrandingUpdate({
        name: brandingName.trim(),
        version: brandingVersion.trim(),
        version_code: versionCode,
        server_url: brandingServerUrl.trim(),
        updates_server: brandingUpdatesServer.trim(),
      });
      if (resp.ok && resp.data?.code === 0) {
        setBranding(resp.data.data as Branding);
        toast.success(t("managerAppBuilder.branding.saved"));
        void loadReleases();
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("managerAppBuilder.branding.saveFailed", { message: "" })));
      }
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.branding.saveFailed", { message: "" })));
    } finally {
      setSavingBranding(false);
    }
  };

  const handlePickAsset = (slot: string) => {
    pendingAssetSlot.current = slot;
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
      fileInputRef.current.click();
    }
  };

  const handleAssetFile = async (file: File | null) => {
    const slot = pendingAssetSlot.current;
    pendingAssetSlot.current = null;
    if (!slot || !file) return;
    if (file.type !== "image/png") {
      toast.error(t("managerAppBuilder.assets.pngOnly"));
      return;
    }
    setBusyAssetSlot(slot);
    try {
      const previewUrl = URL.createObjectURL(file);
      setLocalPreviews((prev) => ({ ...prev, [slot]: previewUrl }));
      const resp = await api.api.v1AdminAppBuilderBrandingAssetCreate(slot, file);
      if (resp.ok && resp.data?.code === 0) {
        setBranding(resp.data.data as Branding);
        toast.success(t("managerAppBuilder.assets.uploaded"));
      } else {
        setLocalPreviews((prev) => {
          const next = { ...prev };
          delete next[slot];
          return next;
        });
        toast.error(errorMessage(resp.data as Envelope, t("managerAppBuilder.assets.uploadFailed", { message: "" })));
      }
    } catch (error) {
      setLocalPreviews((prev) => {
        const next = { ...prev };
        delete next[slot];
        return next;
      });
      toast.error(errorMessage(error, t("managerAppBuilder.assets.uploadFailed", { message: "" })));
    } finally {
      setBusyAssetSlot(null);
    }
  };

  const handleClearAsset = async (slot: string) => {
    setBusyAssetSlot(slot);
    try {
      const resp = await api.api.v1AdminAppBuilderBrandingAssetDelete(slot);
      if (resp.ok && resp.data?.code === 0) {
        setBranding(resp.data.data as Branding);
        setLocalPreviews((prev) => {
          const next = { ...prev };
          delete next[slot];
          return next;
        });
        toast.success(t("managerAppBuilder.assets.cleared"));
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("managerAppBuilder.assets.clearFailed", { message: "" })));
      }
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.assets.clearFailed", { message: "" })));
    } finally {
      setBusyAssetSlot(null);
    }
  };

  const handleStartBuild = async () => {
    if (!status?.enabled) return;
    if (activeJob) {
      toast.error(t("managerAppBuilder.build.busy"));
      return;
    }
    const version = buildVersion.trim();
    if (!VERSION_RE.test(version)) {
      toast.error(t("managerAppBuilder.branding.versionInvalid"));
      return;
    }
    const versionCode = Number(buildVersionCode);
    if (!Number.isInteger(versionCode) || versionCode <= 0) {
      toast.error(t("managerAppBuilder.branding.versionCodeInvalid"));
      return;
    }
    setStartingBuild(true);
    try {
      const resp = await api.api.v1AdminAppBuilderBuildsCreate({
        platform: "android",
        version,
        version_code: versionCode,
      });
      if (resp.ok && resp.data?.code === 0) {
        const job = resp.data.data as BuildJob;
        toast.success(t("managerAppBuilder.build.enqueued"));
        setExpandedJobId(job.id);
        setBuildVersionCode(String(job.version_code + 1));
        void loadJobs();
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("managerAppBuilder.build.enqueueFailed", { message: "" })));
      }
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.build.enqueueFailed", { message: "" })));
    } finally {
      setStartingBuild(false);
    }
  };

  const handleDownload = async (job: BuildJob) => {
    setBusyJobId(job.id);
    try {
      const resp = await api.api.v1AdminAppBuilderBuildsDownload(job.id);
      if (!resp.ok) {
        toast.error(t("managerAppBuilder.build.downloadFailed"));
        return;
      }
      const blob = await (resp.data as unknown as Response).blob();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = "authyide.apk";
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 10000);
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.build.downloadFailed")));
    } finally {
      setBusyJobId(null);
    }
  };

  const handlePublish = async (job: BuildJob) => {
    setBusyJobId(job.id);
    try {
      const resp = await api.api.v1AdminAppBuilderBuildsPublish(job.id);
      if (resp.ok && resp.data?.code === 0) {
        toast.success(t("managerAppBuilder.build.published"));
        void loadReleases();
        void loadJobs();
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("managerAppBuilder.build.publishFailed", { message: "" })));
      }
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.build.publishFailed", { message: "" })));
    } finally {
      setBusyJobId(null);
    }
  };

  const handleRefresh = () => {
    void loadBranding();
    void loadJobs();
    void loadReleases();
  };

  if (statusLoading) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-24">
        <Spinner className="size-5" />
      </div>
    );
  }

  if (!status?.enabled) {
    return (
      <div className="flex flex-col gap-4">
        <Card className="w-full shadow-none">
          <CardContent className="flex flex-col gap-2 pt-6">
            <div className="flex items-center gap-2">
              <Smartphone className="size-4" />
              <CardTitle>{t("managerAppBuilder.title")}</CardTitle>
            </div>
            <CardDescription>{t("managerAppBuilder.subtitle")}</CardDescription>
          </CardContent>
        </Card>
        <Alert>
          <Smartphone className="size-4" />
          <AlertTitle>{t("managerAppBuilder.disabled.title")}</AlertTitle>
          <AlertDescription>{t("managerAppBuilder.disabled.description")}</AlertDescription>
        </Alert>
      </div>
    );
  }

  const assets = branding?.assets?.length ? branding.assets : status.asset_slots;
  const buildSteps = status.build_steps.length ? status.build_steps : [];

  return (
    <div className="flex flex-col gap-4">
      <input
        ref={fileInputRef}
        type="file"
        accept="image/png"
        className="hidden"
        onChange={(event) => {
          void handleAssetFile(event.target.files?.[0] ?? null);
        }}
      />

      <Card className="w-full shadow-none">
        <CardContent className="flex flex-wrap items-center gap-3 pt-6">
          <div className="flex items-center gap-2">
            <Smartphone className="size-4" />
            <CardTitle className="text-lg">{t("managerAppBuilder.title")}</CardTitle>
          </div>
          <CardDescription className="flex-1 basis-64">{t("managerAppBuilder.subtitle")}</CardDescription>
          <Badge variant="secondary">{t("managerAppBuilder.enabled")}</Badge>
        </CardContent>
      </Card>

      <Card className="w-full shadow-none">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Palette />
            {t("managerAppBuilder.branding.title")}
          </CardTitle>
          <CardDescription>{t("managerAppBuilder.branding.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-builder-name">{t("managerAppBuilder.branding.name")}</Label>
              <Input
                id="app-builder-name"
                value={brandingName}
                placeholder={t("managerAppBuilder.branding.namePlaceholder")}
                onChange={(event) => setBrandingName(event.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-builder-version">
                {t("managerAppBuilder.branding.version")}
              </Label>
              <Input
                id="app-builder-version"
                value={brandingVersion}
                placeholder="260608"
                onChange={(event) => setBrandingVersion(event.target.value)}
              />
              <span className="text-xs text-muted-foreground">
                {t("managerAppBuilder.branding.versionHint")}
              </span>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-builder-version-code">
                {t("managerAppBuilder.branding.versionCode")}
              </Label>
              <Input
                id="app-builder-version-code"
                type="number"
                min={1}
                value={brandingVersionCode}
                onChange={(event) => setBrandingVersionCode(event.target.value)}
              />
              <span className="text-xs text-muted-foreground">
                {t("managerAppBuilder.branding.versionCodeHint")}
              </span>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-builder-server-url">
                {t("managerAppBuilder.branding.serverUrl")}
              </Label>
              <Input
                id="app-builder-server-url"
                value={brandingServerUrl}
                placeholder={t("managerAppBuilder.branding.serverUrlPlaceholder")}
                onChange={(event) => setBrandingServerUrl(event.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2 sm:col-span-2">
              <Label htmlFor="app-builder-updates-server">
                {t("managerAppBuilder.branding.updatesServer")}
              </Label>
              <Input
                id="app-builder-updates-server"
                value={brandingUpdatesServer}
                placeholder={t("managerAppBuilder.branding.updatesServerPlaceholder")}
                onChange={(event) => setBrandingUpdatesServer(event.target.value)}
              />
            </div>
          </div>
          <div className="flex justify-end">
            <Button onClick={() => void handleSaveBranding()} disabled={savingBranding}>
              {savingBranding ? (
                <Spinner className="size-4" />
              ) : null}
              {savingBranding
                ? t("managerAppBuilder.branding.saving")
                : t("managerAppBuilder.branding.save")}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card className="w-full shadow-none">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ImageIcon />
            {t("managerAppBuilder.assets.title")}
          </CardTitle>
          <CardDescription>{t("managerAppBuilder.assets.description")}</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {assets.map((slot) => {
              const previewUrl = localPreviews[slot.slot] || (slot.set ? slot.url : undefined);
              const busy = busyAssetSlot === slot.slot;
              return (
                <div
                  key={slot.slot}
                  className="flex flex-col gap-2 rounded-lg border p-3"
                >
                  <div className="flex items-center gap-3">
                    <div className="flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-md border bg-muted">
                      {previewUrl ? (
                        <img src={previewUrl} alt={slot.slot} className="size-full object-contain" />
                      ) : (
                        <ImageIcon className="size-5 text-muted-foreground" />
                      )}
                    </div>
                    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                      <span className="truncate text-sm font-medium">
                        {t(`managerAppBuilder.assets.slots.${slot.slot}`)}
                      </span>
                      <span className="truncate text-xs text-muted-foreground">
                        {t(`managerAppBuilder.assets.hints.${slot.slot}`)}
                      </span>
                    </div>
                  </div>
                  <div className="flex items-center justify-between gap-2">
                    {slot.set ? (
                      <Badge variant="secondary">
                        {t("managerAppBuilder.assets.customised")}
                        {slot.bytes ? ` · ${formatBytes(slot.bytes)}` : ""}
                      </Badge>
                    ) : (
                      <Badge variant="outline">{t("managerAppBuilder.assets.defaultUsed")}</Badge>
                    )}
                    <div className="flex gap-1.5">
                      {slot.set ? (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={busy}
                          onClick={() => void handleClearAsset(slot.slot)}
                        >
                          {busy ? <Spinner className="size-3.5" /> : <Trash2 className="size-3.5" />}
                          {t("managerAppBuilder.assets.clear")}
                        </Button>
                      ) : null}
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy}
                        onClick={() => handlePickAsset(slot.slot)}
                      >
                        {busy ? <Spinner className="size-3.5" /> : <Upload className="size-3.5" />}
                        {slot.set
                          ? t("managerAppBuilder.assets.replace")
                          : t("managerAppBuilder.assets.upload")}
                      </Button>
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </CardContent>
      </Card>

      <Card className="w-full shadow-none">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Boxes />
            {t("managerAppBuilder.build.title")}
          </CardTitle>
          <CardDescription>{t("managerAppBuilder.build.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-builder-target-version">
                {t("managerAppBuilder.build.version")}
              </Label>
              <Input
                id="app-builder-target-version"
                value={buildVersion}
                placeholder={t("managerAppBuilder.build.versionPlaceholder")}
                onChange={(event) => setBuildVersion(event.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-builder-target-version-code">
                {t("managerAppBuilder.build.versionCode")}
              </Label>
              <Input
                id="app-builder-target-version-code"
                type="number"
                min={1}
                value={buildVersionCode}
                onChange={(event) => setBuildVersionCode(event.target.value)}
              />
            </div>
            <div className="flex items-end">
              <Button onClick={() => void handleStartBuild()} disabled={startingBuild || Boolean(activeJob)} className="w-full">
                {startingBuild ? <Spinner className="size-4" /> : <Rocket className="size-4" />}
                {startingBuild
                  ? t("managerAppBuilder.build.starting")
                  : t("managerAppBuilder.build.start")}
              </Button>
            </div>
          </div>

          <Separator />

          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">{t("managerAppBuilder.build.history")}</h3>
            <Button variant="ghost" size="sm" onClick={handleRefresh}>
              <RefreshCw className="size-4" />
              {t("managerAppBuilder.releases.refresh")}
            </Button>
          </div>

          {jobs.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t("managerAppBuilder.build.emptyHistory")}
            </p>
          ) : (
            <div className="flex flex-col gap-2">
              {jobs.map((job) => {
                const expanded = expandedJobId === job.id;
                const isActive = job.status === "queued" || job.status === "running";
                const stepIndex = isActive && job.current_step
                  ? buildSteps.indexOf(job.current_step)
                  : -1;
                return (
                  <div key={job.id} className="rounded-lg border">
                    <div className="flex flex-wrap items-center gap-3 p-3">
                      <div className="flex flex-col gap-0.5">
                        <span className="text-sm font-medium">
                          {job.target_version}
                          <span className="ml-2 text-xs font-normal text-muted-foreground">
                            #{job.version_code}
                          </span>
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {formatTime(job.enqueued_at)}
                        </span>
                      </div>
                      <Badge
                        variant={
                          job.status === "succeeded"
                            ? "secondary"
                            : job.status === "failed"
                              ? "destructive"
                              : "outline"
                        }
                      >
                        {isActive ? <Spinner className="size-3" /> : null}
                        {t(`managerAppBuilder.build.status.${job.status}`, job.status)}
                      </Badge>
                      <span className="text-xs text-muted-foreground">
                        {isActive && job.current_step
                          ? t(`managerAppBuilder.build.steps.${job.current_step}`, job.current_step)
                          : job.status === "succeeded"
                            ? `${formatDuration(job.duration_ms)} · ${formatBytes(job.artifact_size)}`
                            : formatDuration(job.duration_ms) !== "-"
                              ? formatDuration(job.duration_ms)
                              : ""}
                      </span>
                      <div className="ml-auto flex flex-wrap items-center gap-1.5">
                        {isActive ? (
                          <Button variant="outline" size="sm" onClick={() => setExpandedJobId(expanded ? null : job.id)}>
                            <Terminal className="size-3.5" />
                            {t("managerAppBuilder.build.log")}
                          </Button>
                        ) : null}
                        {job.status === "succeeded" && !job.published_at ? (
                          <Button
                            size="sm"
                            disabled={busyJobId === job.id}
                            onClick={() => void handlePublish(job)}
                          >
                            {busyJobId === job.id ? <Spinner className="size-3.5" /> : <Rocket className="size-3.5" />}
                            {t("managerAppBuilder.build.publish")}
                          </Button>
                        ) : null}
                        {job.status === "succeeded" ? (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busyJobId === job.id}
                            onClick={() => void handleDownload(job)}
                          >
                            {busyJobId === job.id ? <Spinner className="size-3.5" /> : <Download className="size-3.5" />}
                            {t("managerAppBuilder.build.download")}
                          </Button>
                        ) : null}
                        {job.status === "succeeded" && job.published_at ? (
                          <Badge variant="secondary">{formatTime(job.published_at)}</Badge>
                        ) : null}
                      </div>
                    </div>
                    {isActive ? (
                      <div className="border-t px-3 py-2">
                        <Progress value={stepIndex >= 0 ? ((stepIndex + 1) / Math.max(buildSteps.length, 1)) * 100 : 0} />
                      </div>
                    ) : null}
                    {expanded && currentJob ? (
                      <JobLog job={currentJob} />
                    ) : null}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="w-full shadow-none">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Rocket />
            {t("managerAppBuilder.releases.title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-4 lg:grid-cols-2">
            <div className="flex flex-col gap-2 rounded-lg border p-3">
              <div className="flex items-center gap-2">
                <Boxes className="size-4" />
                <span className="text-sm font-medium">
                  {t("managerAppBuilder.releases.native")}
                </span>
                <Badge variant={androidRelease?.enabled ? "secondary" : "outline"}>
                  {androidRelease?.enabled
                    ? t("managerAppBuilder.releases.enabled")
                    : t("managerAppBuilder.releases.disabled")}
                </Badge>
              </div>
              <span className="text-xs text-muted-foreground">
                {t("managerAppBuilder.releases.nativeDescription")}
              </span>
              <div className="flex flex-wrap items-center gap-2 pt-1">
                {androidRelease?.manifest ? (
                  <>
                    <Badge variant="secondary">v{androidRelease.manifest.version}</Badge>
                    <a
                      href={androidRelease.manifest.url}
                      target="_blank"
                      rel="noreferrer"
                      className="truncate text-xs text-primary underline"
                    >
                      {androidRelease.manifest.url}
                    </a>
                  </>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {t("managerAppBuilder.releases.notPublished")}
                  </span>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {androidRelease?.artifact_url ? (
                  <a
                    href={androidRelease.artifact_url}
                    target="_blank"
                    rel="noreferrer"
                    className="text-xs text-primary underline"
                  >
                    {t("managerAppBuilder.releases.artifact")}
                  </a>
                ) : null}
                <Badge variant={androidRelease?.artifact_present ? "outline" : "destructive"}>
                  {androidRelease?.artifact_present
                    ? t("managerAppBuilder.releases.artifactPresent")
                    : t("managerAppBuilder.releases.artifactMissing")}
                </Badge>
                {androidRelease?.artifact_sha256 ? (
                  <span className="font-mono text-xs text-muted-foreground">
                    {shortSha(androidRelease.artifact_sha256)}
                  </span>
                ) : null}
              </div>
            </div>

            <div className="flex flex-col gap-2 rounded-lg border p-3">
              <div className="flex items-center gap-2">
                <Smartphone className="size-4" />
                <span className="text-sm font-medium">
                  {t("managerAppBuilder.releases.updateServer")}
                </span>
                <Badge variant={branding?.updates_server ? "secondary" : "outline"}>
                  {branding?.updates_server
                    ? t("managerAppBuilder.releases.enabled")
                    : t("managerAppBuilder.releases.disabled")}
                </Badge>
              </div>
              <span className="text-xs text-muted-foreground">
                {t("managerAppBuilder.releases.updateServerDescription")}
              </span>
              <div className="flex flex-wrap items-center gap-2 pt-1">
                {branding?.updates_server ? (
                  <code className="truncate rounded bg-muted px-2 py-1 text-xs">
                    {branding.updates_server}
                  </code>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {t("managerAppBuilder.releases.updateServerNotSet")}
                  </span>
                )}
              </div>
              {androidRelease?.manifest && branding?.updates_server ? (
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-xs text-muted-foreground">
                    {t("managerAppBuilder.releases.manifestPath")}
                  </span>
                  <code className="truncate rounded bg-muted px-2 py-1 text-xs">
                    {branding.updates_server.replace(/\/$/, "")}/app-version/android.json
                  </code>
                </div>
              ) : null}
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function JobLog({ job }: { job: BuildJob }) {
  const { t } = useTranslation();
  if (!job.error && !job.log_tail) {
    return (
      <div className="border-t px-3 py-3 text-xs text-muted-foreground">
        {t("managerAppBuilder.build.logEmpty")}
      </div>
    );
  }
  return (
    <div className="flex flex-col gap-2 border-t px-3 py-3">
      {job.error ? (
        <Alert variant="destructive">
          <AlertTitle>{t("managerAppBuilder.build.error")}</AlertTitle>
          <AlertDescription className="whitespace-pre-wrap break-all">
            {job.error}
          </AlertDescription>
        </Alert>
      ) : null}
      {job.log_tail ? (
        <pre className="max-h-72 overflow-auto rounded-md bg-muted p-3 font-mono text-xs">
          {job.log_tail}
        </pre>
      ) : null}
      {job.artifact_sha256 ? (
        <div className="flex flex-wrap gap-3 text-xs text-muted-foreground">
          <span>
            {t("managerAppBuilder.build.sha256")}: <span className="font-mono">{job.artifact_sha256}</span>
          </span>
          <span>
            {t("managerAppBuilder.build.size")}: {formatBytes(job.artifact_size)}
          </span>
        </div>
      ) : null}
    </div>
  );
}
