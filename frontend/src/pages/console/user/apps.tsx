import { AppWindow, Box, CirclePlus } from "lucide-react"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import type { DomainApp } from "@/api/Api"
import { apiRequest } from "@/utils/requestUtils"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { AppList } from "@/components/console/apps/app-list"
import { AppsEditor } from "@/components/console/apps/app-editor"
import { AppBuildsPanel } from "@/components/console/apps/app-builds-panel"
import { CreateAppDialog } from "@/components/console/apps/create-app-dialog"

export default function AppsPage() {
  const { t } = useTranslation()
  const [apps, setApps] = useState<DomainApp[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedId, setSelectedId] = useState("")
  const [tab, setTab] = useState("list")
  const [createOpen, setCreateOpen] = useState(false)
  const [entitlement, setEntitlement] = useState<{ tier?: string; host_quota?: number }>({})

  const selected = apps.find((app) => app.id === selectedId)
  const tier = entitlement.tier ?? "free"
  const hostQuota = entitlement.host_quota ?? 1
  const knownPlan = ["free", "basic", "pro", "ultra"].includes(tier)
  const planLabel = t(knownPlan ? `consoleApps.plans.${tier}` : "consoleApps.plans.free")

  const load = () => {
    apiRequest("v1UsersAppsList", {}, [], (resp) => {
      const payload = (resp.data ?? {}) as { apps?: DomainApp[]; tier?: string; host_quota?: number }
      setApps(payload.apps ?? [])
      setEntitlement({ tier: payload.tier, host_quota: payload.host_quota })
    })
      .catch(() => undefined)
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
  }, [])

  const openEditor = (app: DomainApp) => {
    setSelectedId(app.id ?? "")
    setTab("editor")
  }

  return (
    <div className="flex h-full w-full flex-col">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <div className="flex size-9 items-center justify-center rounded-lg bg-primary/10">
            <AppWindow className="size-5 text-primary" />
          </div>
          <div className="space-y-0.5">
            <h1 className="text-lg font-semibold leading-tight">{t("consoleApps.title")}</h1>
            <p className="text-xs text-muted-foreground">{t("consoleApps.subtitle")}</p>
          </div>
        </div>
        <Button onClick={() => setCreateOpen(true)}>
          <CirclePlus className="size-4" />
          {t("consoleApps.actions.create")}
        </Button>
      </div>

      <div className="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="rounded-lg border bg-card p-3">
          <div className="text-xs text-muted-foreground">{t("consoleApps.quota.plan")}</div>
          <div className="mt-1 text-base font-semibold">{planLabel}</div>
        </div>
        <div className="rounded-lg border bg-card p-3">
          <div className="text-xs text-muted-foreground">{t("consoleApps.quota.hosts")}</div>
          <div className="mt-1 text-base font-semibold">
            {t("consoleApps.quota.hostsValue", { quota: hostQuota })}
          </div>
        </div>
        <div className="rounded-lg border bg-card p-3">
          <div className="text-xs text-muted-foreground">{t("consoleApps.quota.apps")}</div>
          <div className="mt-1 text-base font-semibold">{apps.length}</div>
        </div>
        <div className="rounded-lg border bg-card p-3">
          <div className="text-xs text-muted-foreground">{t("consoleApps.quota.locked")}</div>
          <div className="mt-1 text-base font-semibold">
            {apps.filter((app) => app.locked).length}
          </div>
        </div>
      </div>

      {tab !== "list" && selected ? (
        <Breadcrumb className="mb-3">
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbPage onClick={() => setTab("list")} className="cursor-pointer">
                {t("consoleApps.tabs.list")}
              </BreadcrumbPage>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage className="truncate">{selected.name}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      ) : null}

      <div className="flex-1 min-h-0">
        <Tabs value={tab} onValueChange={setTab} className="flex h-full flex-col">
          <div className="flex flex-row justify-between">
            <TabsList>
              <TabsTrigger value="list">{t("consoleApps.tabs.list")}</TabsTrigger>
              <TabsTrigger value="editor" disabled={!selected}>
                {t("consoleApps.tabs.editor")}
              </TabsTrigger>
              <TabsTrigger value="build" disabled={!selected}>
                {t("consoleApps.tabs.build")}
              </TabsTrigger>
            </TabsList>
            {tab === "editor" && selected && (
              <Button variant="outline" size="sm" onClick={() => setTab("list")}>
                <Box className="size-3.5" />
                {t("consoleApps.actions.backToList")}
              </Button>
            )}
          </div>

          <TabsContent value="list" className="mt-3 min-h-0 flex-1">
            {loading ? (
              <div className="flex items-center justify-center py-20">
                <Spinner />
              </div>
            ) : (
              <AppList
                apps={apps}
                loading={false}
                selectedId={selectedId}
                onSelect={openEditor}
                onOpenCreate={() => setCreateOpen(true)}
                onDelete={(removed) =>
                  setApps((current) => current.filter((app) => app.id !== removed.id))
                }
              />
            )}
          </TabsContent>

          <TabsContent value="editor" className="mt-3 min-h-0 flex-1">
            {selected ? (
              <AppsEditor
                app={selected}
                lockedNotice={t("consoleApps.lockedNotice")}
                onAppChange={(updated) =>
                  setApps((current) => current.map((app) => (app.id === updated.id ? updated : app)))
                }
              />
            ) : (
              <div className="flex items-center justify-center py-20 text-sm text-muted-foreground">
                {t("consoleApps.editor.empty")}
              </div>
            )}
          </TabsContent>

          <TabsContent value="build" className="mt-3 min-h-0 flex-1">
            {selected ? (
              <AppBuildsPanel key={selected.id} app={selected} onChanged={load} />
            ) : (
              <div className="flex items-center justify-center py-20 text-sm text-muted-foreground">
                {t("consoleApps.editor.empty")}
              </div>
            )}
          </TabsContent>
        </Tabs>
      </div>

      <CreateAppDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onSuccess={(created) => {
          load()
          if (created.id) {
            openEditor(created)
          }
        }}
      />
    </div>
  )
}
