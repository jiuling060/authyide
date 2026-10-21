import { AlertTriangle, Loader2, Save, Smartphone } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import type { DomainApp } from "@/api/Api"
import { apiRequest } from "@/utils/requestUtils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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

  useEffect(() => {
    setName(app.name ?? "")
    setDescription(app.description ?? "")
    setPlatforms(
      (app.enabled_platforms ?? []).filter(
        (value): value is AppPlatform => PLATFORM_LABEL_KEY[value as AppPlatform] !== undefined
      )
    )
  }, [app.id])

  const locked = !!app.locked

  const save = () => {
    if (!app.id || !name.trim() || platforms.length === 0) {
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
          <div className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-muted">
            <PlatformIcon platform={platforms[0]} className="size-5 text-muted-foreground" />
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
            <Button onClick={save} disabled={saving}>
              {saving ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
              {saving ? t("consoleApps.actions.saving") : t("consoleApps.actions.save")}
            </Button>
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
