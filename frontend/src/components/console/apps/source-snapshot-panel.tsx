import { Archive, CloudUpload, Loader2, RotateCw, TriangleAlert } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Api, type DomainApp, type DomainSnapshot } from "@/api/Api"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { Separator } from "@/components/ui/separator"
import { packSessionWorkspace, type SessionArchiveProgress } from "@/utils/session-archive"
import { formatShort } from "./app-editor"

interface SourceSnapshotPanelProps {
  taskId: string
  vmId: string
}

const api = new Api()
const ARCHIVE_NAME = "workspace.tar.gz"

function formatBytes(bytes?: number): string {
  if (!bytes || bytes <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB"]
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(value >= 100 || unit === 0 ? 0 : 1)} ${units[unit]}`
}

function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === "object") {
    const body = error as { message?: string; data?: { message?: string; code?: number } }
    if (body.data?.message) return body.data.message
    if (body.message) return body.message
  }
  if (error instanceof Error && error.message) return error.message
  return fallback
}

function codeOf(resp: { data?: unknown }): number | undefined {
  const data = resp.data as { code?: number } | null | undefined
  return typeof data?.code === "number" ? data.code : undefined
}

export function SourceSnapshotPanel({ taskId, vmId }: SourceSnapshotPanelProps) {
  const { t } = useTranslation()
  const [current, setCurrent] = useState<DomainSnapshot | null>(null)
  const [boundApps, setBoundApps] = useState<DomainApp[]>([])
  const [loading, setLoading] = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [progress, setProgress] = useState<SessionArchiveProgress | null>(null)
  const [failure, setFailure] = useState("")

  const load = useCallback(async () => {
    if (!taskId) return
    setLoading(true)
    setFailure("")
    try {
      const resp = await api.api.v1UsersSnapshotsCurrentGet({ task_id: taskId })
      if (codeOf(resp) !== 0) {
        throw new Error(errorMessage(resp.data, t("consoleApps.sourceSnapshot.loadFailed")))
      }
      setCurrent(resp.data?.data ?? null)
    } catch (error) {
      setFailure(errorMessage(error, t("consoleApps.sourceSnapshot.loadFailed")))
    } finally {
      setLoading(false)
    }
  }, [t, taskId])

  useEffect(() => {
    void load()
  }, [load])

  // Requirement R4 AC6: show which apps already build from this snapshot, so the
  // user can see who is affected when the snapshot goes stale.
  const snapshotId = current?.id ?? ""

  useEffect(() => {
    let cancelled = false
    const stop = () => {
      cancelled = true
    }
    if (!snapshotId) {
      setBoundApps([])
      return stop
    }
    void api
      .api.v1UsersAppsList({ limit: 100 })
      .then((resp) => {
        if (cancelled) return
        const apps: DomainApp[] = resp.data?.data?.apps ?? []
        setBoundApps(apps.filter((app) => app.source_snapshot_id === snapshotId))
      })
      .catch(() => {
        if (!cancelled) setBoundApps([])
      })
    return stop
  }, [snapshotId])

  const sync = async () => {
    if (!taskId || !vmId || syncing) return
    setSyncing(true)
    setFailure("")
    setProgress({ phase: "scanning", seen: 0, total: 0 })
    // Set as soon as the row exists so an interrupted upload cannot leave an
    // "uploading" snapshot behind.
    let pendingId: string | undefined
    try {
      const archive = await packSessionWorkspace(api, vmId, "", setProgress)

      const presign = await api.api.v1UsersSnapshotsUploadCreate({
        task_id: taskId,
        archive: { file_count: archive.fileCount, size_bytes: archive.sizeBytes },
        file_name: ARCHIVE_NAME,
      })
      if (codeOf(presign) !== 0) {
        throw new Error(errorMessage(presign.data, t("consoleApps.sourceSnapshot.presignFailed")))
      }
      const payload = (presign.data?.data ?? {}) as { upload_url?: string; snapshot?: DomainSnapshot }
      if (!payload.upload_url || !payload.snapshot?.id) {
        throw new Error(t("consoleApps.sourceSnapshot.presignFailed"))
      }
      pendingId = payload.snapshot.id

      setProgress({ phase: "uploading", seen: 0, total: 1 })
      const put = await fetch(payload.upload_url, { method: "PUT", body: archive.blob })
      if (!put.ok) {
        throw new Error(t("consoleApps.sourceSnapshot.uploadFailed"))
      }

      setProgress({ phase: "verifying", seen: 0, total: 1 })
      const done = await api.api.v1UsersSnapshotsCompleteCreate(payload.snapshot.id, {
        sha256: archive.sha256,
      })
      if (codeOf(done) !== 0) {
        throw new Error(errorMessage(done.data, t("consoleApps.sourceSnapshot.completeFailed")))
      }

      toast.success(t("consoleApps.sourceSnapshot.synced"))
      await load()
    } catch (error) {
      const message = errorMessage(error, t("consoleApps.sourceSnapshot.syncFailed"))
      if (pendingId) {
        void api
          .api.v1UsersSnapshotsFailCreate(pendingId, { error: message })
          .catch(() => undefined)
      }
      setFailure(message)
      toast.error(message)
    } finally {
      setSyncing(false)
      setProgress(null)
    }
  }

  const progressValue = progress
    ? progress.phase === "scanning"
      ? 10
      : progress.phase === "packing"
        ? 10 + (progress.total > 0 ? (progress.seen / progress.total) * 55 : 0)
        : progress.phase === "hashing"
          ? 70
          : progress.phase === "uploading"
            ? 85
            : 95
    : 0

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Archive className="size-4" />
          {t("consoleApps.sourceSnapshot.title")}
        </CardTitle>
        <CardDescription>{t("consoleApps.sourceSnapshot.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0 text-sm">
            {current ? (
              <span className="font-medium">{t("consoleApps.sourceSnapshot.bound")}</span>
            ) : (
              <span className="text-muted-foreground">
                {t("consoleApps.sourceSnapshot.none")}
              </span>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {loading && !syncing ? (
              <Loader2 className="size-4 animate-spin text-muted-foreground" />
            ) : (
              <Button
                size="sm"
                variant="outline"
                disabled={syncing || !vmId}
                onClick={() => void sync()}
              >
                <RotateCw className="size-4" />
                {t("consoleApps.sourceSnapshot.refresh")}
              </Button>
            )}
            <Button size="sm" disabled={syncing || !vmId} onClick={() => void sync()}>
              {syncing ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <CloudUpload className="size-4" />
              )}
              {syncing
                ? t("consoleApps.sourceSnapshot.syncing")
                : t("consoleApps.sourceSnapshot.sync")}
            </Button>
          </div>
        </div>

        {!vmId ? (
          <p className="text-xs text-muted-foreground">
            {t("consoleApps.sourceSnapshot.vmUnavailable")}
          </p>
        ) : null}

        {syncing && progress ? (
          <div className="flex flex-col gap-2">
            <Progress value={progressValue} />
            <div className="text-xs text-muted-foreground">
              {progress.phase === "scanning"
                ? t("consoleApps.sourceSnapshot.phaseScan", { seen: progress.seen })
                : progress.phase === "packing"
                  ? t("consoleApps.sourceSnapshot.phasePack", {
                      seen: progress.seen,
                      total: progress.total,
                    })
                  : progress.phase === "hashing"
                    ? t("consoleApps.sourceSnapshot.phaseHash")
                    : progress.phase === "uploading"
                      ? t("consoleApps.sourceSnapshot.phaseUpload")
                      : t("consoleApps.sourceSnapshot.phaseVerify")}
            </div>
          </div>
        ) : null}

        {failure ? (
          <div className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/5 p-2.5 text-xs text-destructive">
            <TriangleAlert className="mt-0.5 size-3.5 shrink-0" />
            <span className="min-w-0 break-words">{failure}</span>
          </div>
        ) : null}

        {current ? (
          <div className="rounded-lg border bg-muted/30 p-3">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <span className="font-mono text-xs text-muted-foreground">
                {(current.sha256 ?? "").slice(0, 12)}
              </span>
              {current.stale ? (
                <Badge variant="secondary" className="border-amber-400/60 bg-amber-500/10 text-amber-900 dark:text-amber-200">
                  {t("consoleApps.sourceSnapshot.stale")}
                </Badge>
              ) : (
                <Badge variant="secondary">{t("consoleApps.sourceSnapshot.fresh")}</Badge>
              )}
            </div>
            <div className="grid grid-cols-3 gap-3 text-xs text-muted-foreground">
              <div className="flex flex-col gap-0.5">
                <span>{t("consoleApps.sourceSnapshot.files")}</span>
                <span className="font-medium text-foreground">{current.file_count ?? 0}</span>
              </div>
              <div className="flex flex-col gap-0.5">
                <span>{t("consoleApps.sourceSnapshot.size")}</span>
                <span className="font-medium text-foreground">
                  {formatBytes(current.size_bytes)}
                </span>
              </div>
              <div className="flex flex-col gap-0.5">
                <span>{t("consoleApps.sourceSnapshot.createdAt")}</span>
                <span className="font-medium text-foreground">
                  {formatShort(current.completed_at ?? current.created_at)}
                </span>
              </div>
            </div>
            {current.stale ? (
              <p className="mt-2 text-xs text-amber-900 dark:text-amber-200">
                {t("consoleApps.sourceSnapshot.staleHint")}
              </p>
            ) : null}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t("consoleApps.sourceSnapshot.hint")}
          </p>
        )}

        {current && boundApps.length > 0 ? (
          <div className="rounded-lg border bg-muted/30 p-3">
            <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
              <span className="text-sm font-medium">
                {t("consoleApps.sourceSnapshot.boundAppsTitle")}
              </span>
              <span className="font-mono text-xs text-muted-foreground">
                {boundApps.length}
              </span>
            </div>
            <ul className="flex flex-col gap-1">
              {boundApps.map((app) => (
                <li
                  key={app.id ?? app.package_id ?? app.name ?? ""}
                  className="flex items-baseline justify-between gap-2 text-xs"
                >
                  <span className="min-w-0 truncate text-foreground">
                    {app.name ?? app.package_id ?? ""}
                  </span>
                  <span className="shrink-0 font-mono text-muted-foreground">
                    {app.package_id ?? ""}
                  </span>
                </li>
              ))}
            </ul>
            {current.stale ? (
              <p className="mt-2 text-xs text-amber-900 dark:text-amber-200">
                {t("consoleApps.sourceSnapshot.boundAppsStale")}
              </p>
            ) : null}
          </div>
        ) : null}

        {current ? <Separator /> : null}

        <div className="flex flex-col gap-1 text-xs text-muted-foreground">
          <span>{t("consoleApps.sourceSnapshot.excludeTitle")}</span>
          <span>{t("consoleApps.sourceSnapshot.excludeHint")}</span>
        </div>
      </CardContent>
    </Card>
  )
}
