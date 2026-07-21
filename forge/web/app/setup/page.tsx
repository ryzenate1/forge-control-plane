"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Check, Database, Eye, EyeOff, Globe, Mail, Server, ShieldCheck, HardDrive, Building2 } from "lucide-react";
import { fetchSetupStatus, runSetup } from "@/lib/api";
import { AuthShell } from "@/components/ui/auth-shell";
import { Alert, Button, Field, Input, Select } from "@/components/ui/primitives";

type SetupStep = 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8;

export default function SetupPage() {
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
    onError: (error) => setErrors({ form: error instanceof Error ? error.message : "Setup could not be completed." }),
  });

  if (statusQuery.isPending)
    return (
      <AuthShell
        eyebrow="First-run setup"
        title="Checking readiness"
        description="Connecting to the panel API before setup begins."
      >
        <div className="ui-card p-6 text-sm text-slate-400" role="status">
          Verifying environment…
        </div>
      </AuthShell>
    );
  if (statusQuery.isError)
    return (
      <AuthShell
        eyebrow="First-run setup"
        title="Readiness check failed"
        description="Setup remains locked until the API confirms its state."
      >
        <Alert
          actions={
            <Button loading={statusQuery.isFetching} onClick={() => void statusQuery.refetch()} variant="secondary">
              Retry
            </Button>
          }
          title="Unable to verify setup status"
          tone="error"
        >
          No setup state has been assumed. Confirm the API and database services are reachable, then retry.
        </Alert>
      </AuthShell>
    );
  if (!statusQuery.data?.required && step !== 8)
    return (
      <AuthShell title="Setup already complete" description="This panel already has an administrator.">
        <div className="ui-card p-6 text-sm text-slate-400" role="status">
          Returning to sign in…
        </div>
      </AuthShell>
    );

  const stepLabels = [
    { n: 1, label: "Readiness" },
    { n: 2, label: "Administrator" },
    { n: 3, label: "Organization" },
    { n: 4, label: "Node" },
    { n: 5, label: "SMTP" },
    { n: 6, label: "Backup" },
    { n: 7, label: "Domain" },
  ];

  const visibleSteps = stepLabels.slice(
    0,
    stepLabels.findIndex((s) => s.n === Math.min(step, 8)) + 1
  );

  return (
    <AuthShell
      eyebrow="First-run setup"
      title={
        step === 1
          ? "Panel readiness"
          : step === 2
            ? "Create the first administrator"
            : step === 3
              ? "Organization setup"
              : step === 4
                ? "Node configuration"
                : step === 5
                  ? "SMTP configuration"
                  : step === 6
                    ? "Backup destination"
                    : step === 7
                      ? "Domain & TLS"
                      : "Setup complete"
      }
      description={
        step === 1
          ? "Review the state reported by the setup API before configuring the panel."
          : step === 2
            ? "These credentials will have full administrative access to the panel."
            : step === 3
              ? "Set up your organization details."
              : step === 4
                ? "Configure the first game server node."
                : step === 5
                  ? "Configure email delivery for notifications."
                  : step === 6
                    ? "Choose where backups are stored."
                    : step === 7
                      ? "Configure your domain and TLS certificate."
                      : "Your administrator account and initial configuration are ready."
      }
    >
      <ol aria-label="Setup progress" className="mb-5 flex flex-wrap gap-2 text-xs">
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
              <h2 className="font-semibold text-slate-100">API is ready</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                The API reports that setup is required and no administrator exists.
              </p>
              <dl className="mt-4 grid grid-cols-2 gap-3 text-xs">
                <div className="rounded-lg bg-black/20 p-3">
                  <dt className="text-slate-500">Version</dt>
                  <dd className="mt-1 font-mono text-slate-200">{statusQuery.data?.appVersion || "Not reported"}</dd>
                </div>
                <div className="rounded-lg bg-black/20 p-3">
                  <dt className="text-slate-500">Administrator</dt>
                  <dd className="mt-1 text-slate-200">{statusQuery.data?.hasAdmin ? "Present" : "Not created"}</dd>
                </div>
              </dl>
            </div>
          </div>
          <Alert className="mt-5" tone="info">
            This check confirms only the setup API state. Optional services (SMTP, backup, TLS) can be configured now or
            later in settings.
          </Alert>
          <Button className="mt-5 w-full" onClick={() => setStep(2)}>
            Continue
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
            if (!/^\S+@\S+\.\S+$/.test(email.trim())) next.email = "Enter a valid email address.";
            if (password.length < 12) next.password = "Use at least 12 characters.";
            else if (password === email) next.password = "Choose a password that differs from your email.";
            if (confirm !== password) next.confirm = "Passwords do not match.";
            setErrors(next);
            if (!next.email && !next.password && !next.confirm) setStep(3);
          }}
        >
          <Field error={errors.email} id="setup-email" label="Administrator email">
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
            hint="At least 12 characters. A longer, unique passphrase is recommended."
            id="setup-password"
            label="Password"
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
                aria-label={showPassword ? "Hide passwords" : "Show passwords"}
                className="ui-icon-button absolute right-1 top-1"
                onClick={() => setShowPassword((value) => !value)}
                type="button"
              >
                {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
          </Field>
          <Field error={errors.confirm} id="setup-confirm" label="Confirm password">
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
              Back
            </Button>
            <Button type="submit">Continue</Button>
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
              <h2 className="font-semibold text-slate-100">Organization</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                Set up your organization or personal workspace name.
              </p>
            </div>
          </div>
          <Field hint="Your organization name shown throughout the panel." id="setup-org" label="Organization name">
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
              Back
            </Button>
            <Button type="submit">Continue</Button>
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
              <h2 className="font-semibold text-slate-100">Node configuration</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                Configure the first game server node. This can also be done later in Admin &rarr; Nodes.
              </p>
            </div>
          </div>
          <Field hint="A display name for this node." id="setup-node-name" label="Node name">
            <Input
              autoFocus
              id="setup-node-name"
              onChange={(event) => setSetupData({ ...setupData, nodeName: event.target.value })}
              placeholder="Primary Node"
              value={setupData.nodeName}
            />
          </Field>
          <Field hint="Public FQDN or IP address of this node." id="setup-node-fqdn" label="Node FQDN">
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
              Back
            </Button>
            <Button type="submit">Continue</Button>
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
              <h2 className="font-semibold text-slate-100">SMTP configuration</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                Configure email delivery for password resets and notifications. Can be skipped and configured later.
              </p>
            </div>
          </div>
          <Field hint="SMTP server hostname" id="setup-smtp-host" label="SMTP host">
            <Input
              autoFocus
              id="setup-smtp-host"
              onChange={(event) => setSetupData({ ...setupData, smtpHost: event.target.value })}
              placeholder="smtp.example.com"
              value={setupData.smtpHost}
            />
          </Field>
          <div className="grid grid-cols-2 gap-4">
            <Field id="setup-smtp-port" label="SMTP port">
              <Input
                id="setup-smtp-port"
                onChange={(event) => setSetupData({ ...setupData, smtpPort: event.target.value })}
                placeholder="587"
                value={setupData.smtpPort}
              />
            </Field>
            <Field id="setup-smtp-encryption" label="Encryption">
              <Select
                id="setup-smtp-encryption"
                onChange={(event) => setSetupData({ ...setupData, smtpEncryption: event.target.value })}
                value={setupData.smtpEncryption}
              >
                <option value="tls">STARTTLS (587)</option>
                <option value="ssl">SSL/TLS (465)</option>
                <option value="">None</option>
              </Select>
            </Field>
          </div>
          <Field id="setup-smtp-user" label="SMTP username">
            <Input
              id="setup-smtp-user"
              onChange={(event) => setSetupData({ ...setupData, smtpUser: event.target.value })}
              placeholder="user@example.com"
              value={setupData.smtpUser}
            />
          </Field>
          <Field id="setup-smtp-pass" label="SMTP password">
            <Input
              id="setup-smtp-pass"
              onChange={(event) => setSetupData({ ...setupData, smtpPass: event.target.value })}
              type="password"
              value={setupData.smtpPass}
            />
          </Field>
          <Field id="setup-smtp-from" label="From address">
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
              Back
            </Button>
            <Button type="submit">Continue</Button>
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
              <h2 className="font-semibold text-slate-100">Backup destination</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                Choose where game server backups are stored. Can be changed later in settings.
              </p>
            </div>
          </div>
          <Field id="setup-backup-driver" label="Backup driver">
            <Select
              id="setup-backup-driver"
              onChange={(event) => setSetupData({ ...setupData, backupDriver: event.target.value })}
              value={setupData.backupDriver}
            >
              <option value="local">Local disk</option>
              <option value="s3">S3-compatible</option>
            </Select>
          </Field>
          {setupData.backupDriver === "s3" ? (
            <>
              <Field id="setup-s3-bucket" label="S3 bucket">
                <Input
                  id="setup-s3-bucket"
                  onChange={(event) => setSetupData({ ...setupData, s3Bucket: event.target.value })}
                  placeholder="my-backup-bucket"
                  value={setupData.s3Bucket}
                />
              </Field>
              <Field id="setup-s3-region" label="S3 region">
                <Input
                  id="setup-s3-region"
                  onChange={(event) => setSetupData({ ...setupData, s3Region: event.target.value })}
                  placeholder="us-east-1"
                  value={setupData.s3Region}
                />
              </Field>
              <Field id="setup-s3-endpoint" label="S3 endpoint">
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
              Back
            </Button>
            <Button type="submit">Continue</Button>
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
              <h2 className="font-semibold text-slate-100">Domain &amp; TLS</h2>
              <p className="mt-1 text-sm leading-6 text-slate-400">
                Configure your panel domain and automatic TLS certificate via Let&rsquo;s Encrypt.
              </p>
            </div>
          </div>
          <Field hint="The domain name where the panel will be accessible." id="setup-domain" label="Panel domain">
            <Input
              autoFocus
              id="setup-domain"
              onChange={(event) => setSetupData({ ...setupData, domainName: event.target.value })}
              placeholder="panel.example.com"
              value={setupData.domainName}
            />
          </Field>
          <Field
            hint="Email for Let's Encrypt certificate notifications."
            id="setup-tls-email"
            label="TLS contact email"
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
            TLS certificates are provisioned automatically via Let&rsquo;s Encrypt when the domain is configured with a
            reverse proxy. This can also be set up manually later.
          </Alert>
          <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-between">
            <Button onClick={() => setStep(6)} type="button" variant="ghost">
              Back
            </Button>
            <Button loading={setupMutation.isPending} type="submit">
              Complete setup
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
          <h2 className="mt-4 text-lg font-semibold text-white">Setup complete</h2>
          <p className="mt-2 text-sm leading-6 text-slate-400">
            Your administrator account and initial configuration are ready. Sign in with{" "}
            <strong className="font-medium text-slate-200">{email.trim().toLowerCase()}</strong> to continue
            configuring the panel.
          </p>
          <div className="mt-4 rounded-lg bg-black/20 p-4 text-left text-xs text-slate-400">
            <p className="font-medium text-slate-300">What was configured:</p>
            <ul className="mt-2 list-inside list-disc space-y-1">
              <li>Administrator account created</li>
              {setupData.orgName ? <li>Organization: {setupData.orgName}</li> : null}
              {setupData.nodeName ? <li>Node: {setupData.nodeName}</li> : null}
              {setupData.smtpHost ? <li>SMTP: {setupData.smtpHost}</li> : <li>SMTP: Not configured (can be set later)</li>}
              <li>Backup: {setupData.backupDriver === "s3" ? "S3-compatible storage" : "Local disk"}</li>
              {setupData.domainName ? <li>Domain: {setupData.domainName}</li> : null}
            </ul>
          </div>
          <Link className="ui-button ui-button-primary mt-6 w-full" href="/?setup=complete">
            Continue to sign in
          </Link>
        </div>
      ) : null}
    </AuthShell>
  );
}
