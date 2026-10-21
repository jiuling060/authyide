import { useAppRuntime } from "@/components/app-runtime-provider";
import { cn } from "@/lib/utils";
import { IconArrowRight, IconMenu2, IconPointFilled } from "@tabler/icons-react";
import React from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

const DOCS_LINK = "/docs";
const GITHUB_LINK = "https://github.com/jiuling060/AuthyIDE/";
const FORUM_LINK = "/";
const SHOWCASE_LINK = "/docs#showcase";
const SELF_HOSTING_PAGE_PATH = "/self-hosting";
const SUPPORT_LINK = "/docs#support";
const BUSINESS_LINK = "/docs#contact";

const resourceLinks = [
  { titleKey: "welcomeShell.footer.productDocs", href: DOCS_LINK },
  { titleKey: "welcomeShell.footer.forum", href: FORUM_LINK },
  { titleKey: "welcomeShell.nav.openSourceRepo", href: GITHUB_LINK },
];

const companyLinks = [
  { titleKey: "welcomeShell.footer.support", href: SUPPORT_LINK },
  { titleKey: "welcomeShell.footer.business", href: BUSINESS_LINK },
];

const legalAboutLinks = [
  { titleKey: "welcomeShell.nav.privacyPolicy", href: "/privacy-policy" },
  { titleKey: "welcomeShell.nav.userAgreement", href: "/user-agreement" },
];

function LogoWordmark({ href }: { href: string }) {
  return (
    <a href={href} className="inline-flex items-center gap-3">
      <img src="/logo-dark.png" alt="AuthyIDE" className="size-10" />
      <span className="text-[17px] font-semibold tracking-[-0.02em] text-[var(--a-fg)]">
        Authy<span className="text-[var(--a-accent)]">IDE</span>
      </span>
    </a>
  );
}

function HeaderAction({
  to,
  href,
  external,
  children,
  primary,
}: {
  to?: string;
  href?: string;
  external?: boolean;
  children: React.ReactNode;
  primary?: boolean;
}) {
  const className = cn(
    "inline-flex items-center justify-center gap-2 rounded-[4px] border px-3.5 py-2 text-[13px] transition-colors",
    primary
      ? "border-[rgba(37,99,235,0.3)] bg-[var(--a-accent)] text-[var(--a-bg)] shadow-[0_0_24px_rgba(37,99,235,0.24)] hover:bg-[#1d4ed8]"
      : "border-[var(--a-line-2)] bg-[var(--a-panel)] text-[var(--a-fg)] hover:bg-[#e8eef8] hover:text-[var(--a-fg)]"
  );

  if (to) return <Link to={to} className={className}>{children}</Link>;

  return (
    <a href={href} target={external ? "_blank" : undefined} rel={external ? "noreferrer" : undefined} className={className}>
      {children}
    </a>
  );
}

function FooterLinkItem({ title, href }: { title: string; href: string }) {
  if (href.startsWith("/")) {
    return (
      <Link to={href} className="block py-1.5 text-sm text-[var(--a-fg-dim)] transition-colors hover:text-[var(--a-fg)]">
        {title}
      </Link>
    );
  }

  return (
    <a href={href} target="_blank" rel="noreferrer" className="block py-1.5 text-sm text-[var(--a-fg-dim)] transition-colors hover:text-[var(--a-fg)]">
      {title}
    </a>
  );
}

export function TerminalHeader({ homeAnchors = true }: { homeAnchors?: boolean }) {
  const { auth, serverConfig } = useAppRuntime();
  const isLoggedIn = auth.status === "authenticated";
  const isGlobalRegion = serverConfig?.region === "global";
  const { t } = useTranslation();
  const [isScrolled, setIsScrolled] = React.useState(false);
  const [menuOpen, setMenuOpen] = React.useState(false);
  const inviterId = typeof window !== "undefined" ? localStorage.getItem("ic") || "" : "";
  const signUpLink = `/api/v1/users/login?redirect=&inviter_id=${inviterId}`;
  const navPrefix = homeAnchors ? "" : "/";

  const pageNav: Array<{ labelKey: string; href: string; external?: boolean }> = [
    { labelKey: "welcomeShell.nav.showcase", href: SHOWCASE_LINK },
    { labelKey: "welcomeShell.nav.desktopClient", href: `${navPrefix}#desktop-client` },
    { labelKey: "welcomeShell.nav.mobileClient", href: `${navPrefix}#mobile-client` },
    { labelKey: "welcomeShell.nav.selfHosting", href: SELF_HOSTING_PAGE_PATH },
  ];

  React.useEffect(() => {
    const handleScroll = () => setIsScrolled(window.scrollY > 4);
    handleScroll();
    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, []);

  return (
    <header
      className={cn(
        "fixed inset-x-0 top-0 z-50 border-b border-[var(--a-line)] backdrop-blur-xl transition-colors",
        isScrolled ? "bg-[rgba(255,255,255,0.94)]" : "bg-[rgba(255,255,255,0.85)]"
      )}
    >
      <div className="mx-auto flex max-w-[1280px] items-center gap-5 px-4 py-3 sm:px-8">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={() => setMenuOpen((value) => !value)}
            className="inline-flex size-10 items-center justify-center rounded-[4px] border border-[var(--a-line-2)] bg-[var(--a-panel)] text-[var(--a-fg)] transition-colors hover:bg-[#e8eef8] md:hidden"
            aria-label={t("welcomeShell.nav.toggleMenu")}
          >
            <IconMenu2 className="size-5" />
          </button>

          <LogoWordmark href={homeAnchors ? "#hero" : "/"} />
        </div>

        <nav className="hidden items-center gap-1 md:flex">
          {pageNav.map((item) =>
            item.external ? (
              <a
                key={item.labelKey}
                href={item.href}
                target="_blank"
                rel="noreferrer"
                className="px-2.5 py-1.5 text-[13px] text-[var(--a-fg-dim)] transition-colors hover:text-[var(--a-fg)]"
              >
                {t(item.labelKey)}
              </a>
            ) : (
              <a
                key={item.labelKey}
                href={item.href}
                className="px-2.5 py-1.5 text-[13px] text-[var(--a-fg-dim)] transition-colors hover:text-[var(--a-fg)]"
              >
                {t(item.labelKey)}
              </a>
            )
          )}
        </nav>

        <div className="ml-auto hidden items-center gap-2 md:flex">
          {!isGlobalRegion && !isLoggedIn ? (
            <HeaderAction href={signUpLink}>{t("welcomeShell.actions.signUp")}</HeaderAction>
          ) : null}
          {!isLoggedIn ? (
            <HeaderAction to="/login" primary>
              {t("welcomeShell.actions.login")}
            </HeaderAction>
          ) : (
            <HeaderAction to="/console" primary>
              {t("welcomeShell.actions.console")} <IconArrowRight className="size-4" />
            </HeaderAction>
          )}
        </div>

        {menuOpen ? (
          <div className="mt-4 space-y-4 border-t border-[var(--a-line)] pt-4 md:hidden">
            <div className="flex items-center gap-2 text-[11px] uppercase tracking-[0.22em] text-[var(--a-fg-dim)]">
              <IconPointFilled className="size-3 text-[var(--a-accent)]" />
              {t("welcomeShell.terminal.systemOnline")}
            </div>
            <div className="grid gap-2">
              {pageNav.map((item) =>
                item.external ? (
                  <a
                    key={item.labelKey}
                    href={item.href}
                    target="_blank"
                    rel="noreferrer"
                    onClick={() => setMenuOpen(false)}
                    className="rounded-[4px] border border-[var(--a-line)] px-4 py-3 text-sm text-[var(--a-fg)] transition-colors hover:bg-[#e8eef8]"
                  >
                    {t(item.labelKey)}
                  </a>
                ) : (
                  <a
                    key={item.labelKey}
                    href={item.href}
                    onClick={() => setMenuOpen(false)}
                    className="rounded-[4px] border border-[var(--a-line)] px-4 py-3 text-sm text-[var(--a-fg)] transition-colors hover:bg-[#e8eef8]"
                  >
                    {t(item.labelKey)}
                  </a>
                )
              )}
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              {!isGlobalRegion && !isLoggedIn ? (
                <HeaderAction href={signUpLink}>{t("welcomeShell.actions.signUp")}</HeaderAction>
              ) : null}
              {!isLoggedIn ? (
                <HeaderAction to="/login" primary>
                  {t("welcomeShell.actions.login")}
                </HeaderAction>
              ) : (
                <HeaderAction to="/console" primary>
                  {t("welcomeShell.actions.console")} <IconArrowRight className="size-4" />
                </HeaderAction>
              )}
            </div>
          </div>
        ) : null}
      </div>
    </header>
  );
}
export function TerminalFooter() {
  const { t } = useTranslation();
  const aboutLinks = [...companyLinks, ...legalAboutLinks];

  return (
    <footer id="community" className="relative z-10 mt-10 border-t border-[var(--a-line)] px-5 pb-8 pt-14 sm:px-8">
      <div className="mx-auto max-w-[1280px]">
        <div className="grid gap-10 xl:grid-cols-[1.3fr_0.8fr_0.8fr]">
          <div>
            <LogoWordmark href="/#hero" />
            <p className="mt-5 max-w-[320px] text-sm leading-7 text-[var(--a-fg-dim)]">
              {t("welcomeShell.footer.description")}
            </p>
          </div>

          <div>
            <div className="mb-4 text-[11px] font-semibold tracking-[0.08em] text-[var(--a-fg)]"># {t("welcomeShell.footer.resources")}</div>
            {resourceLinks.map((item) => (
              <FooterLinkItem key={item.titleKey} title={t(item.titleKey)} href={item.href} />
            ))}
          </div>

          <div>
            <div className="mb-4 text-[11px] font-semibold tracking-[0.08em] text-[var(--a-fg)]"># {t("welcomeShell.footer.about")}</div>
            {aboutLinks.map((item) => (
              <FooterLinkItem key={item.titleKey} title={t(item.titleKey)} href={item.href} />
            ))}
          </div>
        </div>

        <div className="mt-10 flex flex-col gap-3 border-t border-dashed border-[var(--a-line-2)] pt-5 text-[11px] tracking-[0.06em] text-[var(--a-fg-mute)] sm:flex-row sm:items-center sm:justify-between">
          <span>{t("welcomeShell.footer.copyright")}</span>
        </div>
      </div>
    </footer>
  );
}
