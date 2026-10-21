import { CirclePlus, Loader2 } from "lucide-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"
import type { DomainApp } from "@/api/Api"
import { apiRequest } from "@/utils/requestUtils"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldContent, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { PlatformIcon } from "./app-platform-icon"
import { APP_PLATFORMS, PLATFORM_LABEL_KEY, type AppPlatform } from "./app-platforms"

interface CreateAppDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: (app: DomainApp) => void
}

export function CreateAppDialog({ open, onOpenChange, onSuccess }: CreateAppDialogProps) {
  const { t } = useTranslation()
  const [name, setName] = useState("")
  const [packageId, setPackageId] = useState("")
  const [description, setDescription] = useState("")
  const [platforms, setPlatforms] = useState<AppPlatform[]>(["web"])
  const [busy, setBusy] = useState(false)

  const togglePlatform = (platform: AppPlatform) => {
    setPlatforms((current) => {
      const next = current.includes(platform)
        ? current.filter((item) => item !== platform)
        : [...current, platform]
      return next.length > 0 ? next : current
    })
  }

  const reset = () => {
    setName("")
    setPackageId("")
    setDescription("")
    setPlatforms(["web"])
  }

  const submit = () => {
    if (!name.trim() || !packageId.trim()) {
      return
    }
    if (platforms.length === 0) {
      return
    }
    setBusy(true)
    apiRequest(
      "v1UsersAppsCreate",
      {
        name: name.trim(),
        package_id: packageId.trim(),
        description: description.trim() || undefined,
        enabled_platforms: platforms,
      },
      [],
      (resp) => {
        const created = resp.data as DomainApp | undefined
        reset()
        onOpenChange(false)
        if (created) {
          onSuccess(created)
        }
      }
    )
      .catch(() => undefined)
      .finally(() => setBusy(false))
  }

  return (
    <Dialog open={open} onOpenChange={(value) => !value && reset()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("consoleApps.create.title")}</DialogTitle>
          <DialogDescription>
            {t("consoleApps.create.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <Field>
            <FieldLabel>{t("consoleApps.create.name")}</FieldLabel>
            <FieldContent>
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder={t("consoleApps.create.namePlaceholder")}
                autoFocus
              />
            </FieldContent>
          </Field>
          <Field>
            <FieldLabel>{t("consoleApps.create.packageId")}</FieldLabel>
            <FieldDescription>{t("consoleApps.create.packageIdHint")}</FieldDescription>
            <FieldContent>
              <Input
                value={packageId}
                onChange={(event) => setPackageId(event.target.value.toLowerCase())}
                placeholder="com.example.authyapp"
                className="font-mono"
              />
            </FieldContent>
          </Field>
          <Field>
            <FieldLabel>{t("consoleApps.create.descriptionLabel")}</FieldLabel>
            <FieldContent>
              <Textarea
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder={t("consoleApps.create.descriptionPlaceholder")}
                rows={3}
              />
            </FieldContent>
          </Field>
          <Field>
            <FieldLabel>{t("consoleApps.create.platforms")}</FieldLabel>
            <FieldDescription>{t("consoleApps.create.platformsHint")}</FieldDescription>
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
                  >
                    <Checkbox
                      checked={platforms.includes(platform)}
                      onCheckedChange={() => togglePlatform(platform)}
                    />
                    <PlatformIcon platform={platform} className="size-4 text-muted-foreground" />
                    <span className="text-sm">{t(PLATFORM_LABEL_KEY[platform])}</span>
                  </div>
                ))}
              </div>
            </FieldContent>
          </Field>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
            {t("consoleApps.actions.cancel")}
          </Button>
          <Button
            onClick={submit}
            disabled={busy || !name.trim() || !packageId.trim() || platforms.length === 0}
          >
            {busy ? <Loader2 className="size-4 animate-spin" /> : <CirclePlus className="size-4" />}
            {busy ? t("consoleApps.actions.creating") : t("consoleApps.actions.create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
