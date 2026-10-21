import { Link, useLocation } from "react-router-dom"
import { ShieldCheck } from "lucide-react"
import { ConstsUserRole } from "@/api/Api"
import { useCommonData } from "@/components/console/data-provider"
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { useTranslation } from "react-i18next"

export default function NavManager() {
  const { t } = useTranslation()
  const location = useLocation()
  const { user } = useCommonData()

  const isAdmin = user?.role === ConstsUserRole.UserRoleAdmin

  if (!isAdmin) {
    return null
  }

  const isActive = location.pathname.startsWith("/manager")

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton asChild isActive={isActive}>
          <Link to="/manager">
            <ShieldCheck />
            <span>{t("navManager.label")}</span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
