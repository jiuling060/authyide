import { TerminalFooter, TerminalHeader } from "@/components/welcome/terminal-chrome";
import { cn } from "@/lib/utils";
import { IconArrowRight } from "@tabler/icons-react";
import React from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

const themeVars = {
  "--a-bg": "#ffffff",
  "--a-bg-2": "#f6f8fc",
  "--a-panel": "#ffffff",
  "--a-line": "#d7e2f2",
  "--a-line-2": "#c0d2ea",
  "--a-fg": "#13233f",
  "--a-fg-dim": "#5b708c",
  "--a-fg-mute": "#96a9c2",
  "--a-accent": "#2563eb",
  "--a-accent-dim": "#1d4ed8",
  "--a-warn": "#d97706",
  "--a-danger": "#dc2626",
  "--a-info": "#0ea5e9",
} as React.CSSProperties;

const navKeys = ["overview", "models", "identity", "gitbot", "activities", "quickstart", "support", "contact"] as const;

type StepItem = { title: string; body: string };
type ProviderItem = { name: string; base: string };
type ContactItem = { label: string; value: string; note: string };

function NavPill({ active, href, label }: { active: boolean; href: string; label: string }) {
  return (
    <a
      href={href}
      className={cn(
        "block whitespace-nowrap rounded border px-4 py-2.5 text-sm font-medium transition-colors",
        active
          ? "border-[rgba(37,99,235,0.4)] bg-[rgba(37,99,235,0.1)] text-[var(--a-accent)]"
          : "border-[var(--a-line-2)] bg-[var(--a-bg-2)] text-[var(--a-fg-dim)] hover:text-[var(--a-fg)]"
      )}
    >
      {label}
    </a>
  );
}

function DocSection({
  id,
  index,
  title,
  description,
  children,
}: {
  id: string;
  index: string;
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <section id={id} className="scroll-mt-[132px]">
      <div className="mb-6 flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div>
          <div className="text-[11px] tracking-[0.16em] text-[var(--a-accent)]">{index}</div>
          <h2 className="mt-3 text-3xl font-semibold tracking-[-0.03em] text-[var(--a-fg)] sm:text-4xl">{title}</h2>
        </div>
        <p className="max-w-[520px] text-sm leading-7 text-[var(--a-fg-dim)]">{description}</p>
      </div>
      {children}
    </section>
  );
}

function NumberedSteps({ steps }: { steps: { title: string; body: string }[] }) {
  return (
    <ol className="space-y-4">
      {steps.map((step, index) => (
        <li key={step.title} className="flex gap-5 rounded-md border border-[var(--a-line)] bg-[var(--a-panel)] p-5">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-full border border-[rgba(37,99,235,0.32)] bg-[var(--a-bg-2)] text-sm font-semibold text-[var(--a-accent)]">
            {index + 1}
          </span>
          <div className="min-w-0">
            <h3 className="text-base font-semibold tracking-[-0.01em] text-[var(--a-fg)]">{step.title}</h3>
            <p className="mt-2 text-sm leading-7 text-[var(--a-fg-dim)]">{step.body}</p>
          </div>
        </li>
      ))}
    </ol>
  );
}

function NoteBox({ children }: { children: React.ReactNode }) {
  return (
    <div className="mt-5 rounded-md border border-[var(--a-line-2)] bg-[var(--a-bg-2)] p-4">
      <p className="text-sm leading-7 text-[var(--a-fg-dim)]">{children}</p>
    </div>
  );
}

function ContactGrid({ items }: { items: ContactItem[] }) {
  return (
    <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
      {items.map((item) => (
        <div key={item.label} className="flex flex-col gap-2 rounded border border-[var(--a-line-2)] bg-[var(--a-panel)] p-5">
          <span className="text-[11px] tracking-[0.12em] text-[var(--a-fg-mute)]">{item.label}</span>
          <span className="font-mono text-sm text-[var(--a-accent)]">{item.value}</span>
          <p className="text-sm leading-7 text-[var(--a-fg-dim)]">{item.note}</p>
        </div>
      ))}
    </div>
  );
}

export default function DocsPage() {
  const { t } = useTranslation();

  return (
    <div style={themeVars} className="min-h-screen overflow-x-hidden bg-[var(--a-bg)] text-[var(--a-fg)]">
      <div className="pointer-events-none fixed inset-0">
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(37,99,235,0.12),transparent_28%),radial-gradient(circle_at_top_right,rgba(14,165,233,0.08),transparent_22%),linear-gradient(180deg,#ffffff_0%,#ffffff_48%,#f6f8fc_100%)]" />
      </div>

      <TerminalHeader homeAnchors={false} />

      <main className="relative z-10 mx-auto max-w-[1280px] px-5 pb-12 pt-[108px] sm:px-8 sm:pb-16 sm:pt-[120px]">
        <section className="border-b border-[var(--a-line)] pb-10">
          <div className="inline-flex rounded border border-[rgba(37,99,235,0.18)] bg-[rgba(37,99,235,0.08)] px-3 py-1 text-[11px] tracking-[0.16em] text-[var(--a-accent)]">
            {t("docsPage.hero.eyebrow")}
          </div>
          <h1 className="mt-6 text-4xl font-semibold leading-[1.04] tracking-[-0.04em] text-[var(--a-fg)] sm:text-5xl lg:text-[64px]">
            {t("docsPage.hero.title")}
          </h1>
          <p className="mt-5 max-w-[760px] text-sm leading-8 text-[var(--a-fg-dim)] sm:text-[15px]">
            {t("docsPage.hero.subtitle")}
          </p>
          <nav aria-label="documentation" className="mt-8 flex gap-2 overflow-x-auto pb-1">
            {navKeys.map((key) => (
              <NavPill key={key} href={`#${key}`} active={false} label={t(`docsPage.nav.${key}`)} />
            ))}
          </nav>
        </section>

        <div className="mt-12 space-y-14">
          <DocSection
            id="overview"
            index="01 / OVERVIEW"
            title={t("docsPage.overview.title")}
            description={t("docsPage.overview.description")}
          >
            <div className="grid gap-px overflow-hidden rounded-md border border-[var(--a-line)] bg-[var(--a-line)] md:grid-cols-2">
              {(t("docsPage.overview.items", { returnObjects: true }) as StepItem[]).map((item, index) => (
                <div key={item.title} className="bg-[var(--a-panel)] p-6">
                  <div className="text-[11px] tracking-[0.12em] text-[var(--a-fg-mute)]">
                    {String(index + 1).padStart(2, "0")}
                  </div>
                  <h3 className="mt-4 text-lg font-semibold tracking-[-0.02em] text-[var(--a-accent)]">{item.title}</h3>
                  <p className="mt-3 text-sm leading-7 text-[var(--a-fg-dim)]">{item.body}</p>
                </div>
              ))}
            </div>
          </DocSection>

          <DocSection
            id="models"
            index="02 / MODELS"
            title={t("docsPage.models.title")}
            description={t("docsPage.models.description")}
          >
            <NumberedSteps steps={t("docsPage.models.steps", { returnObjects: true }) as StepItem[]} />
            <NoteBox>{t("docsPage.models.note")}</NoteBox>
          </DocSection>

          <DocSection
            id="identity"
            index="03 / IDENTITY"
            title={t("docsPage.identity.title")}
            description={t("docsPage.identity.description")}
          >
            <div className="mb-5 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {(t("docsPage.identity.providers", { returnObjects: true }) as ProviderItem[]).map((provider) => (
                <div key={provider.name} className="flex items-baseline justify-between gap-4 rounded border border-[var(--a-line-2)] bg-[var(--a-panel)] px-4 py-3">
                  <span className="text-sm font-semibold text-[var(--a-fg)]">{provider.name}</span>
                  <span className="truncate font-mono text-xs text-[var(--a-fg-mute)]">{provider.base}</span>
                </div>
              ))}
            </div>
            <NumberedSteps steps={t("docsPage.identity.steps", { returnObjects: true }) as StepItem[]} />
            <NoteBox>{t("docsPage.identity.note")}</NoteBox>
          </DocSection>

          <DocSection
            id="gitbot"
            index="04 / GIT BOT"
            title={t("docsPage.gitbot.title")}
            description={t("docsPage.gitbot.description")}
          >
            <NumberedSteps steps={t("docsPage.gitbot.steps", { returnObjects: true }) as StepItem[]} />
            <NoteBox>{t("docsPage.gitbot.note")}</NoteBox>
          </DocSection>

          <DocSection
            id="activities"
            index="05 / ACTIVITIES"
            title={t("docsPage.activities.title")}
            description={t("docsPage.activities.description")}
          >
            <NumberedSteps steps={t("docsPage.activities.steps", { returnObjects: true }) as StepItem[]} />
            <NoteBox>{t("docsPage.activities.note")}</NoteBox>
          </DocSection>

          <DocSection
            id="quickstart"
            index="06 / QUICKSTART"
            title={t("docsPage.quickstart.title")}
            description=""
          >
            <NumberedSteps steps={t("docsPage.quickstart.steps", { returnObjects: true }) as StepItem[]} />
          </DocSection>

          <DocSection
            id="support"
            index="07 / SUPPORT"
            title={t("docsPage.support.title")}
            description={t("docsPage.support.description")}
          >
            <ContactGrid items={t("docsPage.support.items", { returnObjects: true }) as ContactItem[]} />
            <NoteBox>{t("docsPage.support.note")}</NoteBox>
          </DocSection>

          <DocSection
            id="contact"
            index="08 / CONTACT"
            title={t("docsPage.contact.title")}
            description={t("docsPage.contact.description")}
          >
            <ContactGrid items={t("docsPage.contact.items", { returnObjects: true }) as ContactItem[]} />
            <NoteBox>{t("docsPage.contact.note")}</NoteBox>
          </DocSection>
        </div>

        <section className="mt-14 rounded-md border border-[var(--a-line)] bg-[var(--a-panel)] p-6 sm:p-8">
          <div className="grid gap-6 lg:grid-cols-[1fr_auto] lg:items-center">
            <div>
              <div className="text-[11px] tracking-[0.16em] text-[var(--a-accent)]">GET STARTED</div>
              <h2 className="mt-3 text-3xl font-semibold tracking-[-0.03em] text-[var(--a-fg)]">
                {t("selfHostingPage.final.title")}
              </h2>
              <p className="mt-4 max-w-[760px] text-sm leading-7 text-[var(--a-fg-dim)]">
                {t("selfHostingPage.final.description")}
              </p>
            </div>
            <div className="flex flex-wrap gap-3">
              <Link
                to="/self-hosting"
                className="inline-flex items-center justify-center gap-2 rounded-[4px] border border-[rgba(37,99,235,0.3)] bg-[var(--a-accent)] px-4 py-3 text-sm font-semibold text-[var(--a-bg)] transition-colors hover:bg-[#1d4ed8]"
              >
                <IconArrowRight className="size-4" />
                <span>{t("docsPage.footer.selfHosting")}</span>
              </Link>
              <Link
                to="/"
                className="inline-flex items-center justify-center gap-2 rounded-[4px] border border-[var(--a-line-2)] bg-[var(--a-panel)] px-4 py-3 text-sm font-semibold text-[var(--a-fg)] transition-colors hover:bg-[#e8eef8] hover:text-[var(--a-fg)]"
              >
                <span>{t("docsPage.footer.backHome")}</span>
              </Link>
            </div>
          </div>
        </section>
      </main>

      <TerminalFooter />
    </div>
  );
}
