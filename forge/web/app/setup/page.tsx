"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Check, Database, Eye, EyeOff, Globe, Mail, Server, ShieldCheck, HardDrive, Building2 } from "lucide-react";
import { fetchSetupStatus, runSetup } from "@/lib/api";
import { AuthShell } from "@/components/ui/auth-shell";
import { Alert, Button, Field, Input, Select } from "@/components/ui/primitives";
import { useT } from "@/components/TranslationProvider";

type SetupStep = 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8;

export default function SetupPage() {
  const t = useT();
  const router = useRouter();
  const [step, setStep] = useState<SetupStep>(1);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [setupData, setSetupData] = useState({
    orgName: "",
    nodeName: "",
    nodeFqdn: "",
    smtpHost: "",
    smtpPort: "587",
    smtpUser: "",
    smtpPass: "",
    smtpFrom: "",
    smtpEncryption: "tls",
    backupDriver: "local",
    s3Bucket: "",
    s3Region: "",
    s3Endpoint: "",
    domainName: "",
    tlsEmail: "",
  });

  const statusQuery = useQuery({ queryKey: ["setup-status"], queryFn: fetchSetupStatus, retry: false });

  useEffect(() => {
    if (statusQuery.data && !statusQuery.data.required && step !== 8) {
      router.replace("/");
    }
  }, [router, statusQuery.data, step]);

  const setupMutation = useMutation({
    mutationFn: () =>
      runSetup({
        email: email.trim().toLowerCase(),
        password,
        ...setupData,
      }),
    onSuccess: () => setStep(8),
    onError: (error) => setErrors({ form: error instanceof Error ? error.message : t("setupWizard.genericError") }),
  });

  if (statusQuery.isPending)
    return (
      <AuthShell
        eyebrow={t("setupWizard.eyebrow")}
        title={t("setupWizard.checkingReadiness")}
        description={t("setupWizard.checkingReadinessDesc")}
      >
        <div className="ui-card p-6 text-sm text-slate-400" role="status">
          {t("setupWizard.verifyingEnvironment")}
        </div>
      </AuthShell>
    );
  if (statusQuery.isError)
    return (
      <AuthShell
        eyebrow={t("setupWizard.eyebrow")}
        title={t("setupWizard.readinessFailed")}
        description={t("setupWizard.readinessFailedDesc")}
      >
        <Alert
          actions={
            <Button loading={statusQuery.isFetching} onClick={() => void statusQuery.refetch()} variant="secondary">
              {t("common.retry")}
            </Button>
          }
          title={t("setupWizard.unableToVerify")}
          tone="error"
        >
          {t("setupWizard.noStateAssumed")}
        </Alert>
      </AuthShell>
    );
  if (!statusQuery.data?.required && step !== 8)
    return (
      <AuthShell title={t("setupWizard.alreadyComplete")} description={t("setupWizard.alreadyCompleteDesc")}>
        <div className="ui-card p-6 text-sm text-slate-400" role="status">
          {t("setupWizard.returningToSignIn")}
        </div>
      </AuthShell>
    );

  const stepLabels = [
    { n: 1, label: t("setupWizard.steps.readiness") },
    { n: 2, label: t("setupWizard.steps.administrator") },
    { n: 3, label: t("setupWizard.steps.organization") },
    { n: 4, label: t("setupWizard.steps.node") },
    { n: 5, label: t("setupWizard.steps.smtp") },
    { n: 6, label: t("setupWizard.steps.backup") },
    { n: 7, label: t("setupWizard.steps.domain") },
  ];

  const visibleSteps = stepLabels.slice(
    0,
    stepLabels.findIndex((s) => s.n === Math.min(step, 8)) + 1
  );

  return (
    <AuthShell
      eyebrow={t("setupWizard.eyebrow")}
      title={
        step === 1
          ? t("setupWizard.step1.title")
          : step === 2
            ? t("setupWizard.step2.title")
            : step === 3
              ? t("setupWizard.step3.title")
              : step === 4
                ? t("setupWizard.step4.title")
                : step === 5
                  ? t("setupWizard.step5.title")
                  : step === 6
                    ? t("setupWizard.step6.title")
                    : step === 7
                      ? t("setupWizard.step7.title")
                      : t("setupWizard.step8.title")
      }
      description={
        step === 1
          ? t("setupWizard.step1.description")
          : step === 2
            ? t("setupWizard.step2.description")
            : step === 3
              ? t("setupWizard.step3.description")
              : step === 4
                ? t("setupWizard.step4.description")
                : step === 5
                  ? t("setupWizard.step5.description")
                  : step === 6
                    ? t("setupWizard.step6.description")
                    : step === 7
                      ? t("setupWizard.step7.description")
                      : t("setupWizard.step8.description")
      }
    >
      <ol aria-label={t("setupWizard.progressLabel")} className="mb-5 flex flex-wrap gap-2 text-xs">
        {visibleSteps.map((item) => (
          <li
            aria-current={step === item.n ? "step" : undefined}
            className={`flex items-center gap-2 rounded-lg border px-3 py-2 ${
              step >= item.n
                ? "border-red-500/30 bg-red-500/10 text-red-200"
                : "border-white/10 text-slate-500"
            }`}
            key={item.n}
          >
            <span className="grid h-5 w-5 place-items-center rounded-full border border-current text-[10px]">
              {step > item.n ? <Check className="h-3 w-3" /> : item.n}
            </span>
            {item.label}
          </li>
        ))}
      </ol>

      {/* Step 1: Readiness */}
      {step === 1 ? (
        <div className="ui-card p-5 sm:p-6">
          <div className="flex items-start gap-4">
            <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-emerald-500/10 text-emerald-400">
              <Database className="h-5 w-5" />
            </span>
            <div>
              <h2 className="font-semibold text-slate-100">{t("setupWizard.step1.apiReady")}</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                {t("setupWizard.step1.apiReadyDesc")}
              </p>
              <dl className="mt-4 grid grid-cols-2 gap-3 text-xs">
                <div className="rounded-lg bg-black/20 p-3">
                  <dt className="text-slate-500">{t("setupWizard.step1.version")}</dt>
                  <dd className="mt-1 font-mono text-slate-200">{statusQuery.data?.appVersion || t("setupWizard.step1.notReported")}</dd>
                </div>
                <div className="rounded-lg bg-black/20 p-3">
                  <dt className="text-slate-500">{t("setupWizard.step1.administrator")}</dt>
                  <dd className="mt-1 text-slate-200">{statusQuery.data?.hasAdmin ? t("setupWizard.step1.present") : t("setupWizard.step1.notCreated")}</dd>
                </div>
              </dl>
            </div>
          </div>
          <Alert className="mt-5" tone="info">
            {t("setupWizard.step1.infoNote")}
          </Alert>
          <Button className="mt-5 w-full" onClick={() => setStep(2)}>
            {t("common.continue")}
          </Button>
        </div>
      ) : null}

      {/* Step 2: Administrator */}
      {step === 2 ? (
        <form
          className="ui-card space-y-5 p-5 sm:p-6"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            const next: Record<string, string> = {};
            if (!/^\S+@\S+\.\S+$/.test(email.trim())) next.email = t("setupWizard.step2.errors.invalidEmail");
            if (password.length < 12) next.password = t("setupWizard.step2.errors.tooShort");
            else if (password === email) next.password = t("setupWizard.step2.errors.sameAsEmail");
            if (confirm !== password) next.confirm = t("setupWizard.step2.errors.mismatch");
            setErrors(next);
            if (!next.email && !next.password && !next.confirm) setStep(3);
          }}
        >
          <Field error={errors.email} id="setup-email" label={t("setupWizard.step2.emailLabel")}>
            <Input
              autoComplete="email"
              autoFocus
              id="setup-email"
              invalid={Boolean(errors.email)}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="admin@example.com"
              type="email"
              value={email}
            />
          </Field>
          <Field
            error={errors.password}
            hint={t("setupWizard.step2.passwordHint")}
            id="setup-password"
            label={t("auth.password")}
          >
            <div className="relative">
              <Input
                autoComplete="new-password"
                className="pr-11"
                id="setup-password"
                invalid={Boolean(errors.password)}
                minLength={12}
                onChange={(event) => setPassword(event.target.value)}
                type={showPassword ? "text" : "password"}
                value={password}
              />
              <button
                aria-label={showPassword ? t("setupWizard.step2.hidePasswords") : t("setupWizard.step2.showPasswords")}
                className="ui-icon-button absolute right-1 top-1"
                onClick={() => setShowPassword((value) => !value)}
                type="button"
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
          </Field>
          <Field error={errors.confirm} id="setup-confirm" label={t("setupWizard.step2.confirmLabel")}>
            <Input
              autoComplete="new-password"
              id="setup-confirm"
              invalid={Boolean(errors.confirm)}
              onChange={(event) => setConfirm(event.target.value)}
              type={showPassword ? "text" : "password"}
              value={confirm}
            />
          </Field>
          {errors.form ? <Alert tone="error">{errors.form}</Alert> : null}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
            <Button onClick={() => setStep(1)} type="button" variant="ghost">
              {t("common.back")}
            </Button>
            <Button type="submit">{t("common.continue")}</Button>
          </div>
        </form>
      ) : null}

      {/* Step 3: Organization */}
      {step === 3 ? (
        <form
          className="ui-card space-y-5 p-5 sm:p-6"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            setStep(4);
          }}
        >
          <div className="flex items-start gap-4">
            <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-violet-500/10 text-violet-400">
              <Building2 className="h-5 w-5" />
            </span>
            <div>
              <h2 className="font-semibold text-slate-100">{t("setupWizard.step3.heading")}</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                {t("setupWizard.step3.subheading")}
              </p>
            </div>
          </div>
          <Field hint={t("setupWizard.step3.orgHint")} id="setup-org" label={t("setupWizard.step3.orgLabel")}>
            <Input
              autoFocus
              id="setup-org"
              onChange={(event) => setSetupData({ ...setupData, orgName: event.target.value })}
              placeholder="My Game Host"
              value={setupData.orgName}
            />
          </Field>
          {errors.form ? <Alert tone="error">{errors.form}</Alert> : null}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
            <Button onClick={() => setStep(2)} type="button" variant="ghost">
              {t("common.back")}
            </Button>
            <Button type="submit">{t("common.continue")}</Button>
          </div>
        </form>
      ) : null}

      {/* Step 4: Node Configuration */}
      {step === 4 ? (
        <form
          className="ui-card space-y-5 p-5 sm:p-6"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            setStep(5);
          }}
        >
          <div className="flex items-start gap-4">
            <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-blue-500/10 text-blue-400">
              <Server className="h-5 w-5" />
            </span>
            <div>
              <h2 className="font-semibold text-slate-100">{t("setupWizard.step4.heading")}</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                {t("setupWizard.step4.subheading")}
              </p>
            </div>
          </div>
          <Field hint={t("setupWizard.step4.nameHint")} id="setup-node-name" label={t("setupWizard.step4.nameLabel")}>
            <Input
              autoFocus
              id="setup-node-name"
              onChange={(event) => setSetupData({ ...setupData, nodeName: event.target.value })}
              placeholder="Primary Node"
              value={setupData.nodeName}
            />
          </Field>
          <Field hint={t("setupWizard.step4.fqdnHint")} id="setup-node-fqdn" label={t("setupWizard.step4.fqdnLabel")}>
            <Input
              id="setup-node-fqdn"
              onChange={(event) => setSetupData({ ...setupData, nodeFqdn: event.target.value })}
              placeholder="node1.example.com"
              value={setupData.nodeFqdn}
            />
          </Field>
          {errors.form ? <Alert tone="error">{errors.form}</Alert> : null}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
            <Button onClick={() => setStep(3)} type="button" variant="ghost">
              {t("common.back")}
            </Button>
            <Button type="submit">{t("common.continue")}</Button>
          </div>
        </form>
      ) : null}

      {/* Step 5: SMTP Configuration */}
      {step === 5 ? (
        <form
          className="ui-card space-y-5 p-5 sm:p-6"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            setStep(6);
          }}
        >
          <div className="flex items-start gap-4">
            <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-amber-500/10 text-amber-400">
              <Mail className="h-5 w-5" />
            </span>
            <div>
              <h2 className="font-semibold text-slate-100">{t("setupWizard.step5.heading")}</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                {t("setupWizard.step5.subheading")}
              </p>
            </div>
          </div>
          <Field hint={t("setupWizard.step5.hostHint")} id="setup-smtp-host" label={t("setupWizard.step5.hostLabel")}>
            <Input
              autoFocus
              id="setup-smtp-host"
              onChange={(event) => setSetupData({ ...setupData, smtpHost: event.target.value })}
              placeholder="smtp.example.com"
              value={setupData.smtpHost}
            />
          </Field>
          <div className="grid grid-cols-2 gap-4">
            <Field id="setup-smtp-port" label={t("setupWizard.step5.portLabel")}>
              <Input
                id="setup-smtp-port"
                onChange={(event) => setSetupData({ ...setupData, smtpPort: event.target.value })}
                placeholder="587"
                value={setupData.smtpPort}
              />
            </Field>
            <Field id="setup-smtp-encryption" label={t("setupWizard.step5.encryptionLabel")}>
              <Select
                id="setup-smtp-encryption"
                onChange={(event) => setSetupData({ ...setupData, smtpEncryption: event.target.value })}
                value={setupData.smtpEncryption}
              >
                <option value="tls">{t("setupWizard.step5.starttls")}</option>
                <option value="ssl">{t("setupWizard.step5.ssl")}</option>
                <option value="">{t("setupWizard.step5.none")}</option>
              </Select>
            </Field>
          </div>
          <Field id="setup-smtp-user" label={t("setupWizard.step5.userLabel")}>
            <Input
              id="setup-smtp-user"
              onChange={(event) => setSetupData({ ...setupData, smtpUser: event.target.value })}
              placeholder="user@example.com"
              value={setupData.smtpUser}
            />
          </Field>
          <Field id="setup-smtp-pass" label={t("setupWizard.step5.passLabel")}>
            <Input
              id="setup-smtp-pass"
              onChange={(event) => setSetupData({ ...setupData, smtpPass: event.target.value })}
              type="password"
              value={setupData.smtpPass}
            />
          </Field>
          <Field id="setup-smtp-from" label={t("setupWizard.step5.fromLabel")}>
            <Input
              id="setup-smtp-from"
              onChange={(event) => setSetupData({ ...setupData, smtpFrom: event.target.value })}
              placeholder="noreply@example.com"
              value={setupData.smtpFrom}
            />
          </Field>
          {errors.form ? <Alert tone="error">{errors.form}</Alert> : null}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
            <Button onClick={() => setStep(4)} type="button" variant="ghost">
              {t("common.back")}
            </Button>
            <Button type="submit">{t("common.continue")}</Button>
          </div>
        </form>
      ) : null}

      {/* Step 6: Backup Destination */}
      {step === 6 ? (
        <form
          className="ui-card space-y-5 p-5 sm:p-6"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            setStep(7);
          }}
        >
          <div className="flex items-start gap-4">
            <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-teal-500/10 text-teal-400">
              <HardDrive className="h-5 w-5" />
            </span>
            <div>
              <h2 className="font-semibold text-slate-100">{t("setupWizard.step6.heading")}</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                {t("setupWizard.step6.subheading")}
              </p>
            </div>
          </div>
          <Field id="setup-backup-driver" label={t("setupWizard.step6.driverLabel")}>
            <Select
              id="setup-backup-driver"
              onChange={(event) => setSetupData({ ...setupData, backupDriver: event.target.value })}
              value={setupData.backupDriver}
            >
              <option value="local">{t("setupWizard.step6.local")}</option>
              <option value="s3">{t("setupWizard.step6.s3")}</option>
            </Select>
          </Field>
          {setupData.backupDriver === "s3" ? (
            <>
              <Field id="setup-s3-bucket" label={t("setupWizard.step6.bucketLabel")}>
                <Input
                  id="setup-s3-bucket"
                  onChange={(event) => setSetupData({ ...setupData, s3Bucket: event.target.value })}
                  placeholder="my-backup-bucket"
                  value={setupData.s3Bucket}
                />
              </Field>
              <Field id="setup-s3-region" label={t("setupWizard.step6.regionLabel")}>
                <Input
                  id="setup-s3-region"
                  onChange={(event) => setSetupData({ ...setupData, s3Region: event.target.value })}
                  placeholder="us-east-1"
                  value={setupData.s3Region}
                />
              </Field>
              <Field id="setup-s3-endpoint" label={t("setupWizard.step6.endpointLabel")}>
                <Input
                  id="setup-s3-endpoint"
                  onChange={(event) => setSetupData({ ...setupData, s3Endpoint: event.target.value })}
                  placeholder="https://s3.amazonaws.com"
                  value={setupData.s3Endpoint}
                />
              </Field>
            </>
          ) : null}
          {errors.form ? <Alert tone="error">{errors.form}</Alert> : null}
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
            <Button onClick={() => setStep(5)} type="button" variant="ghost">
              {t("common.back")}
            </Button>
            <Button type="submit">{t("common.continue")}</Button>
          </div>
        </form>
      ) : null}

      {/* Step 7: Domain & TLS */}
      {step === 7 ? (
        <form
          className="ui-card space-y-5 p-5 sm:p-6"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            setupMutation.mutate();
          }}
        >
          <div className="flex items-start gap-4">
            <span className="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-indigo-500/10 text-indigo-400">
              <Globe className="h-5 w-5" />
            </span>
            <div>
              <h2 className="font-semibold text-slate-100">{t("setupWizard.step7.heading")}</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                {t("setupWizard.step7.subheading")}
              </p>
            </div>
          </div>
          <Field hint={t("setupWizard.step7.domainHint")} id="setup-domain" label={t("setupWizard.step7.domainLabel")}>
            <Input
              autoFocus
              id="setup-domain"
              onChange={(event) => setSetupData({ ...setupData, domainName: event.target.value })}
              placeholder="panel.example.com"
              value={setupData.domainName}
            />
          </Field>
          <Field
            hint={t("setupWizard.step7.tlsEmailHint")}
            id="setup-tls-email"
            label={t("setupWizard.step7.tlsEmailLabel")}
          >
            <Input
              id="setup-tls-email"
              onChange={(event) => setSetupData({ ...setupData, tlsEmail: event.target.value })}
              placeholder="admin@example.com"
              value={setupData.tlsEmail}
            />
          </Field>
          {errors.form ? <Alert tone="error">{errors.form}</Alert> : null}
          <Alert tone="info">
            {t("setupWizard.step7.tlsInfo")}
          </Alert>
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
            <Button onClick={() => setStep(6)} type="button" variant="ghost">
              {t("common.back")}
            </Button>
            <Button loading={setupMutation.isPending} type="submit">
              {t("setupWizard.step7.completeSetup")}
            </Button>
          </div>
        </form>
      ) : null}

      {/* Step 8: Complete */}
      {step === 8 ? (
        <div className="ui-card p-6 text-center">
          <span className="mx-auto grid h-14 w-14 place-items-center rounded-full bg-emerald-500/10 text-emerald-400">
            <ShieldCheck className="h-7 w-7" />
          </span>
          <h2 className="mt-4 text-lg font-semibold text-white">{t("setupWizard.step8.title")}</h2>
          <p className="mt-2 text-sm leading-6 text-slate-400">
            {t("setupWizard.step8.readyIntro")}{" "}
            <strong className="font-medium text-slate-200">{email.trim().toLowerCase()}</strong> {t("setupWizard.step8.readyOutro")}
          </p>
          <div className="mt-4 rounded-lg bg-black/20 p-4 text-left text-xs text-slate-400">
            <p className="font-medium text-slate-300">{t("setupWizard.step8.whatConfigured")}</p>
            <ul className="mt-2 list-inside list-disc space-y-1">
              <li>{t("setupWizard.step8.adminCreated")}</li>
              {setupData.orgName ? <li>{t("setupWizard.step8.organizationLine", { name: setupData.orgName })}</li> : null}
              {setupData.nodeName ? <li>{t("setupWizard.step8.nodeLine", { name: setupData.nodeName })}</li> : null}
              {setupData.smtpHost ? <li>{t("setupWizard.step8.smtpLine", { host: setupData.smtpHost })}</li> : <li>{t("setupWizard.step8.smtpNotConfigured")}</li>}
              <li>{t("setupWizard.step8.backupLine", { driver: setupData.backupDriver === "s3" ? t("setupWizard.step6.s3") : t("setupWizard.step6.local") })}</li>
              {setupData.domainName ? <li>{t("setupWizard.step8.domainLine", { domain: setupData.domainName })}</li> : null}
            </ul>
          </div>
          <Link className="ui-button ui-button-primary mt-6 w-full" href="/?setup=complete">
            {t("setupWizard.step8.continueToSignIn")}
          </Link>
        </div>
      ) : null}
    </AuthShell>
  );
}
