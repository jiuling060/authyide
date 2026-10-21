import { AlertTriangle, Box, Clock, PackageOpen, Pencil, Trash2, X } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import type { DomainApp } from "@/api/Api"
import { apiRequest } from "@/utils/requestUtils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { PlatformIcon } from "./app-platform-icon"
import { PLATFORM_LABEL_KEY, type AppPlatform } from "./app-platforms"

interface AppListProps {
  apps: DomainApp[]
  loading: boolean
  selectedId?: string
  onSelect: (app: DomainApp) => void
  onOpenCreate: () => void
  onDelete: (app: DomainApp) => void
}

export function AppList({ apps, loading, selectedId, onSelect, onOpenCreate, onDelete }: AppListProps) {
  const { t } = useTranslation()
  const [deleting, setDeleting] = useState<DomainApp | null>(null)
  const [busy, setBusy] = useState(false)

  const confirmDelete = () => {
    if (!deleting) {
      return
    }
    const id = deleting.id
    const target = deleting
    if (!id) {
      return
    }
    setBusy(true)
    apiRequest("v1UsersAppsDelete", {}, [id], () => {
      onDelete(target)
    })
      .catch(() => undefined)
      .finally(() => {
        setBusy(false)
        setDeleting(null)
      })
  }

  if (loading) {
    return <div className="text-sm text-muted-foreground p-6">{t("consoleApps.common.loading")}</div>
  }

  if (apps.length === 0) {
    return (
      <Card className="border-dashed">
        <CardContent className="flex flex-col items-center justify-center gap-3 py-16">
          <PackageOpen className="size-10 text-muted-foreground/60" />
          <div className="text-center text-sm text-muted-foreground">{t("consoleApps.list.empty")}</div>
          <Button onClick={onOpenCreate}>
            <Box className="size-4" />
            {t("consoleApps.actions.create")}
          </Button>
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
        {apps.map((app) => {
          const platforms = (app.enabled_platforms ?? []).filter(
            (value): value is AppPlatform => PLATFORM_LABEL_KEY[value as AppPlatform] !== undefined
          )
          return (
            <Card
              key={app.id}
              role="button"
              tabIndex={0}
              className={
                "cursor-pointer transition-colors hover:bg-muted/60 " +
                (app.id === selectedId ? "border-primary" : "")
              }
              onClick={() => app.id && onSelect(app)}
              onKeyDown={(event) => {
                if ((event.key === "Enter" || event.key === " ") && app.id) {
                  event.preventDefault()
                  onSelect(app)
                }
              }}
            >
              <CardContent className="flex items-start gap-4 pt-5">
                <div className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-muted">
                  <PlatformIcon platform={platforms[0]} className="size-6 text-muted-foreground" />
                </div>
                <div className="min-w-0 flex-1 space-y-1.5">
                  <div className="flex items-center gap-2">
                    <span className="truncate font-semibold">{app.name ?? "—"}</span>
                    {app.locked && (
                      <Badge variant="secondary" className="shrink-0">
                        <AlertTriangle className="size-3" />
                        {t("consoleApps.badge.locked")}
                      </Badge>
                    )}
                  </div>
                  <div className="truncate font-mono text-xs text-muted-foreground">{app.package_id ?? ""}</div>
                  <div className="flex flex-wrap items-center gap-1.5 pt-1">
                    {platforms.map((platform) => (
                      <Badge key={platform} variant="outline" className="font-normal">
                        {t(PLATFORM_LABEL_KEY[platform])}
                      </Badge>
                    ))}
                  </div>
                  <div className="flex items-center gap-3 pt-1.5 text-xs text-muted-foreground">
                    <span className="inline-flex items-center gap-1">
                      <Clock className="size-3" />
                      {formatDate(app.created_at)}
                    </span>
                  </div>
                </div>
                <div className="flex shrink-0 flex-col gap-1">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8"
                    title={t("consoleApps.actions.edit")}
                    aria-label={t("consoleApps.actions.edit")}
                    onClick={(event) => {
                      event.stopPropagation()
                      if (app.id) {
                        onSelect(app)
                      }
                    }}
                  >
                    <Pencil className="size-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8 hover:text-destructive"
                    title={t("consoleApps.actions.delete")}
                    aria-label={t("consoleApps.actions.delete")}
                    onClick={(event) => {
                      event.stopPropagation()
                      setDeleting(app)
                    }}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>

      <Dialog open={!!deleting} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("consoleApps.delete.title")}</DialogTitle>
            <DialogDescription>
              {t("consoleApps.delete.description", { name: deleting?.name ?? "" })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              <X className="size-4" />
              {t("consoleApps.actions.cancel")}
            </Button>
            <Button variant="destructive" disabled={busy} onClick={confirmDelete}>
              <Trash2 className="size-4" />
              {busy ? t("consoleApps.actions.deleting") : t("consoleApps.delete.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function formatDate(value?: string): string {
  if (!value) {
    return "—"
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toISOString().slice(0, 10)
}
