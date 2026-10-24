import {
  Archive,
  AlertTriangle,
  ImagePlus,
  Loader2,
  Search,
  Save,
  Smartphone,
  Trash2,
  X,
} from "lucide-react"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import { toast } from "sonner"
import { useTranslation } from "react-i18next"
import { Api, type DomainApp, type DomainSnapshot } from "@/api/Api"
import { apiRequest } from "@/utils/requestUtils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldContent, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { Textarea } from "@/components/ui/textarea"
import { PlatformIcon } from "./app-platform-icon"
import { APP_PLATFORMS, PLATFORM_LABEL_KEY, type AppPlatform } from "./app-platforms"

interface AppsEditorProps {
  app: DomainApp
  lockedNotice: string
  onAppChange: (app: DomainApp) => void
}

const KNOWN_PLANS = ["free", "basic", "pro", "ultra"]
const MAX_ICON_BYTES = 16 * 1024 * 1024

function errorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === "object") {
    const body = error as { message?: string; data?: { message?: string } }
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

function sha256Short(value?: string): string {
  return (value ?? "").slice(0, 12)
}

const api = new Api()

export function AppsEditor({ app, lockedNotice, onAppChange }: AppsEditorProps) {
  const { t } = useTranslation()
  const [saving, setSaving] = useState(false)

  const [name, setName] = useState(app.name ?? "")
  const [description, setDescription] = useState(app.description ?? "")
  const [platforms, setPlatforms] = useState<AppPlatform[]>(
    (app.enabled_platforms ?? []).filter(
      (value): value is AppPlatform => PLATFORM_LABEL_KEY[value as AppPlatform] !== undefined
    )
  )

  const [iconUrl, setIconUrl] = useState<string | null>(null)
  const [uploading, setUploading] = useState(false)
  const [removing, setRemoving] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  // Object URLs are revoked on replacement, on app switch and on unmount.
  const iconUrlRef = useRef<string | null>(null)

  useEffect(() => {
    setName(app.name ?? "")
    setDescription(app.description ?? "")
    setPlatforms(
      (app.enabled_platforms ?? []).filter(
        (value): value is AppPlatform => PLATFORM_LABEL_KEY[value as AppPlatform] !== undefined
      )
    )
  }, [app.id])

  const revokeIconUrl = useCallback(() => {
    if (iconUrlRef.current) {
      URL.revokeObjectURL(iconUrlRef.current)
      iconUrlRef.current = null
    }
    setIconUrl(null)
  }, [])

  useEffect(() => {
    revokeIconUrl()
  }, [app.id, revokeIconUrl])

  useEffect(() => () => revokeIconUrl(), [revokeIconUrl])

  const [snapshots, setSnapshots] = useState<DomainSnapshot[]>([])
  const [snapshotLoading, setSnapshotLoading] = useState(false)
  const [snapshotPickerOpen, setSnapshotPickerOpen] = useState(false)
  const [snapshotFilter, setSnapshotFilter] = useState("")
  const [binding, setBinding] = useState(false)
  const [boundSnapshot, setBoundSnapshot] = useState<DomainSnapshot | null>(null)

  const boundId = app.source_snapshot_id

  const loadBoundSnapshot = useCallback(async () => {
    if (!boundId) {
      setBoundSnapshot(null)
      return
    }
    setSnapshotLoading(true)
    try {
      const resp = await api.api.v1UsersSnapshotsGet(boundId)
      if (codeOf(resp) !== 0) {
        setBoundSnapshot(null)
        return
      }
      const snapshot = (resp.data?.data ?? {}) as DomainSnapshot
      setBoundSnapshot(snapshot.id ? snapshot : null)
    } catch {
      setBoundSnapshot(null)
    } finally {
      setSnapshotLoading(false)
    }
  }, [boundId])

  useEffect(() => {
    void loadBoundSnapshot()
  }, [loadBoundSnapshot])

  const loadSnapshots = useCallback(async () => {
    setSnapshotFilter("")
    setSnapshotLoading(true)
    try {
      const resp = await api.api.v1UsersSnapshotsList({ limit: 100 })
      if (codeOf(resp) !== 0) {
        toast.error(errorMessage(resp.data, t("consoleApps.editor.sourceSnapshot.loadFailed")))
        setSnapshots([])
        return
      }
      setSnapshots(Array.isArray(resp.data?.data) ? (resp.data.data as DomainSnapshot[]) : [])
    } catch (error) {
      setSnapshots([])
      toast.error(errorMessage(error, t("consoleApps.editor.sourceSnapshot.loadFailed")))
    } finally {
      setSnapshotLoading(false)
    }
  }, [t])

  const showPicker = () => {
    setSnapshotPickerOpen(true)
    void loadSnapshots()
  }

  const bindSnapshot = async (snapshot: DomainSnapshot) => {
    if (!app.id || !snapshot.id || binding) return
    setBinding(true)
    try {
      const resp = await api.api.v1UsersAppsSourceUpdate(app.id, {
        source_snapshot_id: snapshot.id,
      })
      if (codeOf(resp) !== 0) {
        throw new Error(errorMessage(resp.data, t("consoleApps.editor.sourceSnapshot.bindFailed")))
      }
      const updated = (resp.data?.data ?? {}) as DomainApp
      if (updated.id) {
        onAppChange(updated)
      }
      toast.success(t("consoleApps.editor.sourceSnapshot.bound"))
      setSnapshotPickerOpen(false)
    } catch (error) {
      toast.error(errorMessage(error, t("consoleApps.editor.sourceSnapshot.bindFailed")))
    } finally {
      setBinding(false)
    }
  }

  const unbindSnapshot = async () => {
    if (!app.id || binding) return
    setBinding(true)
    try {
      const resp = await api.api.v1UsersAppsSourceUpdate(app.id, {})
      if (codeOf(resp) !== 0) {
        throw new Error(errorMessage(resp.data, t("consoleApps.editor.sourceSnapshot.unbindFailed")))
      }
      const updated = (resp.data?.data ?? {}) as DomainApp
      if (updated.id) {
        onAppChange(updated)
      }
      toast.success(t("consoleApps.editor.sourceSnapshot.unbound"))
    } catch (error) {
      toast.error(errorMessage(error, t("consoleApps.editor.sourceSnapshot.unbindFailed")))
    } finally {
      setBinding(false)
    }
  }

  const filteredSnapshots = useMemo(() => {
    const needle = snapshotFilter.trim().toLowerCase()
    if (!needle) return snapshots
    return snapshots.filter((snapshot) =>
      (snapshot.task_title ?? "").toLowerCase().includes(needle),
    )
  }, [snapshots, snapshotFilter])

  const snapshotStale = !!boundSnapshot?.stale

  const loadIcon = useCallback(async () => {
    if (!app.id || !app.icon_key) {
      revokeIconUrl()
      return
    }
    try {
      const resp = await api.api.v1UsersAppsIconDownload(app.id)
      if (!resp.ok) {
        revokeIconUrl()
        return
      }
      const blob = await (resp.data as unknown as Response).blob()
      revokeIconUrl()
      const url = URL.createObjectURL(blob)
      iconUrlRef.current = url
      setIconUrl(url)
    } catch {
      revokeIconUrl()
    }
  }, [app.id, app.icon_key, revokeIconUrl])

  useEffect(() => {
    void loadIcon()
  }, [loadIcon])

  const uploadIcon = async (file: File) => {
    if (!app.id || file.size > MAX_ICON_BYTES || !file.type.startsWith("image/png")) {
      toast.error(t("consoleApps.editor.iconHint"))
      return
    }
    setUploading(true)
    try {
      const resp = await api.api.v1UsersAppsIconUpload(app.id, file)
      if (codeOf(resp) !== 0) {
        throw new Error(errorMessage(resp.data, t("consoleApps.editor.iconHint")))
      }
      toast.success(t("consoleApps.editor.iconUploaded"))
      onAppChange({ ...app, icon_key: app.id })
    } catch (error) {
      toast.error(errorMessage(error, t("consoleApps.editor.iconUploadFailed", { message: "" })))
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ""
    }
  }

  const removeIcon = async () => {
    if (!app.id) return
    setRemoving(true)
    try {
      const resp = await api.api.v1UsersAppsIconDelete(app.id)
      if (codeOf(resp) !== 0) {
        throw new Error(errorMessage(resp.data, t("consoleApps.editor.iconHint")))
      }
      toast.success(t("consoleApps.editor.iconRemoved"))
      onAppChange({ ...app, icon_key: undefined })
    } catch (error) {
      toast.error(errorMessage(error, t("consoleApps.editor.iconRemoveFailed", { message: "" })))
    } finally {
      setRemoving(false)
    }
  }

  const locked = !!app.locked

  const save = () => {
    if (!app.id || !name.trim() || platforms.length === 0 || !!boundSnapshot?.stale) {
      return
    }
    const req: Record<string, unknown> = {
      name: name.trim(),
      description: description.trim() || undefined,
      enabled_platforms: platforms,
    }
    setSaving(true)
    apiRequest("v1UsersAppsUpdate", req, [app.id], (resp) => {
      const updated = resp.data as DomainApp | undefined
      if (updated) {
        onAppChange(updated)
      }
    })
      .catch(() => undefined)
      .finally(() => setSaving(false))
  }

  const togglePlatform = (platform: AppPlatform) => {
    setPlatforms((current) => {
      const next = current.includes(platform)
        ? current.filter((item) => item !== platform)
        : [...current, platform]
      return next.length > 0 ? next : current
    })
  }

  const disabledStyle = { pointerEvents: "none" as const }

  return (
    <div className="flex h-full flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-muted">
            {iconUrl ? (
              <img src={iconUrl} alt="" className="size-full object-cover" />
            ) : (
              <PlatformIcon platform={platforms[0]} className="size-5 text-muted-foreground" />
            )}
          </div>
          <div className="min-w-0 space-y-0.5">
            <div className="flex items-center gap-2">
              <span className="truncate text-lg font-semibold">{name || app.name || "—"}</span>
              {locked && (
                <Badge variant="secondary" className="shrink-0">
                  <AlertTriangle className="size-3" />
                  {t("consoleApps.badge.locked")}
                </Badge>
              )}
            </div>
            <div className="truncate font-mono text-xs text-muted-foreground">{app.package_id ?? ""}</div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {locked ? (
            <div className="flex items-center gap-2 rounded-lg border border-amber-300/70 bg-amber-50 px-3 py-1.5 text-xs text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-200">
              <AlertTriangle className="size-3.5 shrink-0" />
              <span>{lockedNotice}</span>
            </div>
          ) : (
            <>
              {snapshotStale ? (
                <div className="flex items-center gap-2 rounded-lg border border-amber-300/70 bg-amber-50 px-3 py-1.5 text-xs text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-200">
                  <AlertTriangle className="size-3.5 shrink-0" />
                  <span>{t("consoleApps.editor.sourceSnapshot.staleSaveHint")}</span>
                </div>
              ) : null}
              <Button onClick={save} disabled={saving || snapshotStale}>
                {saving ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
                {saving ? t("consoleApps.actions.saving") : t("consoleApps.actions.save")}
              </Button>
            </>
          )}
        </div>
      </div>

      <Card className="mx-auto max-w-3xl">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Smartphone className="size-4" />
            {t("consoleApps.editor.settingsTitle")}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div style={locked ? disabledStyle : undefined} className="flex flex-col gap-4">
            <Field>
              <FieldLabel>{t("consoleApps.editor.icon")}</FieldLabel>
              <FieldDescription>{t("consoleApps.editor.iconHint")}</FieldDescription>
              <FieldContent>
                <div className="flex items-center gap-3">
                  <div
                    className={
                      "flex size-14 shrink-0 items-center justify-center overflow-hidden rounded-lg border " +
                      (iconUrl ? "bg-muted" : "bg-muted/40")
                    }
                  >
                    {iconUrl ? (
                      <img src={iconUrl} alt="" className="size-full object-cover" />
                    ) : (
                      <Smartphone className="size-5 text-muted-foreground" />
                    )}
                  </div>
                  <div className="flex min-w-0 flex-col gap-1.5">
                    <span className="truncate text-xs text-muted-foreground">
                      {iconUrl ? t("consoleApps.editor.icon") : t("consoleApps.editor.iconDefault")}
                    </span>
                    <div className="flex items-center gap-2">
                      <input
                        ref={fileInputRef}
                        type="file"
                        accept="image/png"
                        className="hidden"
                        onChange={(event) => {
                          const file = event.target.files?.[0]
                          if (file) void uploadIcon(file)
                        }}
                      />
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={locked || uploading}
                        onClick={() => fileInputRef.current?.click()}
                      >
                        {uploading ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : (
                          <ImagePlus className="size-4" />
                        )}
                        {uploading ? t("consoleApps.editor.iconChanging") : t("consoleApps.editor.iconChoose")}
                      </Button>
                      {iconUrl && !locked && (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          disabled={removing}
                          onClick={() => void removeIcon()}
                        >
                          {removing ? (
                            <Loader2 className="size-4 animate-spin" />
                          ) : (
                            <Trash2 className="size-4" />
                          )}
                          {removing
                            ? t("consoleApps.editor.iconRemoving")
                            : t("consoleApps.editor.iconRemove")}
                        </Button>
                      )}
                    </div>
                  </div>
                </div>
              </FieldContent>
            </Field>
            <Field>
              <FieldLabel>{t("consoleApps.editor.name")}</FieldLabel>
              <FieldContent>
                <Input
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  disabled={locked}
                />
              </FieldContent>
            </Field>
            <Field>
              <FieldLabel>{t("consoleApps.editor.packageId")}</FieldLabel>
              <FieldDescription>{t("consoleApps.editor.packageIdHint")}</FieldDescription>
              <FieldContent>
                <Input value={app.package_id ?? ""} disabled className="font-mono" />
              </FieldContent>
            </Field>
            <Field>
              <FieldLabel>{t("consoleApps.editor.description")}</FieldLabel>
              <FieldContent>
                <Textarea
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  rows={3}
                  disabled={locked}
                />
              </FieldContent>
            </Field>
            <Field>
              <FieldLabel>{t("consoleApps.editor.platforms")}</FieldLabel>
              <FieldContent>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                  {APP_PLATFORMS.map((platform) => (
                    <div
                      key={platform}
                      className={
                        "flex items-center gap-2 rounded-lg border px-2.5 py-2 " +
                        (platforms.includes(platform)
                          ? "border-primary/60 bg-primary/5"
                          : "border-border bg-muted/30")
                      }
                      style={locked ? disabledStyle : undefined}
                    >
                      <Checkbox
                        checked={platforms.includes(platform)}
                        onCheckedChange={() => togglePlatform(platform)}
                        disabled={locked}
                      />
                      <PlatformIcon platform={platform} className="size-4 text-muted-foreground" />
                      <span className="text-sm">{t(PLATFORM_LABEL_KEY[platform])}</span>
                    </div>
                  ))}
                </div>
              </FieldContent>
            </Field>
            <Field>
              <FieldLabel>{t("consoleApps.editor.sourceSnapshot.label")}</FieldLabel>
              <FieldDescription>
                {t("consoleApps.editor.sourceSnapshot.description")}
              </FieldDescription>
              <FieldContent>
                <div className="flex flex-col gap-3">
                  {boundSnapshot?.id ? (
                    <div className="rounded-lg border bg-muted/30 p-3">
                      <div className="flex flex-wrap items-start justify-between gap-2">
                        <div className="flex min-w-0 flex-col gap-1">
                          <span className="flex items-center gap-2">
                            <Smartphone className="size-3.5 shrink-0 text-muted-foreground" />
                            <span className="truncate text-sm font-medium">
                              {boundSnapshot.task_title || t("consoleApps.editor.sourceSnapshot.untitled")}
                            </span>
                            {boundSnapshot.stale ? (
                              <Badge variant="secondary" className="border-amber-400/60 bg-amber-500/10 text-amber-900 dark:text-amber-200">
                                {t("consoleApps.editor.sourceSnapshot.stale")}
                              </Badge>
                            ) : (
                              <Badge variant="secondary">
                                {t("consoleApps.editor.sourceSnapshot.fresh")}
                              </Badge>
                            )}
                          </span>
                          <span className="font-mono text-xs text-muted-foreground">
                            {sha256Short(boundSnapshot.sha256)}
                          </span>
                        </div>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={locked || binding}
                          onClick={() => void unbindSnapshot()}
                        >
                          <X className="size-4" />
                          {t("consoleApps.editor.sourceSnapshot.unbind")}
                        </Button>
                      </div>
                      <div className="mt-3 grid grid-cols-3 gap-3 text-xs text-muted-foreground">
                        <div className="flex flex-col gap-0.5">
                          <span>{t("consoleApps.editor.sourceSnapshot.files")}</span>
                          <span className="font-medium text-foreground">
                            {boundSnapshot.file_count ?? 0}
                          </span>
                        </div>
                        <div className="flex flex-col gap-0.5">
                          <span>{t("consoleApps.editor.sourceSnapshot.size")}</span>
                          <span className="font-medium text-foreground">
                            {formatBytes(boundSnapshot.size_bytes)}
                          </span>
                        </div>
                        <div className="flex flex-col gap-0.5">
                          <span>{t("consoleApps.editor.sourceSnapshot.createdAt")}</span>
                          <span className="font-medium text-foreground">
                            {formatShort(boundSnapshot.completed_at ?? boundSnapshot.created_at)}
                          </span>
                        </div>
                      </div>
                      {boundSnapshot.stale ? (
                        <p className="mt-2 text-xs text-amber-900 dark:text-amber-200">
                          {t("consoleApps.editor.sourceSnapshot.staleHint")}
                        </p>
                      ) : null}
                    </div>
                  ) : (
                    <div className="flex flex-col gap-2">
                      <p className="text-xs text-muted-foreground">
                        {t("consoleApps.editor.sourceSnapshot.templateHint")}
                      </p>
                      <div className="flex items-center gap-2">
                        <Dialog open={snapshotPickerOpen} onOpenChange={setSnapshotPickerOpen}>
                          <DialogTrigger asChild>
                            <Button
                              size="sm"
                              disabled={locked || binding}
                              onClick={showPicker}
                            >
                              <Archive className="size-4" />
                              {t("consoleApps.editor.sourceSnapshot.choose")}
                            </Button>
                          </DialogTrigger>
                          <DialogContent className="max-w-lg">
                            <DialogHeader>
                              <DialogTitle>
                                {t("consoleApps.editor.sourceSnapshot.pickTitle")}
                              </DialogTitle>
                              <DialogDescription>
                                {t("consoleApps.editor.sourceSnapshot.pickDescription")}
                              </DialogDescription>
                            </DialogHeader>
                            <div className="flex flex-col gap-3">
                              <div className="relative">
                                <Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" />
                                <Input
                                  className="pl-8"
                                  value={snapshotFilter}
                                  onChange={(event) => setSnapshotFilter(event.target.value)}
                                  placeholder={t("consoleApps.editor.sourceSnapshot.filter")}
                                />
                              </div>
                              <div className="flex max-h-72 flex-col gap-2 overflow-y-auto">
                                {filteredSnapshots.length === 0 ? (
                                  <p className="py-6 text-center text-xs text-muted-foreground">
                                    {snapshotLoading
                                      ? t("consoleApps.editor.sourceSnapshot.loading")
                                      : t("consoleApps.editor.sourceSnapshot.empty")}
                                  </p>
                                ) : (
                                  filteredSnapshots.map((snapshot) => (
                                    <button
                                      key={snapshot.id}
                                      type="button"
                                      disabled={binding || snapshot.id === boundSnapshot?.id}
                                      onClick={() => void bindSnapshot(snapshot)}
                                      className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/30 p-2.5 text-left transition-colors hover:border-primary/50 hover:bg-primary/5 disabled:cursor-not-allowed disabled:opacity-60"
                                    >
                                      <span className="flex min-w-0 flex-col gap-0.5">
                                        <span className="flex items-center gap-2">
                                          <span className="truncate text-sm font-medium">
                                            {snapshot.task_title || t("consoleApps.editor.sourceSnapshot.untitled")}
                                          </span>
                                          {snapshot.stale ? (
                                            <Badge
                                              variant="secondary"
                                              className="border-amber-400/60 bg-amber-500/10 text-amber-900 dark:text-amber-200"
                                            >
                                              {t("consoleApps.editor.sourceSnapshot.stale")}
                                            </Badge>
                                          ) : null}
                                        </span>
                                        <span className="text-xs text-muted-foreground">
                                          {formatShort(snapshot.completed_at ?? snapshot.created_at)}
                                          {" · "}
                                          {snapshot.file_count ?? 0} {t("consoleApps.editor.sourceSnapshot.files")}
                                          {" · "}
                                          {formatBytes(snapshot.size_bytes)}
                                        </span>
                                      </span>
                                      <span className="font-mono text-xs text-muted-foreground">
                                        {sha256Short(snapshot.sha256)}
                                      </span>
                                    </button>
                                  ))
                                )}
                              </div>
                            </div>
                            <DialogFooter>
                              <Button variant="outline" onClick={() => setSnapshotPickerOpen(false)}>
                                {t("consoleApps.actions.cancel")}
                              </Button>
                            </DialogFooter>
                          </DialogContent>
                        </Dialog>
                      </div>
                    </div>
                  )}
                </div>
              </FieldContent>
            </Field>
          </div>

          <Separator />

          <div className="grid grid-cols-2 gap-3 text-xs text-muted-foreground sm:grid-cols-3">
            <div className="flex flex-col gap-1">
              <span>{t("consoleApps.editor.status")}</span>
              <span className="font-medium text-foreground">{t("consoleApps.status.active")}</span>
            </div>
            <div className="flex flex-col gap-1">
              <span>{t("consoleApps.editor.tier")}</span>
              <span className="font-medium text-foreground">{t(app.tier && KNOWN_PLANS.includes(app.tier) ? `consoleApps.plans.${app.tier}` : "consoleApps.plans.free")}</span>
            </div>
            <div className="flex flex-col gap-1">
              <span>{t("consoleApps.editor.createdAt")}</span>
              <span className="font-medium text-foreground">{formatShort(app.created_at)}</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

export function formatShort(value?: string): string {
  if (!value) {
    return "—"
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
