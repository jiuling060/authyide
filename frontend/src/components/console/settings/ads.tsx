"use client"

import { AlertTriangle, Loader2, Save, Ticket } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import type { DomainAdNetworkOption, DomainApp } from "@/api/Api"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldContent, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { toast } from "sonner"
import { apiRequest } from "@/utils/requestUtils"

export default function Ads() {
  const { t } = useTranslation()

  const [apps, setApps] = useState<DomainApp[]>([])
  const [networks, setNetworks] = useState<DomainAdNetworkOption[]>([])
  const [selectedId, setSelectedId] = useState("")
  const [adNetwork, setAdNetwork] = useState("none")
  const [adAppId, setAdAppId] = useState("")
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const selected = apps.find((app) => app.id === selectedId)

  const load = () => {
    apiRequest("v1UsersAppsList", {}, [], (resp) => {
      setApps(resp.data?.apps ?? [])
    })
      .catch(() => undefined)
      .finally(() => setLoading(false))

    apiRequest("v1UsersAppsAdNetworksList", {}, [], (resp) => {
      setNetworks(resp.data ?? [])
    }).catch(() => undefined)
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const save = () => {
    if (!selectedId) {
      return
    }
    const req: Record<string, unknown> = {
      ad_network: adNetwork,
      ad_app_id: adAppId.trim() || undefined,
    }
    setSaving(true)
    apiRequest("v1UsersAppsUpdate", req, [selectedId], (resp) => {
      const updated = resp.data as DomainApp | undefined
      if (updated) {
        setApps((current) => current.map((app) => (app.id === updated.id ? updated : app)))
        toast.success(t("consoleSettings.ads.toast.saveSuccess"))
      }
    })
      .catch(() => toast.error(t("consoleSettings.ads.toast.saveFailed")))
      .finally(() => setSaving(false))
  }

  // Switching apps discards unsaved edits so app A's id can never land on app B.
  const selectApp = (id: string) => {
    setSelectedId(id)
    const app = apps.find((item) => item.id === id)
    if (app) {
      setAdNetwork(app.ad_network ?? "none")
      setAdAppId(app.ad_app_id ?? "")
    }
  }

  const locked = !!selected?.locked
  const disabledStyle = locked ? { pointerEvents: "none" as const } : undefined
  // Picking "no ads" discards the typed id, per requirement 1.4: the field is
  // meaningless while no network is attached, so it must not resurface later.
  const chooseNetwork = (id: string) => {
    setAdNetwork(id)
    if (id === "none") {
      setAdAppId("")
    }
  }

  // "none" is rendered from a localized label instead of the registry's
  // Chinese-only displayName, so English users see a translated option.
  const networkLabel = (id: string) =>
    id === "none"
      ? t("consoleSettings.ads.noAds")
      : networks.find((network) => network.id === id)?.display_name ?? id

  if (loading) {
    return (
      <Card>
        <CardContent className="flex items-center justify-center py-20">
          <Spinner />
        </CardContent>
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Ticket className="size-4" />
          {t("consoleSettings.ads.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="rounded-lg border border-dashed bg-muted/30 px-3 py-2.5 text-xs text-muted-foreground">
          {t("consoleSettings.ads.note")}
        </div>

        <Field>
          <FieldLabel>{t("consoleSettings.ads.selectApp")}</FieldLabel>
          <FieldContent>
            <Select value={selectedId} onValueChange={selectApp}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t("consoleSettings.ads.selectAppPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                {apps.length === 0 ? (
                  <SelectItem value="" disabled>
                    {t("consoleSettings.ads.noApps")}
                  </SelectItem>
                ) : (
                  apps.map((app) => (
                    <SelectItem key={app.id} value={app.id ?? ""}>
                      {app.name ?? ""}{" "}
                      <span className="font-mono text-xs text-muted-foreground">
                        ({app.package_id ?? ""})
                      </span>
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
          </FieldContent>
        </Field>

        {selected ? (
          <>
            {locked && (
              <div className="flex items-center gap-2 rounded-lg border border-amber-300/70 bg-amber-50 px-3 py-2 text-xs text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-200">
                <AlertTriangle className="size-3.5 shrink-0" />
                <span>{t("consoleSettings.ads.lockedNotice")}</span>
              </div>
            )}

            <div style={disabledStyle} className="flex flex-col gap-4">
              <Field>
                <FieldLabel>{t("consoleSettings.ads.network")}</FieldLabel>
                <FieldContent>
                  <Select value={adNetwork} onValueChange={chooseNetwork}>
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {networks.map((network) => (
                        <SelectItem key={network.id} value={network.id}>
                          {networkLabel(network.id)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FieldContent>
              </Field>

              {adNetwork !== "none" && (
                <Field>
                  <FieldLabel>{t("consoleSettings.ads.appId")}</FieldLabel>
                  <FieldDescription>
                    {t("consoleSettings.ads.appIdHint", { network: networkLabel(adNetwork) })}
                  </FieldDescription>
                  <FieldContent>
                    <Input
                      value={adAppId}
                      onChange={(event) => setAdAppId(event.target.value)}
                      placeholder="ca-app-pub-..."
                      className="font-mono"
                    />
                  </FieldContent>
                </Field>
              )}
            </div>

            <div className="flex items-center justify-between gap-2">
              <div className="text-xs text-muted-foreground">
                {t("consoleSettings.ads.packageId")}:{" "}
                <span className="font-mono font-medium text-foreground">
                  {selected.package_id ?? ""}
                </span>
              </div>
              <Button onClick={save} disabled={saving || locked}>
                {saving ? <Loader2 className="size-4 animate-spin" /> : <Save className="size-4" />}
                {saving ? t("consoleSettings.ads.saving") : t("consoleSettings.ads.save")}
              </Button>
            </div>
          </>
        ) : (
          <div className="rounded-lg border border-dashed px-3 py-8 text-center text-sm text-muted-foreground">
            {t("consoleSettings.ads.selectAppPlaceholder")}
          </div>
        )}
      </CardContent>
    </Card>
  )
}