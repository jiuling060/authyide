import { Link, useLocation } from "react-router-dom"
import { AppWindow } from "lucide-react"
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { useTranslation } from "react-i18next"

export default function NavApps() {
  const { t } = useTranslation()
  const location = useLocation()
  const isActive = location.pathname.startsWith("/console/apps")

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton asChild isActive={isActive}>
          <Link to="/console/apps">
            <AppWindow />
            <span>{t("navApps.label")}</span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
