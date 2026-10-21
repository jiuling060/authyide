/**
 * Platform vocabulary shared by the app list, editor, settings and create
 * dialog. Kept as a plain module so it can be reused without pulling React in.
 *
 * The platform set must stay in sync with the backend's valid platforms.
 * Ad networks are not defined here; the backend serves them from
 * packer/ad_networks.yaml through the ad-networks endpoint.
 */

export const APP_PLATFORMS = ["web", "android", "ios", "harmonyos", "desktop"] as const

export type AppPlatform = (typeof APP_PLATFORMS)[number]

/** Every key is `consoleApps.platforms.<id>`. */
export const PLATFORM_LABEL_KEY: Record<AppPlatform, string> = {
  web: "consoleApps.platforms.web",
  android: "consoleApps.platforms.android",
  ios: "consoleApps.platforms.ios",
  harmonyos: "consoleApps.platforms.harmony",
  desktop: "consoleApps.platforms.desktop",
}

export function isAppPlatform(value: unknown): value is AppPlatform {
  return typeof value === "string" && (APP_PLATFORMS as readonly string[]).includes(value)
}
