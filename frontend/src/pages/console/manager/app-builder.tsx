import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Boxes,
  Download,
  Image as ImageIcon,
  Monitor,
  Palette,
  RefreshCw,
  Rocket,
  Smartphone,
  Terminal,
  Trash2,
  TriangleAlert,
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
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useTranslation } from "react-i18next";

const VERSION_RE = /^\d{6,8}$/;
const POLL_INTERVAL_MS = 5000;
const APK_NAME = "authyide.apk";
const DESKTOP_ICON_SLOT = "desktop-icon";
const BUNDLE_IDENTIFIER_RE = /^[a-z][a-z0-9]+(?:\.[a-z][a-z0-9]+){1,}$/i;

const PLATFORM_IDS = ["android", "desktop-linux"] as const;
type PlatformId = (typeof PLATFORM_IDS)[number];

const PLATFORM_LABEL_KEYS = {
  android: "managerAppBuilder.platform.android",
  "desktop-linux": "managerAppBuilder.platform.desktopLinux",
} as const;

type AssetSlotInfo = {
  slot: string;
  key?: string;
  url?: string;
  bytes: number;
  set: boolean;
};

type PlatformStatus = {
  platform: string;
  enabled: boolean;
  reason?: string;
  missing_prereqs?: string[];
  build_steps?: string[];
  channels?: string[];
  workspace?: string;
};

type DesktopStatus = {
  publishable_channels?: string[];
  unsigned?: boolean;
};

type BuilderStatus = {
  platforms?: PlatformStatus[];
  version_pattern?: string;
  asset_slots?: AssetSlotInfo[];
  max_asset_bytes?: number;
  next_version_code?: number;
  desktop?: DesktopStatus | null;
};

type Branding = {
  name: string;
  version: string;
  version_code: number;
  server_url: string;
  updates_server?: string;
  assets: AssetSlotInfo[];
};

type DesktopBranding = {
  id?: string;
  product_name?: string;
  bundle_identifier?: string;
  updater_endpoint?: string;
  icon_key?: string;
  created_at?: string;
  updated_at?: string;
};

type BuildArtifact = {
  channel: string;
  file_name?: string;
  size?: number;
  sha256?: string;
  duration_ms?: number;
  created_at?: string;
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
  artifacts?: BuildArtifact[];
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

type DesktopReleaseManifest = {
  version: string;
  notes?: string;
  pub_date?: string;
  url: string;
  signature?: string;
};

type DesktopReleaseStatus = {
  platform?: string;
  enabled: boolean;
  manifest?: DesktopReleaseManifest | null;
  artifact_present: boolean;
  artifact_sha256?: string;
  unsigned?: boolean;
};

type OtaStatus = {
  enabled: boolean;
  url?: string;
  manifest?: ReleaseManifest;
};

type Releases = {
  native?: ReleaseStatus[];
  desktop?: DesktopReleaseStatus | null;
  ota?: OtaStatus;
};

type Envelope = {
  code?: number;
  message?: string;
  data?: any;
};

function platformIcon(platform: string, className = "size-4") {
  return platform === "desktop-linux" ? (
    <Monitor className={className} />
  ) : (
    <Smartphone className={className} />
  );
}

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

// Steps come from the backend per platform so a failed pipeline can still be
// located even when the backend did not declare them.
const FALLBACK_STEPS: Record<string, string[]> = {
  android: ["overlay-source", "inject-assets", "rewrite-config", "prebuild", "assemble", "verify"],
  "desktop-linux": ["ui-next", "engine", "browser-extension", "shell", "overlay-source", "bundle", "verify"],
};

function stepsOf(platform: string, declared?: string[]): string[] {
  const steps = declared ?? FALLBACK_STEPS[platform] ?? [];
  return steps.length ? steps : FALLBACK_STEPS[platform] ?? [];
}

function compileVersionPattern(pattern?: string): RegExp {
  if (!pattern) return VERSION_RE;
  try {
    return new RegExp(pattern);
  } catch {
    return VERSION_RE;
  }
}

function blockedReasons(entry?: PlatformStatus): string[] {
  if (!entry) return [];
  if (entry.missing_prereqs?.length) return entry.missing_prereqs;
  if (entry.reason) return [entry.reason];
  return [];
}

function channelLabelKey(channel: string): string {
  return `managerAppBuilder.channels.${channel}`;
}

export default function TeamManagerAppBuilder() {
  const { t } = useTranslation();
  const api = new Api();

  const [status, setStatus] = useState<BuilderStatus | null>(null);
  const [branding, setBranding] = useState<Branding | null>(null);
  const [desktopBranding, setDesktopBranding] = useState<DesktopBranding | null>(null);
  const [jobs, setJobs] = useState<BuildJob[]>([]);
  const [releases, setReleases] = useState<Releases | null>(null);

  const [statusLoading, setStatusLoading] = useState(true);
  const [buildPlatform, setBuildPlatform] = useState<PlatformId>("android");
  const [brandingName, setBrandingName] = useState("");
  const [brandingVersion, setBrandingVersion] = useState("");
  const [brandingVersionCode, setBrandingVersionCode] = useState("1");
  const [brandingServerUrl, setBrandingServerUrl] = useState("");
  const [brandingUpdatesServer, setBrandingUpdatesServer] = useState("");
  const [savingBranding, setSavingBranding] = useState(false);

  const [desktopProductName, setDesktopProductName] = useState("");
  const [desktopBundleIdentifier, setDesktopBundleIdentifier] = useState("");
  const [desktopUpdaterEndpoint, setDesktopUpdaterEndpoint] = useState("");
  const [savingDesktopBranding, setSavingDesktopBranding] = useState(false);

  const [buildVersion, setBuildVersion] = useState("");
  const [buildVersionCode, setBuildVersionCode] = useState("1");
  const [startingBuild, setStartingBuild] = useState(false);
  // Jobs hold several artifacts now, so busy tracking is keyed per channel.
  const [busyKey, setBusyKey] = useState<string | null>(null);

  const [expandedJobId, setExpandedJobId] = useState<string | null>(null);
  const [jobArtifacts, setJobArtifacts] = useState<Record<string, BuildArtifact[]>>({});
  const [localPreviews, setLocalPreviews] = useState<Record<string, string>>({});
  const [busyAssetSlot, setBusyAssetSlot] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const desktopIconInputRef = useRef<HTMLInputElement | null>(null);
  const pendingAssetSlot = useRef<string | null>(null);

  const platforms = status?.platforms ?? [];
  const platformEntries = useMemo(
    () =>
      PLATFORM_IDS.map((id) => ({
        id,
        entry: platforms.find((item) => item.platform === id),
      })),
    [platforms]
  );
  const selectedPlatform = platformEntries.find((item) => item.id === buildPlatform);

  // Keep the form pointing at a buildable platform when the instance only has
  // one of them ready.
  useEffect(() => {
    const current = platformEntries.find((item) => item.id === buildPlatform);
    if (current?.entry?.enabled === true) return;
    const firstAvailable = platformEntries.find((item) => item.entry?.enabled === true);
    if (firstAvailable) setBuildPlatform(firstAvailable.id);
  }, [platformEntries, buildPlatform]);
  const versionRe = useMemo(() => compileVersionPattern(status?.version_pattern), [status?.version_pattern]);
  const desktopUnsigned = releases?.desktop?.unsigned === true;

  const activeJob = jobs.find((job) => job.status === "queued" || job.status === "running");
  const activeJobId = activeJob?.id ?? null;
  const currentJob = jobs.find((job) => job.id === expandedJobId) ?? null;
  const androidRelease = releases?.native?.find((item) => item.platform === "android");
  const desktopRelease = releases?.desktop ?? null;

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

  const loadDesktopBranding = useCallback(async () => {
    try {
      const resp = await api.api.v1AdminAppBuilderDesktopBrandingList();
      if (resp.ok && resp.data?.code === 0) {
        const data = (resp.data.data as DesktopBranding | null) ?? null;
        setDesktopBranding(data);
        setDesktopProductName(data?.product_name ?? "");
        setDesktopBundleIdentifier(data?.bundle_identifier ?? "");
        setDesktopUpdaterEndpoint(data?.updater_endpoint ?? "");
      }
    } catch {
      // Desktop branding is optional until the admin saves it once.
    }
  }, [api]);

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
    void loadDesktopBranding();
    void loadJobs();
    void loadReleases();
  }, [loadStatus, loadBranding, loadDesktopBranding, loadJobs, loadReleases]);

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
    if (!versionRe.test(brandingVersion.trim())) {
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

  const handleSaveDesktopBranding = async () => {
    if (!desktopProductName.trim()) {
      toast.error(t("managerAppBuilder.desktopBranding.productNameRequired"));
      return;
    }
    const bundleIdentifier = desktopBundleIdentifier.trim();
    if (!bundleIdentifier) {
      toast.error(t("managerAppBuilder.desktopBranding.bundleIdentifierRequired"));
      return;
    }
    if (!BUNDLE_IDENTIFIER_RE.test(bundleIdentifier)) {
      toast.error(t("managerAppBuilder.desktopBranding.bundleIdentifierInvalid"));
      return;
    }
    setSavingDesktopBranding(true);
    try {
      const resp = await api.api.v1AdminAppBuilderDesktopBrandingUpdate({
        product_name: desktopProductName.trim(),
        bundle_identifier: bundleIdentifier,
        updater_endpoint: desktopUpdaterEndpoint.trim(),
      });
      if (resp.ok && resp.data?.code === 0) {
        const data = (resp.data.data as DesktopBranding | null) ?? null;
        setDesktopBranding(data);
        toast.success(t("managerAppBuilder.desktopBranding.saved"));
        void loadReleases();
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("managerAppBuilder.desktopBranding.saveFailed", { message: "" })));
      }
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.desktopBranding.saveFailed", { message: "" })));
    } finally {
      setSavingDesktopBranding(false);
    }
  };

  const handlePickDesktopIcon = () => {
    if (desktopIconInputRef.current) {
      desktopIconInputRef.current.value = "";
      desktopIconInputRef.current.click();
    }
  };

  const handleDesktopIconFile = async (file: File | null) => {
    if (!file) return;
    if (file.type !== "image/png") {
      toast.error(t("managerAppBuilder.desktopBranding.pngOnly"));
      return;
    }
    setBusyAssetSlot(DESKTOP_ICON_SLOT);
    try {
      const previewUrl = URL.createObjectURL(file);
      setLocalPreviews((prev) => ({ ...prev, [DESKTOP_ICON_SLOT]: previewUrl }));
      const resp = await api.api.v1AdminAppBuilderDesktopBrandingAssetCreate(file);
      if (resp.ok && resp.data?.code === 0) {
        setDesktopBranding((resp.data.data as DesktopBranding | null) ?? null);
        toast.success(t("managerAppBuilder.desktopBranding.iconUploaded"));
      } else {
        setLocalPreviews((prev) => {
          const next = { ...prev };
          delete next[DESKTOP_ICON_SLOT];
          return next;
        });
        toast.error(errorMessage(resp.data as Envelope, t("managerAppBuilder.desktopBranding.iconUploadFailed", { message: "" })));
      }
    } catch (error) {
      setLocalPreviews((prev) => {
        const next = { ...prev };
        delete next[DESKTOP_ICON_SLOT];
        return next;
      });
      toast.error(errorMessage(error, t("managerAppBuilder.desktopBranding.iconUploadFailed", { message: "" })));
    } finally {
      setBusyAssetSlot(null);
    }
  };

  const handleClearDesktopIcon = async () => {
    setBusyAssetSlot(DESKTOP_ICON_SLOT);
    try {
      const resp = await api.api.v1AdminAppBuilderDesktopBrandingAssetDelete(DESKTOP_ICON_SLOT);
      if (resp.ok && resp.data?.code === 0) {
        const data = (resp.data.data as DesktopBranding | null) ?? null;
        setDesktopBranding(data);
        setLocalPreviews((prev) => {
          const next = { ...prev };
          delete next[DESKTOP_ICON_SLOT];
          return next;
        });
        toast.success(t("managerAppBuilder.desktopBranding.iconCleared"));
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("managerAppBuilder.desktopBranding.iconClearFailed", { message: "" })));
      }
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.desktopBranding.iconClearFailed", { message: "" })));
    } finally {
      setBusyAssetSlot(null);
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
    if (!selectedPlatform?.entry?.enabled) return;
    if (activeJob) {
      toast.error(t("managerAppBuilder.build.busy"));
      return;
    }
    const version = buildVersion.trim();
    if (!versionRe.test(version)) {
      toast.error(t("managerAppBuilder.branding.versionInvalid"));
      return;
    }
    const isDesktop = buildPlatform === "desktop-linux";
    const versionCode = isDesktop ? 0 : Number(buildVersionCode);
    if (!isDesktop && (!Number.isInteger(versionCode) || versionCode <= 0)) {
      toast.error(t("managerAppBuilder.branding.versionCodeInvalid"));
      return;
    }
    setStartingBuild(true);
    try {
      const resp = await api.api.v1AdminAppBuilderBuildsCreate({
        platform: buildPlatform,
        version,
        version_code: versionCode,
      });
      if (resp.ok && resp.data?.code === 0) {
        const job = resp.data.data as BuildJob;
        toast.success(t("managerAppBuilder.build.enqueued"));
        setExpandedJobId(job.id);
        if (!isDesktop) setBuildVersionCode(String(job.version_code + 1));
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

  const handleDownload = async (job: BuildJob, channel?: string) => {
    setBusyKey(`${job.id}:download:${channel ?? "default"}`);
    try {
      const resp = await api.api.v1AdminAppBuilderBuildsDownload(job.id, channel);
      if (!resp.ok) {
        toast.error(t("managerAppBuilder.build.downloadFailed"));
        return;
      }
      const blob = await (resp.data as unknown as Response).blob();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download =
        job.platform === "desktop-linux"
          ? `${job.target_version}.${channel ?? "appimage"}`
          : APK_NAME;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 10000);
    } catch (error) {
      toast.error(errorMessage(error, t("managerAppBuilder.build.downloadFailed")));
    } finally {
      setBusyKey(null);
    }
  };

  const handlePublish = async (job: BuildJob, channel?: string) => {
    setBusyKey(`${job.id}:publish:${channel ?? "default"}`);
    try {
      const resp = await api.api.v1AdminAppBuilderBuildsPublish(job.id, channel);
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
      setBusyKey(null);
    }
  };

  // Artifacts are only returned by the job detail endpoint, so they are fetched
  // once when a finished job is opened and cached for later expansions.
  const loadJobArtifacts = useCallback(async (jobId: string) => {
    if (jobArtifacts[jobId]) return;
    try {
      const resp = await api.api.v1AdminAppBuilderBuildsDetail(jobId);
      if (resp.ok && resp.data?.code === 0) {
        const detail = resp.data.data as BuildJob | undefined;
        setJobArtifacts((prev) => ({ ...prev, [jobId]: detail?.artifacts ?? [] }));
      }
    } catch {
      // The row keeps working without the artifact table.
    }
  }, [api, jobArtifacts]);

  useEffect(() => {
    jobs
      .filter((job) => job.platform === "desktop-linux" && job.status === "succeeded")
      .filter((job) => !(job.id in jobArtifacts))
      .forEach((job) => {
        void loadJobArtifacts(job.id);
      });
  }, [jobs, jobArtifacts, loadJobArtifacts]);

  const handleToggleJob = (job: BuildJob) => {
    const next = expandedJobId === job.id ? null : job.id;
    setExpandedJobId(next);
    if (next) void loadJobArtifacts(next);
  };

  const handleRefresh = () => {
    void loadBranding();
    void loadDesktopBranding();
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

  if (!status) {
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

  const assets = branding?.assets?.length ? branding.assets : status.asset_slots ?? [];
  const isDesktopSelected = buildPlatform === "desktop-linux";
  const buildUnavailable = selectedPlatform?.entry?.enabled !== true;
  const buildUnavailableReasons = buildUnavailable ? blockedReasons(selectedPlatform?.entry) : [];
  const desktopIconPreview = localPreviews[DESKTOP_ICON_SLOT];

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
      <input
        ref={desktopIconInputRef}
        type="file"
        accept="image/png"
        className="hidden"
        onChange={(event) => {
          void handleDesktopIconFile(event.target.files?.[0] ?? null);
        }}
      />

      <Card className="w-full shadow-none">
        <CardContent className="flex flex-wrap items-center gap-3 pt-6">
          <div className="flex items-center gap-2">
            <Boxes className="size-4" />
            <CardTitle className="text-lg">{t("managerAppBuilder.title")}</CardTitle>
          </div>
          <CardDescription className="flex-1 basis-64">{t("managerAppBuilder.subtitle")}</CardDescription>
          <Badge variant="secondary">{t("managerAppBuilder.enabled")}</Badge>
        </CardContent>
      </Card>

      <Card className="w-full shadow-none">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Boxes />
            {t("managerAppBuilder.platforms.title")}
          </CardTitle>
          <CardDescription>{t("managerAppBuilder.platforms.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {platformEntries.length === 0 ? (
            <p className="py-4 text-center text-sm text-muted-foreground">
              {t("managerAppBuilder.platforms.empty")}
            </p>
          ) : (
            platformEntries.map(({ id, entry }) => {
              const available = entry?.enabled === true;
              const reasons = available ? [] : blockedReasons(entry);
              return (
                <div
                  key={id}
                  className={
                    "flex flex-col gap-2 rounded-lg border p-3 " +
                    (available ? "" : "border-amber-500/40 bg-amber-500/5")
                  }
                >
                  <div className="flex flex-wrap items-center gap-2">
                    {platformIcon(id, "size-4")}
                    <span className="text-sm font-medium">
                      {t(PLATFORM_LABEL_KEYS[id])}
                    </span>
                    <Badge
                      variant={available ? "secondary" : "outline"}
                      className={
                        available
                          ? ""
                          : "border-amber-400/60 bg-amber-500/10 text-amber-900 dark:text-amber-200"
                      }
                    >
                      {available
                        ? t("managerAppBuilder.platforms.available")
                        : t("managerAppBuilder.platforms.unavailable")}
                    </Badge>
                    {entry?.workspace ? (
                      <span className="font-mono text-xs text-muted-foreground" title={entry.workspace}>
                        {entry.workspace}
                      </span>
                    ) : null}
                    <span className="ml-auto flex flex-wrap items-center gap-1">
                      {(entry?.channels ?? []).map((channel) => (
                        <Badge key={channel} variant="outline" className="capitalize">
                          {t(channelLabelKey(channel), channel)}
                        </Badge>
                      ))}
                    </span>
                  </div>
                  {!available ? (
                    <div className="flex flex-col gap-1">
                      {reasons.length > 0 ? (
                        <ul className="flex flex-col gap-0.5">
                          {reasons.map((reason, index) => (
                            <li
                              key={`${reason}-${index}`}
                              className="flex items-start gap-1.5 text-xs text-amber-900 dark:text-amber-200"
                            >
                              <TriangleAlert className="mt-0.5 size-3 shrink-0" />
                              <span>{reason}</span>
                            </li>
                          ))}
                        </ul>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              );
            })
          )}
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
            {!isDesktopSelected ? (
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
            ) : null}
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
            <Monitor />
            {t("managerAppBuilder.desktopBranding.title")}
          </CardTitle>
          <CardDescription>{t("managerAppBuilder.desktopBranding.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-builder-desktop-product">
                {t("managerAppBuilder.desktopBranding.productName")}
              </Label>
              <Input
                id="app-builder-desktop-product"
                value={desktopProductName}
                placeholder={t("managerAppBuilder.desktopBranding.productNamePlaceholder")}
                onChange={(event) => setDesktopProductName(event.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-builder-desktop-bundle">
                {t("managerAppBuilder.desktopBranding.bundleIdentifier")}
              </Label>
              <Input
                id="app-builder-desktop-bundle"
                value={desktopBundleIdentifier}
                placeholder="com.authyide.app"
                onChange={(event) => setDesktopBundleIdentifier(event.target.value)}
              />
              <span className="text-xs text-muted-foreground">
                {t("managerAppBuilder.desktopBranding.bundleIdentifierHint")}
              </span>
            </div>
            <div className="flex flex-col gap-2 sm:col-span-2">
              <Label htmlFor="app-builder-desktop-updater">
                {t("managerAppBuilder.desktopBranding.updaterEndpoint")}
              </Label>
              <Input
                id="app-builder-desktop-updater"
                value={desktopUpdaterEndpoint}
                placeholder={t("managerAppBuilder.desktopBranding.updaterEndpointPlaceholder")}
                onChange={(event) => setDesktopUpdaterEndpoint(event.target.value)}
              />
              <span className="text-xs text-muted-foreground">
                {t("managerAppBuilder.desktopBranding.updaterEndpointHint")}
              </span>
            </div>
          </div>

          <div className="rounded-lg border p-3">
            <div className="flex items-center gap-2">
              <ImageIcon className="size-4 text-muted-foreground" />
              <span className="text-sm font-medium">
                {t("managerAppBuilder.desktopBranding.iconTitle")}
              </span>
            </div>
            <span className="mt-1 block text-xs text-muted-foreground">
              {t("managerAppBuilder.desktopBranding.iconDescription")}
            </span>
            <div className="mt-3 flex items-center gap-3">
              <div className="flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-md border bg-muted">
                {desktopIconPreview ? (
                  <img
                    src={desktopIconPreview}
                    alt={DESKTOP_ICON_SLOT}
                    className="size-full object-contain"
                  />
                ) : (
                  <ImageIcon className="size-5 text-muted-foreground" />
                )}
              </div>
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                {desktopBranding?.icon_key ? (
                  <Badge variant="secondary">
                    {t("managerAppBuilder.desktopBranding.iconSet")}
                  </Badge>
                ) : (
                  <Badge variant="outline">
                    {t("managerAppBuilder.desktopBranding.iconDefault")}
                  </Badge>
                )}
                <span className="font-mono truncate text-xs text-muted-foreground">
                  {desktopBranding?.icon_key || "-"}
                </span>
              </div>
              <div className="flex gap-1.5">
                {desktopBranding?.icon_key ? (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={busyAssetSlot === DESKTOP_ICON_SLOT}
                    onClick={() => void handleClearDesktopIcon()}
                  >
                    {busyAssetSlot === DESKTOP_ICON_SLOT ? (
                      <Spinner className="size-3.5" />
                    ) : (
                      <Trash2 className="size-3.5" />
                    )}
                    {t("managerAppBuilder.desktopBranding.iconClear")}
                  </Button>
                ) : null}
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busyAssetSlot === DESKTOP_ICON_SLOT}
                  onClick={handlePickDesktopIcon}
                >
                  {busyAssetSlot === DESKTOP_ICON_SLOT ? (
                    <Spinner className="size-3.5" />
                  ) : (
                    <Upload className="size-3.5" />
                  )}
                  {desktopBranding?.icon_key
                    ? t("managerAppBuilder.desktopBranding.iconReplace")
                    : t("managerAppBuilder.desktopBranding.iconUpload")}
                </Button>
              </div>
            </div>
          </div>

          <div className="flex justify-end">
            <Button
              onClick={() => void handleSaveDesktopBranding()}
              disabled={savingDesktopBranding}
            >
              {savingDesktopBranding ? (
                <Spinner className="size-4" />
              ) : null}
              {savingDesktopBranding
                ? t("managerAppBuilder.desktopBranding.saving")
                : t("managerAppBuilder.desktopBranding.save")}
            </Button>
          </div>
        </CardContent>
      </Card>

      {!isDesktopSelected ? (
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
      ) : null}

      <Card className="w-full shadow-none">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Boxes />
            {t("managerAppBuilder.build.title")}
          </CardTitle>
          <CardDescription>{t("managerAppBuilder.build.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label>{t("managerAppBuilder.build.platform")}</Label>
            <ToggleGroup
              type="single"
              value={buildPlatform}
              onValueChange={(value) => {
                if (value) setBuildPlatform(value as PlatformId);
              }}
              className="w-fit"
            >
              <ToggleGroupItem
                value="android"
                disabled={platformEntries.find((entry) => entry.id === "android")?.entry?.enabled !== true}
              >
                <Smartphone className="size-3.5" />
                {t("managerAppBuilder.build.platformAndroid")}
              </ToggleGroupItem>
              <ToggleGroupItem
                value="desktop-linux"
                disabled={
                  platformEntries.find((entry) => entry.id === "desktop-linux")?.entry?.enabled !== true
                }
              >
                <Monitor className="size-3.5" />
                {t("managerAppBuilder.build.platformDesktopLinux")}
              </ToggleGroupItem>
            </ToggleGroup>
          </div>

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
            {!isDesktopSelected ? (
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
            ) : null}
            <div className="flex flex-col gap-2">
              <Button
                onClick={() => void handleStartBuild()}
                disabled={startingBuild || Boolean(activeJob) || buildUnavailable}
                className="w-full"
              >
                {startingBuild ? <Spinner className="size-4" /> : <Rocket className="size-4" />}
                {startingBuild
                  ? t("managerAppBuilder.build.starting")
                  : t("managerAppBuilder.build.start")}
              </Button>
              {buildUnavailable ? (
                <div className="flex flex-col gap-1">
                  {buildUnavailableReasons.length > 0 ? (
                    <ul className="flex flex-col gap-0.5">
                      {buildUnavailableReasons.map((reason, index) => (
                        <li
                          key={`${reason}-${index}`}
                          className="flex items-start gap-1.5 text-xs text-amber-900 dark:text-amber-200"
                        >
                          <TriangleAlert className="mt-0.5 size-3 shrink-0" />
                          <span>{reason}</span>
                        </li>
                      ))}
                    </ul>
                  ) : null}
                </div>
              ) : null}
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
                const isDesktopJob = job.platform === "desktop-linux";
                const steps = stepsOf(job.platform);
                const stepIndex = isActive && job.current_step ? steps.indexOf(job.current_step) : -1;
                const artifacts = isDesktopJob ? (jobArtifacts[job.id] ?? []) : [];
                const platformLabelKey =
                  (PLATFORM_LABEL_KEYS as Record<string, string>)[job.platform] ??
                  `managerAppBuilder.build.platform.${job.platform}`;
                const isDownloadBusy = (channel: string) =>
                  busyKey === `${job.id}:download:${channel}`;
                const isPublishBusy = (channel?: string) =>
                  busyKey === `${job.id}:publish:${channel ?? "default"}`;
                return (
                  <div key={job.id} className="rounded-lg border">
                    <div className="flex flex-wrap items-center gap-3 p-3">
                      <div className="flex flex-col gap-0.5">
                        <span className="flex items-center gap-1.5 text-sm font-medium">
                          {platformIcon(job.platform, "size-3.5")}
                          {job.target_version}
                          {!isDesktopJob ? (
                            <span className="ml-1 text-xs font-normal text-muted-foreground">
                              #{job.version_code}
                            </span>
                          ) : null}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {formatTime(job.enqueued_at)}
                        </span>
                      </div>
                      <Badge variant="outline" className="capitalize">
                        {t(platformLabelKey, job.platform)}
                      </Badge>
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
                          : job.status === "succeeded" && !isDesktopJob
                            ? `${formatDuration(job.duration_ms)} · ${formatBytes(job.artifact_size)}`
                            : formatDuration(job.duration_ms) !== "-"
                              ? formatDuration(job.duration_ms)
                              : ""}
                      </span>
                      <div className="ml-auto flex flex-wrap items-center gap-1.5">
                        {isActive ? (
                          <Button variant="outline" size="sm" onClick={() => handleToggleJob(job)}>
                            <Terminal className="size-3.5" />
                            {t("managerAppBuilder.build.log")}
                          </Button>
                        ) : null}
                        {job.status === "succeeded" && isDesktopJob && !job.published_at
                          ? artifacts
                              .filter((artifact) => artifact.channel === "appimage")
                              .map((artifact) => (
                                <Button
                                  key={`${job.id}-publish-${artifact.channel}`}
                                  size="sm"
                                  disabled={isPublishBusy(artifact.channel)}
                                  onClick={() => void handlePublish(job, artifact.channel)}
                                >
                                  {isPublishBusy(artifact.channel) ? (
                                    <Spinner className="size-3.5" />
                                  ) : (
                                    <Rocket className="size-3.5" />
                                  )}
                                  {t("managerAppBuilder.build.publish")}
                                </Button>
                              ))
                          : null}
                        {job.status === "succeeded" && isDesktopJob ? (
                          artifacts.map((artifact) => (
                            <Button
                              key={`${job.id}-download-${artifact.channel}`}
                              variant="outline"
                              size="sm"
                              disabled={isDownloadBusy(artifact.channel)}
                              onClick={() => void handleDownload(job, artifact.channel)}
                            >
                              {isDownloadBusy(artifact.channel) ? (
                                <Spinner className="size-3.5" />
                              ) : (
                                <Download className="size-3.5" />
                              )}
                              {t(channelLabelKey(artifact.channel), artifact.channel)}
                            </Button>
                          ))
                        ) : null}
                        {job.status === "succeeded" && !isDesktopJob && !job.published_at ? (
                          <Button
                            size="sm"
                            disabled={isPublishBusy()}
                            onClick={() => void handlePublish(job)}
                          >
                            {isPublishBusy() ? <Spinner className="size-3.5" /> : <Rocket className="size-3.5" />}
                            {t("managerAppBuilder.build.publish")}
                          </Button>
                        ) : null}
                        {job.status === "succeeded" && !isDesktopJob ? (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={isDownloadBusy("default")}
                            onClick={() => void handleDownload(job)}
                          >
                            {isDownloadBusy("default") ? (
                              <Spinner className="size-3.5" />
                            ) : (
                              <Download className="size-3.5" />
                            )}
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
                        <Progress value={stepIndex >= 0 ? ((stepIndex + 1) / Math.max(steps.length, 1)) * 100 : 0} />
                      </div>
                    ) : null}
                    {job.status === "succeeded" && isDesktopJob ? (
                      <div className="border-t">
                        {artifacts.length === 0 ? (
                          <span className="block px-3 py-2 text-xs text-muted-foreground">
                            {t("managerAppBuilder.build.artifactsEmpty")}
                          </span>
                        ) : (
                          artifacts.map((artifact) => (
                            <div
                              key={artifact.channel}
                              className="flex flex-wrap items-center gap-2 border-t px-3 py-2 first:border-t-0"
                            >
                              <Badge variant="outline" className="w-20 justify-center capitalize">
                                {t(channelLabelKey(artifact.channel), artifact.channel)}
                              </Badge>
                              <span
                                className="font-mono truncate text-xs text-muted-foreground"
                                title={artifact.file_name}
                              >
                                {artifact.file_name ?? "-"}
                              </span>
                              <span className="text-xs text-muted-foreground">
                                {formatBytes(artifact.size)}
                              </span>
                              <span className="text-xs text-muted-foreground">
                                {formatDuration(artifact.duration_ms)}
                              </span>
                              <span
                                className="ml-auto font-mono truncate text-xs text-muted-foreground"
                                title={artifact.sha256}
                              >
                                {artifact.sha256 ? shortSha(artifact.sha256) : "-"}
                              </span>
                            </div>
                          ))
                        )}
                      </div>
                    ) : null}
                    {expanded && currentJob ? <JobLog job={currentJob} /> : null}
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
                <Monitor className="size-4" />
                <span className="text-sm font-medium">
                  {t("managerAppBuilder.releases.desktop")}
                </span>
                <Badge variant={desktopRelease?.enabled ? "secondary" : "outline"}>
                  {desktopRelease?.enabled
                    ? t("managerAppBuilder.releases.enabled")
                    : t("managerAppBuilder.releases.disabled")}
                </Badge>
                {desktopUnsigned ? (
                  <Badge variant="destructive">
                    {t("managerAppBuilder.releases.unsignedWarning")}
                  </Badge>
                ) : null}
              </div>
              <span className="text-xs text-muted-foreground">
                {t("managerAppBuilder.releases.desktopDescription")}
              </span>
              <div className="flex flex-wrap items-center gap-2 pt-1">
                {desktopRelease?.manifest ? (
                  <>
                    <Badge variant="secondary">v{desktopRelease.manifest.version}</Badge>
                    {desktopRelease.manifest.pub_date ? (
                      <span className="text-xs text-muted-foreground">
                        {formatTime(desktopRelease.manifest.pub_date)}
                      </span>
                    ) : null}
                    <a
                      href={desktopRelease.manifest.url}
                      target="_blank"
                      rel="noreferrer"
                      className="truncate text-xs text-primary underline"
                    >
                      {desktopRelease.manifest.url}
                    </a>
                  </>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {t("managerAppBuilder.releases.notPublished")}
                  </span>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {desktopRelease?.manifest?.signature ? (
                  <span className="font-mono truncate text-xs text-muted-foreground">
                    {t("managerAppBuilder.releases.signature")}: {shortSha(desktopRelease.manifest.signature)}
                  </span>
                ) : (
                  <Badge variant="outline">
                    {t("managerAppBuilder.releases.noSignature")}
                  </Badge>
                )}
                {desktopRelease?.artifact_sha256 ? (
                  <span
                    className="font-mono truncate text-xs text-muted-foreground"
                    title={desktopRelease.artifact_sha256}
                  >
                    {t("managerAppBuilder.releases.artifact")}: {shortSha(desktopRelease.artifact_sha256)}
                  </span>
                ) : null}
              </div>
            </div>

            {!isDesktopSelected ? (
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
            ) : null}
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
