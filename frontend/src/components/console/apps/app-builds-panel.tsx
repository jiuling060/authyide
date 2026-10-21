import { useCallback, useEffect, useRef, useState } from "react"
import { Download, Loader2, RefreshCw, Rocket, RotateCcw, Smartphone, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"

import { Api, type DomainApp, type DomainAppBuildJob } from "@/api/Api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"

const VERSION_RE = /^\d{6,8}$/
const POLL_INTERVAL_MS = 5000
const APK_NAME = "authyide.apk"

type Envelope = {
  code?: number
  message?: string
  data?: unknown
}

type AppBuildsPanelProps = {
  app: DomainApp
  onChanged?: () => void
}

function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === "object") {
    const body = error as { message?: string; data?: { message?: string } }
    if (body.data?.message) return body.data.message
    if (body.message) return body.message
  }
  if (error instanceof Error && error.message) return error.message
  return fallback
}

function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return "-"
  const units = ["B", "KB", "MB", "GB"]
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value >= 100 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`
}

function formatTime(value?: string): string {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const locale = navigator.language?.toLowerCase().startsWith("zh") ? "zh-CN" : "en-US"
  return date.toLocaleString(locale, { hour12: false }).replace(/\//g, "-")
}

function formatDuration(ms?: number): string {
  if (!ms || ms <= 0) return "-"
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  return `${minutes}m ${seconds % 60}s`
}

export function AppBuildsPanel({ app, onChanged }: AppBuildsPanelProps) {
  const { t } = useTranslation()
  const api = new Api()

  const [jobs, setJobs] = useState<DomainAppBuildJob[]>([])
  const [loading, setLoading] = useState(true)
  const [version, setVersion] = useState("")
  const [versionCode, setVersionCode] = useState("")
  const [enqueueing, setEnqueueing] = useState(false)
  const [busyJobId, setBusyJobId] = useState<string | null>(null)
  const [resetVersionCode, setResetVersionCode] = useState(false)
  const statusSeen = useRef<Map<string, string>>(new Map())
  // Kept in a ref so the polling callback stays stable across parent renders.
  const onChangedRef = useRef(onChanged)
  useEffect(() => {
    onChangedRef.current = onChanged
  }, [onChanged])

  const locked = !!app.locked
  const buildCount = app.build_count ?? 0
  const isFreeTier = (app.tier ?? "free") === "free"
  // Only the free tier caps builds per app; other tiers are capped per day.
  const remainingBuilds = isFreeTier ? Math.max(0, 1 - buildCount) : null
  const tierLabel = t(
    ["free", "basic", "pro", "ultra"].includes(app.tier ?? "")
      ? `consoleApps.plans.${app.tier}`
      : "consoleApps.plans.free"
  )
  const platforms = app.enabled_platforms ?? []
  const maxVersionCode = jobs.reduce((max, job) => Math.max(max, job.version_code), 0)

  const loadJobs = useCallback(async () => {
    const id = app.id
    if (!id) return
    try {
      const resp = await api.api.v1UsersAppsBuildsList(id)
      if (resp.ok && resp.data?.code === 0) {
        const next = (resp.data.data as DomainAppBuildJob[] | undefined) ?? []
        setJobs(next)
        // build_count only moves when a build succeeds, so pull the App list
        // again once a job transitions into a terminal state.
        for (const job of next) {
          if (!job.id) continue
          const previous = statusSeen.current.get(job.id)
          statusSeen.current.set(job.id, job.status)
          const terminal = job.status === "succeeded" || job.status === "failed"
          if (previous && previous !== job.status && terminal) onChangedRef.current?.()
        }
      }
    } catch {
      // Keep the previous list and retry on the next tick.
    } finally {
      setLoading(false)
    }
  }, [api, app.id])

  useEffect(() => {
    setJobs([])
    setLoading(true)
    setVersionCode("")
    setResetVersionCode(true)
    void loadJobs()
  }, [loadJobs])

  const activeJob = jobs.find((job) => job.status === "queued" || job.status === "running")

  useEffect(() => {
    if (!activeJob) return
    const timer = window.setInterval(() => void loadJobs(), POLL_INTERVAL_MS)
    return () => window.clearInterval(timer)
  }, [activeJob, loadJobs])

  useEffect(() => {
    if (!resetVersionCode) return
    setResetVersionCode(false)
    setVersionCode(maxVersionCode > 0 ? String(maxVersionCode + 1) : "")
  }, [resetVersionCode, maxVersionCode])

  const handleRetry = (job: DomainAppBuildJob) => {
    // Reuse the failed version and bump the code, since the server requires a
    // version_code higher than anything already built.
    if (job.target_version) setVersion(job.target_version)
    setVersionCode(String((job.version_code || 0) + 1))
    requestAnimationFrame(() => {
      document.getElementById("app-build-version")?.focus()
    })
  }

  const handleEnqueue = async () => {
    const id = app.id
    if (!id || enqueueing || activeJob) return
    const versionText = version.trim()
    if (!VERSION_RE.test(versionText)) {
      toast.error(t("consoleApps.builds.versionInvalid"))
      return
    }
    const code = Number(versionCode)
    const autoCode = !versionCode
    if (!autoCode && (!Number.isInteger(code) || code <= 0)) {
      toast.error(t("consoleApps.builds.versionCodeInvalid"))
      return
    }
    setEnqueueing(true)
    try {
      const resp = await api.api.v1UsersAppsBuildsCreate(id, {
        platform: "android",
        version: versionText,
        version_code: autoCode ? 0 : code,
      })
      if (resp.ok && resp.data?.code === 0) {
        const job = resp.data.data as DomainAppBuildJob | undefined
        toast.success(t("consoleApps.builds.enqueued"))
        setVersionCode(String((job?.version_code ?? code) + 1))
        void loadJobs()
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("consoleApps.builds.enqueueFailed", { message: "" })))
      }
    } catch (error) {
      toast.error(errorMessage(error, t("consoleApps.builds.enqueueFailed", { message: "" })))
    } finally {
      setEnqueueing(false)
    }
  }

  const handleDownload = async (job: DomainAppBuildJob) => {
    if (!job.id) return
    setBusyJobId(job.id)
    try {
      const resp = await api.api.v1UsersAppsBuildsDownload(job.id)
      if (!resp.ok) {
        toast.error(t("consoleApps.builds.downloadFailed"))
        return
      }
      const blob = await (resp.data as unknown as Response).blob()
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement("a")
      anchor.href = url
      anchor.download = APK_NAME
      document.body.appendChild(anchor)
      anchor.click()
      anchor.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 10000)
    } catch (error) {
      toast.error(errorMessage(error, t("consoleApps.builds.downloadFailed")))
    } finally {
      setBusyJobId(null)
    }
  }

  const handlePublish = async (job: DomainAppBuildJob) => {
    if (!job.id) return
    setBusyJobId(job.id)
    try {
      const resp = await api.api.v1UsersAppsBuildsPublish(job.id)
      if (resp.ok && resp.data?.code === 0) {
        toast.success(t("consoleApps.builds.published"))
        void loadJobs()
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("consoleApps.builds.publishFailed", { message: "" })))
      }
    } catch (error) {
      toast.error(errorMessage(error, t("consoleApps.builds.publishFailed", { message: "" })))
    } finally {
      setBusyJobId(null)
    }
  }

  const handleDelete = async (job: DomainAppBuildJob) => {
    if (!job.id) return
    setBusyJobId(job.id)
    try {
      const resp = await api.api.v1UsersAppsBuildsDelete(job.id)
      if (resp.ok && resp.data?.code === 0) {
        toast.success(t("consoleApps.builds.deleted"))
        void loadJobs()
      } else {
        toast.error(errorMessage(resp.data as Envelope, t("consoleApps.builds.deleteFailed", { message: "" })))
      }
    } catch (error) {
      toast.error(errorMessage(error, t("consoleApps.builds.deleteFailed", { message: "" })))
    } finally {
      setBusyJobId(null)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <Card className="w-full shadow-none">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Smartphone />
            {t("consoleApps.builds.title")}
          </CardTitle>
          <CardDescription>
            {t("consoleApps.builds.description", { packageId: app.package_id ?? "-" })}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div className="rounded-lg border p-3">
              <div className="text-xs text-muted-foreground">
                {t("consoleApps.builds.lastVersion")}
              </div>
              <div className="mt-1 truncate text-base font-semibold">
                {jobs.length > 0 ? jobs[0].target_version : "-"}
              </div>
            </div>
            <div className="rounded-lg border p-3">
              <div className="text-xs text-muted-foreground">
                {t("consoleApps.builds.tier")}
              </div>
              <div className="mt-1 truncate text-base font-semibold">
                {tierLabel}
              </div>
            </div>
            <div className="rounded-lg border p-3">
              <div className="text-xs text-muted-foreground">
                {t("consoleApps.builds.remaining")}
              </div>
              <div className="mt-1 truncate text-base font-semibold">
                {remainingBuilds === null
                  ? t("consoleApps.builds.unlimited")
                  : remainingBuilds}
              </div>
            </div>
            <div className="rounded-lg border p-3">
              <div className="text-xs text-muted-foreground">
                {t("consoleApps.builds.package")}
              </div>
              <div className="mt-1 truncate text-base font-semibold">
                {app.package_id ?? "-"}
              </div>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-xs text-muted-foreground">
              {t("consoleApps.builds.buildPlatforms")}
            </span>
            {platforms.length === 0 ? (
              <span className="text-xs text-muted-foreground">-</span>
            ) : (
              platforms.map((platform) => (
                <Badge key={platform} variant="outline" className="capitalize">
                  {platform}
                </Badge>
              ))
            )}
          </div>

          {locked ? (
            <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
              {t("consoleApps.builds.lockedNotice")}
            </div>
          ) : null}

          <div className="grid gap-4 sm:grid-cols-3">
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-build-version">{t("consoleApps.builds.version")}</Label>
              <Input
                id="app-build-version"
                value={version}
                placeholder="260608"
                disabled={locked || !!activeJob || enqueueing}
                onChange={(event) => setVersion(event.target.value)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="app-build-version-code">
                {t("consoleApps.builds.versionCode")}
              </Label>
              <Input
                id="app-build-version-code"
                type="number"
                min={1}
                value={versionCode}
                placeholder={String(maxVersionCode + 1)}
                disabled={locked || !!activeJob || enqueueing}
                onChange={(event) => setVersionCode(event.target.value)}
              />
            </div>
            <div className="flex items-end">
              <Button
                onClick={() => void handleEnqueue()}
                disabled={locked || enqueueing || !!activeJob}
                className="w-full"
              >
                {enqueueing ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : activeJob ? (
                  <Spinner className="size-4" />
                ) : (
                  <Rocket className="size-4" />
                )}
                {enqueueing
                  ? t("consoleApps.builds.starting")
                  : t("consoleApps.builds.start")}
              </Button>
            </div>
          </div>

          <Separator />

          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">{t("consoleApps.builds.history")}</h3>
            <Button variant="ghost" size="sm" onClick={() => void loadJobs()}>
              <RefreshCw className="size-4" />
              {t("consoleApps.actions.refresh")}
            </Button>
          </div>

          {loading ? (
            <div className="flex items-center justify-center py-10">
              <Spinner />
            </div>
          ) : jobs.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t("consoleApps.builds.emptyHistory")}
            </p>
          ) : (
            <div className="flex flex-col gap-2">
              {jobs.map((job) => {
                const busy = busyJobId === job.id
                const isActive = job.status === "queued" || job.status === "running"
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
                        {t(`consoleApps.builds.status.${job.status}`, job.status)}
                      </Badge>
                      <span className="text-xs text-muted-foreground">
                        {job.status === "succeeded"
                          ? `${formatDuration(job.duration_ms)} · ${formatBytes(job.artifact_size)}`
                          : isActive && job.current_step
                            ? t(`consoleApps.builds.steps.${job.current_step}`, job.current_step)
                            : formatDuration(job.duration_ms) !== "-"
                              ? formatDuration(job.duration_ms)
                              : ""}
                      </span>
                       {job.status === "succeeded" && job.artifact_sha256 ? (
                         <span
                           className="text-xs text-muted-foreground"
                           title={`${t("consoleApps.builds.sha256")}: ${job.artifact_sha256}`}
                         >
                           {t("consoleApps.builds.sha256")}:{" "}
                           <span className="font-mono">{job.artifact_sha256.slice(0, 12)}…</span>
                         </span>
                       ) : null}
                        {job.status === "failed" ? (
                          <div className="min-w-0 flex-1 space-y-1">
                            {job.error ? (
                              <span className="block truncate text-xs text-destructive" title={job.error}>
                                {job.error}
                              </span>
                            ) : null}
                            {job.log_tail ? (
                              <pre
                                className="max-h-24 overflow-auto whitespace-pre-wrap break-all rounded bg-muted/60 p-2 text-[11px] leading-snug text-muted-foreground"
                                title={t("consoleApps.builds.logTail")}
                              >
                                {t("consoleApps.builds.logTail")}
                                {job.log_tail}
                              </pre>
                            ) : null}
                          </div>
                        ) : null}
                      <div className="ml-auto flex flex-wrap items-center gap-1.5">
                        {isActive ? (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void loadJobs()}
                          >
                            <RefreshCw className="size-3.5" />
                            {t("consoleApps.actions.refresh")}
                          </Button>
                        ) : null}
                        {job.status === "failed" && !locked && !activeJob ? (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy}
                            onClick={() => handleRetry(job)}
                          >
                            <RotateCcw className="size-3.5" />
                            {t("consoleApps.builds.retry")}
                          </Button>
                        ) : null}
                        {job.status === "succeeded" ? (
                          <Button
                            size="sm"
                            disabled={busy}
                            onClick={() => void handlePublish(job)}
                          >
                            {busy ? <Spinner className="size-3.5" /> : <Rocket className="size-3.5" />}
                            {t("consoleApps.builds.publish")}
                          </Button>
                        ) : null}
                        {job.status === "succeeded" ? (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy}
                            onClick={() => void handleDownload(job)}
                          >
                            {busy ? <Spinner className="size-3.5" /> : <Download className="size-3.5" />}
                            {t("consoleApps.builds.download")}
                          </Button>
                        ) : null}
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={busy}
                          onClick={() => void handleDelete(job)}
                        >
                          {busy ? <Spinner className="size-3.5" /> : <Trash2 className="size-3.5" />}
                        </Button>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
