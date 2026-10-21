import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
} from "@/components/ui/card"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@/components/ui/tabs"
import { Spinner } from "@/components/ui/spinner"
import React from "react"
import { toast } from "sonner"
import { apiRequest } from "@/utils/requestUtils"
import { Link, useNavigate } from "react-router-dom"
import { captchaChallenge } from "@/utils/common"
import { ArrowLeft, Eye, EyeOff } from "lucide-react"
import { IconBrandGithub, IconBrandGoogle, IconBrandWechat, IconBrandQq, IconBrandTiktok } from "@tabler/icons-react"
import { IS_OFFLINE_EDITION } from "@/utils/edition"
import { Api } from "@/api/Api"
import type { GithubComChaitinMonkeyCodeBackendDomainTeamOIDCPublicConfigResp as DomainTeamOIDCPublicConfigResp, GithubComGoYokoWebResp } from "@/api/Api"
import { useTranslation } from "react-i18next"
import { useAppRuntime } from "@/components/app-runtime-provider"

const USER_STORAGE_KEY = 'login_user'
const MANAGER_STORAGE_KEY = 'login_manager'
type OAuthProvider = "github" | "google" | "wechat" | "qq" | "douyin"

const OAUTH_PROVIDER_META: Record<OAuthProvider, { labelKey: string; Icon: React.ComponentType<{ className?: string }> }> = {
  github: { labelKey: "login.choices.github", Icon: IconBrandGithub },
  google: { labelKey: "login.choices.google", Icon: IconBrandGoogle },
  wechat: { labelKey: "login.choices.wechat", Icon: IconBrandWechat },
  qq: { labelKey: "login.choices.qq", Icon: IconBrandQq },
  douyin: { labelKey: "login.choices.douyin", Icon: IconBrandTiktok },
}

export default function LoginPage({
  className,
  ...props
}: React.ComponentProps<"div">) {
  const [userEmail, setUserEmail] = React.useState('')
  const [userPassword, setUserPassword] = React.useState('')
  const [registerPassword, setRegisterPassword] = React.useState('')
  const [registerConfirm, setRegisterConfirm] = React.useState('')
  const [registering, setRegistering] = React.useState(false)
  const [teamManagerEmail, setTeamManagerEmail] = React.useState('')
  const [teamManagerPassword, setTeamManagerPassword] = React.useState('')
  const [logging, setLogging] = React.useState(false)
  const [showUserPassword, setShowUserPassword] = React.useState(false)
  const [showManagerPassword, setShowManagerPassword] = React.useState(false)
  const [showRegisterPassword, setShowRegisterPassword] = React.useState(false)
  const [userLoginView, setUserLoginView] = React.useState<'choices' | 'password' | 'register'>('choices')
  const [agreedToTerms, setAgreedToTerms] = React.useState(true)
  const [oauthLoggingProvider, setOauthLoggingProvider] = React.useState<OAuthProvider | null>(null)
  const [oauthProviders, setOauthProviders] = React.useState<OAuthProvider[]>([])
  const [defaultOIDCConfig, setDefaultOIDCConfig] = React.useState<DomainTeamOIDCPublicConfigResp | null>(null)
  const navigate = useNavigate()
  const { t } = useTranslation()
  const { captchaEnabled, reloadAuth, serverConfig } = useAppRuntime()
  const serverRegion = serverConfig?.region as string | undefined
  const isGlobalRegion = serverRegion === "global"
  const defaultOIDCLoginURL = defaultOIDCConfig?.enabled ? defaultOIDCConfig.login_url : ''
  const oauthFallbackProviders = React.useMemo<OAuthProvider[]>(
    () => (isGlobalRegion ? ['github', 'google'] : ['wechat', 'qq', 'douyin']),
    [isGlobalRegion],
  )

  const ensureTermsAccepted = React.useCallback(() => {
    if (agreedToTerms) return true
    toast.error(t("login.toast.acceptTerms"))
    return false
  }, [agreedToTerms, t])

  React.useEffect(() => {
    try {
      const savedUser = localStorage.getItem(USER_STORAGE_KEY)
      if (savedUser) {
        const { email, password } = JSON.parse(savedUser)
        if (email) setUserEmail(email)
        if (password) setUserPassword(password)
      }
      const savedManager = localStorage.getItem(MANAGER_STORAGE_KEY)
      if (savedManager) {
        const { email, password } = JSON.parse(savedManager)
        if (email) setTeamManagerEmail(email)
        if (password) setTeamManagerPassword(password)
      }
    } catch {
      // ignore
    }
  }, [])

  React.useEffect(() => {
    if (!IS_OFFLINE_EDITION) return

    const controller = new AbortController()
    fetch('/api/v1/users/oidc/default-team', { signal: controller.signal })
      .then(async (resp) => {
        if (!resp.ok) return
        const body = await resp.json() as GithubComGoYokoWebResp & { data?: DomainTeamOIDCPublicConfigResp }
        if (body.code === 0 && body.data?.enabled && body.data.login_url) {
          setDefaultOIDCConfig(body.data)
        }
      })
      .catch((err) => {
        if ((err as Error).name !== 'AbortError') {
          console.warn('load default oidc config failed', err)
        }
      })

    return () => controller.abort()
  }, [])

  React.useEffect(() => {
    const controller = new AbortController()
    const load = async () => {
      try {
        const resp = await fetch('/api/v1/users/oauth/providers', { signal: controller.signal })
        if (!resp.ok) return setOauthProviders(oauthFallbackProviders)
        const body = (await resp.json()) as GithubComGoYokoWebResp & { data?: { providers?: string[] } }
        const providers = body.data?.providers
        if (body.code !== 0 || !Array.isArray(providers)) return setOauthProviders(oauthFallbackProviders)
        const supported = Object.keys(OAUTH_PROVIDER_META)
        setOauthProviders(providers.filter((p): p is OAuthProvider => supported.includes(p)))
      } catch (err) {
        if ((err as Error).name !== 'AbortError') {
          console.warn('load oauth providers failed', err)
        }
      }
    }

    void load()

    return () => controller.abort()
  }, [oauthFallbackProviders])

  React.useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    if (params.get('register') === 'verified') {
      toast.success(t("login.toast.registerVerified"))
      setUserLoginView('password')
      window.history.replaceState(null, '', window.location.pathname)
    }
  }, [t])

  const handleUserLogin = async () => {
    if (!ensureTermsAccepted()) return

    if (userEmail.trim() === '' || userPassword.trim() === '') {
      toast.error(t("login.toast.missingCredentials"))
      return
    }

    setLogging(true)

    const token = await captchaChallenge(captchaEnabled);
    if (token !== null) {
      await apiRequest('v1UsersPasswordLoginCreate', {
        email: userEmail.trim(),
        password: userPassword.trim(),
        captcha_token: token || '',
      }, [], async (resp) => {
        if (resp.code === 0) {
          localStorage.setItem(USER_STORAGE_KEY, JSON.stringify({ email: userEmail.trim(), password: userPassword.trim() }))
          await reloadAuth()
          navigate('/console/tasks')
        } else if (resp.code === 10637) {
          toast.error(t("login.toast.emailUnverified"))
        } else {
          toast.error(t("login.toast.loginFailed"))
        }
      })
    } else {
      toast.error(t("login.toast.captchaFailed"))
    }
    setLogging(false)
  }

  const handleRegister = async () => {
    if (!ensureTermsAccepted()) return

    if (userEmail.trim() === '' || registerPassword.trim() === '') {
      toast.error(t("login.toast.missingCredentials"))
      return
    }
    if (registerPassword.trim() !== registerConfirm.trim()) {
      toast.error(t("login.toast.passwordMismatch"))
      return
    }

    setRegistering(true)
    try {
      const resp = await fetch('/api/v1/users/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: userEmail.trim(), password: registerPassword.trim() }),
      })
      const body = await resp.json() as GithubComGoYokoWebResp
      if (body.code === 0) {
        toast.success(t("login.toast.registerSent"))
        setUserLoginView('choices')
        setRegisterPassword('')
        setRegisterConfirm('')
      } else if (body.code === 10635) {
        toast.error(t("login.toast.emailTaken"))
      } else {
        toast.error(t("login.toast.registerFailed"))
      }
    } catch {
      toast.error(t("login.toast.registerFailed"))
    }
    setRegistering(false)
  }

  const handleOAuthLogin = async (provider: OAuthProvider) => {
    if (oauthLoggingProvider) return
    if (!ensureTermsAccepted()) return

    setOauthLoggingProvider(provider)

    try {
      const response = await new Api().api.v1UsersOauthLoginDetail(provider, {
        redirect_url: "/console/tasks",
      })
      const authUrl = response.data?.data?.auth_url

      if (response.data?.code !== 0 || !authUrl) {
        toast.error(t("login.toast.loginFailed"))
        return
      }

      window.location.assign(authUrl)
    } catch {
      toast.error(t("login.toast.loginFailed"))
    } finally {
      setOauthLoggingProvider(null)
    }
  }

  const handleTeamManagerLogin = async () => {
    if (!ensureTermsAccepted()) return

    if (teamManagerEmail.trim() === '' || teamManagerPassword.trim() === '') {
      toast.error(t("login.toast.missingCredentials"))
      return
    }

    setLogging(true)

    const token = await captchaChallenge(captchaEnabled);
    if (token !== null) {

      await apiRequest('v1TeamsUsersLoginCreate', {
        email: teamManagerEmail.trim(),
        password: teamManagerPassword.trim(),
        captcha_token: token || '',
      }, [], (resp) => {
        if (resp.code === 0) {
          localStorage.setItem(MANAGER_STORAGE_KEY, JSON.stringify({ email: teamManagerEmail.trim(), password: teamManagerPassword.trim() }))
          navigate('/manager/')
        } else {
          toast.error(t("login.toast.loginFailed"))
        }
      })
    } else {
      toast.error(t("login.toast.captchaFailed"))
    }
    setLogging(false)

  }

  return (
    <div className="flex min-h-svh w-full items-center justify-center p-6 md:p-10">
      <div className="w-full max-w-sm">
        <div className={cn("flex flex-col gap-6", className)} {...props}>
          <Link to="/">
            <h1 className="text-2xl hover:font-bold">{t("login.title")}</h1>
          </Link>
          <Card>
            <CardContent>
              <Tabs defaultValue="user">
                <TabsList className="grid w-full grid-cols-2">
                  <TabsTrigger value="user">{t("login.tabs.user")}</TabsTrigger>
                  <TabsTrigger value="manager">{t("login.tabs.manager")}</TabsTrigger>
                </TabsList>

                <TabsContent value="user" className="mt-4">
                  {userLoginView === 'choices' ? (
                    <div className="mt-1 flex flex-col gap-4">
                      <div className="text-sm font-medium">{t("login.choices.title")}</div>
                      {oauthProviders.length > 0 && (
                        <div className="flex flex-col gap-3">
                          {oauthProviders.map((provider) => {
                            const { labelKey, Icon } = OAUTH_PROVIDER_META[provider]
                            return (
                              <Button
                                key={provider}
                                type="button"
                                size="lg"
                                variant={provider === 'github' ? 'default' : 'outline'}
                                className="w-full"
                                disabled={!!oauthLoggingProvider}
                                onClick={() => { void handleOAuthLogin(provider) }}
                              >
                                {oauthLoggingProvider === provider ? <Spinner className="size-4" /> : <Icon className="size-4" />}
                                {t(labelKey)}
                              </Button>
                            )
                          })}
                        </div>
                      )}
                      {IS_OFFLINE_EDITION && defaultOIDCLoginURL && (
                        <Button size="lg" className="w-full" asChild>
                          <a
                            href={defaultOIDCLoginURL}
                            onClick={(e) => {
                              if (!ensureTermsAccepted()) {
                                e.preventDefault()
                              }
                            }}
                          >
                            {defaultOIDCConfig?.display_name || t("login.choices.oidc")}
                          </a>
                        </Button>
                      )}
                      <Button
                        type="button"
                        size="lg"
                        variant="outline"
                        className="w-full"
                        onClick={() => setUserLoginView('password')}
                      >
                        {t("login.choices.password")}
                      </Button>
                      <Button
                        type="button"
                        size="lg"
                        variant="secondary"
                        className="w-full"
                        onClick={() => setUserLoginView('register')}
                      >
                        {t("login.choices.signup")}
                      </Button>
                    </div>
                  ) : userLoginView === 'register' ? (
                    <div className="mt-1 flex flex-col gap-4">
                      <div className="flex items-center justify-between gap-3">
                        <div className="text-sm font-medium">{t("login.choices.signup")}</div>
                        <Button type="button" variant="secondary" size="sm" onClick={() => setUserLoginView('choices')}>
                          <ArrowLeft size={14} />
                          {t("login.actions.back")}
                        </Button>
                      </div>
                      <form onSubmit={(e) => { e.preventDefault(); handleRegister(); }}>
                        <FieldGroup className="gap-5">
                          <Field>
                            <FieldLabel htmlFor="register-email">{t("login.fields.account")}</FieldLabel>
                            <Input
                              value={userEmail}
                              placeholder="authyide@example.com"
                              onChange={(e) => setUserEmail(e.target.value)}
                              id="register-email"
                              type="email"
                              required
                              disabled={registering}
                            />
                          </Field>
                          <Field>
                            <FieldLabel htmlFor="register-password">{t("login.fields.password")}</FieldLabel>
                            <div className="relative">
                              <Input
                                value={registerPassword}
                                placeholder="************"
                                onChange={(e) => setRegisterPassword(e.target.value)}
                                id="register-password"
                                type={showRegisterPassword ? "text" : "password"}
                                required
                                minLength={8}
                                disabled={registering}
                                className="pr-9"
                              />
                              <button
                                type="button"
                                tabIndex={-1}
                                onClick={() => setShowRegisterPassword(v => !v)}
                                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                              >
                                {showRegisterPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                              </button>
                            </div>
                            <FieldDescription>{t("login.fields.passwordHint")}</FieldDescription>
                          </Field>
                          <Field>
                            <FieldLabel htmlFor="register-password-confirm">{t("login.fields.passwordConfirm")}</FieldLabel>
                            <Input
                              value={registerConfirm}
                              placeholder="************"
                              onChange={(e) => setRegisterConfirm(e.target.value)}
                              id="register-password-confirm"
                              type={showRegisterPassword ? "text" : "password"}
                              required
                              minLength={8}
                              disabled={registering}
                            />
                          </Field>
                          <Field>
                            <Button type="submit" disabled={registering || !agreedToTerms} className="w-full">
                              {registering && <Spinner className="mr-2" />}
                              {t("login.actions.register")}
                            </Button>
                          </Field>
                        </FieldGroup>
                      </form>
                    </div>
                  ) : (
                    <div className="mt-1 flex flex-col gap-4">
                      <div className="flex items-center justify-between gap-3">
                        <div className="text-sm font-medium">{t("login.choices.password")}</div>
                        <Button type="button" variant="secondary" size="sm" onClick={() => setUserLoginView('choices')}>
                          <ArrowLeft size={14} />
                          {t("login.actions.back")}
                        </Button>
                      </div>
                      <form onSubmit={(e) => { e.preventDefault(); handleUserLogin(); }}>
                        <FieldGroup className="gap-5">
                          <Field>
                            <FieldLabel htmlFor="user-email">{t("login.fields.account")}</FieldLabel>
                            <Input
                              value={userEmail}
                              placeholder="authyide@example.com"
                              onChange={(e) => setUserEmail(e.target.value)}
                              id="user-email"
                              type="email"
                              required
                              disabled={logging}
                            />
                          </Field>
                          <Field>
                            <div className="flex flex-row items-center justify-between">
                              <FieldLabel htmlFor="user-password">{t("login.fields.password")}</FieldLabel>
                              <Link to="/findpassword" tabIndex={-1} className="text-sm text-muted-foreground hover:underline">
                                {t("login.actions.forgotPassword")}
                              </Link>
                            </div>
                            <div className="relative">
                              <Input
                                value={userPassword}
                                placeholder="************"
                                onChange={(e) => setUserPassword(e.target.value)}
                                id="user-password"
                                type={showUserPassword ? "text" : "password"}
                                required
                                disabled={logging}
                                className="pr-9"
                              />
                              <button
                                type="button"
                                tabIndex={-1}
                                onClick={() => setShowUserPassword(v => !v)}
                                className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                              >
                                {showUserPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                              </button>
                            </div>
                          </Field>
                          <Field>
                            <Button type="submit" disabled={logging || !agreedToTerms} className="w-full">
                              {logging && <Spinner className="mr-2" />}
                              {t("login.actions.login")}
                            </Button>
                          </Field>
                        </FieldGroup>
                      </form>
                    </div>
                  )}
                </TabsContent>
                <TabsContent value="manager" className="mt-4">
                  <form onSubmit={(e) => { e.preventDefault(); handleTeamManagerLogin(); }}>
                    <FieldGroup>
                      <Field>
                        <FieldLabel htmlFor="email">{t("login.fields.account")}</FieldLabel>
                        <Input
                          value={teamManagerEmail}
                          placeholder="authyide@example.com"
                          onChange={(e) => setTeamManagerEmail(e.target.value)}
                          id="email"
                          type="email"
                          required
                          disabled={logging}
                        />
                      </Field>
                      <Field>
                        <FieldLabel htmlFor="password">{t("login.fields.password")}</FieldLabel>
                        <div className="relative">
                          <Input
                            id="password"
                            placeholder="************"
                            type={showManagerPassword ? "text" : "password"}
                            required
                            disabled={logging}
                            value={teamManagerPassword}
                            onChange={(e) => setTeamManagerPassword(e.target.value)}
                            className="pr-9"
                          />
                          <button
                            type="button"
                            tabIndex={-1}
                            onClick={() => setShowManagerPassword(v => !v)}
                            className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                          >
                            {showManagerPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                          </button>
                        </div>
                      </Field>
                      <Field>
                        <Button type="submit" disabled={logging || !agreedToTerms}>
                        {logging && <Spinner />}
                        {t("login.actions.login")}
                      </Button>
                    </Field>
                  </FieldGroup>
                  </form>
                </TabsContent>
              </Tabs>
              <div className="mt-5 flex items-start gap-2.5 rounded-lg border border-border/60 bg-muted/20 px-3 py-2.5">
                <Checkbox
                  id="login-user-agreement"
                  checked={agreedToTerms}
                  onCheckedChange={(checked) => setAgreedToTerms(checked === true)}
                  className="mt-px size-3.5 rounded-[3px] [&_[data-slot=checkbox-indicator]>svg]:size-3"
                />
                <label htmlFor="login-user-agreement" className="text-[13px] leading-[18px] text-muted-foreground">
                  {t("login.agreement.prefix")}
                  {" "}
                  <Link to="/user-agreement" target="_blank" rel="noreferrer" className="text-foreground hover:underline">
                    {t("login.agreement.link")}
                  </Link>
                </label>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>

  )
}
