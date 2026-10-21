
import { Link } from "react-router-dom"
import { useTranslation } from "react-i18next"

const SUPPORT_LINK = "/docs#support"
const BUSINESS_LINK = "/docs#contact"

const resourceLinks = [
  {
    titleKey: "welcomeShell.footer.productDocs",
    href: "/docs"
  },
  {
    titleKey: "welcomeShell.footer.forum",
    href: "/"
  },
  {
    titleKey: "welcomeShell.nav.openSourceRepo",
    href: "https://github.com/jiuling060/AuthyIDE/"
  }
]

const companyLinks = [
  { titleKey: "welcomeShell.footer.support", href: SUPPORT_LINK },
  { titleKey: "welcomeShell.footer.business", href: BUSINESS_LINK }
]

const legalLinks = [
  {
    titleKey: "welcomeShell.nav.privacyPolicy",
    href: "/privacy-policy"
  },
  {
    titleKey: "welcomeShell.nav.userAgreement",
    href: "/user-agreement"
  }
]

const Footer = () => {
  const { t } = useTranslation()
  const links = [
    {
      titleKey: "welcomeShell.footer.resources",
      links: resourceLinks
    },
    {
      titleKey: "welcomeShell.footer.about",
      links: [...companyLinks, ...legalLinks]
    }
  ]

  return (
    <footer className="bg-primary px-10">
      <div className="flex flex-col lg:flex-row gap-10 lg:gap-0 justify-between mx-auto max-w-[1200px] py-10">
        <div className="flex flex-col gap-4">
          <h3 className="text-background flex flex-row items-center gap-4">
            <img src="/logo.png" className="size-8" />
            {t("welcomeShell.footer.brandTitle")}
          </h3>
          <p className="text-background/50 text-sm max-w-[350px]">
            {t("welcomeShell.footer.brandDescription")}
          </p>
        </div>
        {links.map((link) => (
          <div key={link.titleKey} className="flex flex-col gap-4">
            <h3 className="text-background leading-8">{t(link.titleKey)}</h3>
            <ul className="text-background/50 text-sm flex flex-col gap-2">
              {link.links.map((link) => {
                return (
                  <li key={link.titleKey}>
                    {link.href.startsWith("/") ? (
                      <Link to={link.href} className="flex items-center gap-2 hover:text-background">
                        {t(link.titleKey)}
                      </Link>
                    ) : (
                      <a href={link.href} target="_blank" rel="noreferrer" className="flex items-center gap-2 hover:text-background">
                        {t(link.titleKey)}
                      </a>
                    )}
                  </li>
                )
              })}
            </ul>
          </div>
        ))}
      </div>
    </footer>
  )
};

export default Footer;
