import { Monitor, Smartphone, SmartphoneCharging, TabletSmartphone } from "lucide-react"

/**
 * Device glyph for a target platform. Shared by the app cards and the build
 * job table so a platform always looks the same.
 */
export function PlatformIcon({ platform, className = "size-3.5" }: { platform?: string; className?: string }) {
  switch (platform) {
    case "android":
    case "ios":
      return <Smartphone className={className} />
    case "harmonyos":
    case "harmony":
      return <SmartphoneCharging className={className} />
    case "desktop":
      return <TabletSmartphone className={className} />
    default:
      return <Monitor className={className} />
  }
}
