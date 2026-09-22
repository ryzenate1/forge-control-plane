"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useToast } from "@/components/ui/toast";
import { AlertTriangle, CheckCircle, Plus, RefreshCw, Shield, ShieldCheck, ShieldX, Trash2, XCircle } from "lucide-react";
import { fetchJSON, postJSON } from "@/lib/api";
import {
  AdminErrorState,
  AdminIconButton,
  AdminLoadingState,
  AdminLoadingRows,
  AdminPageLayout,
  AdminTable,
  AdminTBody,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Pill,
  SectionHeader,
  StatsRow,
} from "@/components/admin/admin-ui";
import { FreshnessBadge } from "@/components/admin/telemetry-ui";
import { sourceState } from "@/lib/admin/telemetry";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { errorMessage, formatDate } from "@/lib/utils";

type MTLSCert = {
  id: string;
  certType: "ca" | "server" | "client";
  commonName: string;
  organization: string;
  serialNumber: string;
  expiresAt: string;
  revokedAt: string | null;
  nodeId: string | null;
  createdAt: string;
};

type MTLSCertListResponse = { data: MTLSCert[] };
type MTLSCertResponse = { data: MTLSCert };
type MTLSStatusResponse = { data: { caConfigured: boolean; serverCertCount?: number; clientCertCount?: number; activeCerts?: number; revokedCount?: number; caExpiresAt?: string } };
type MTLSMigrationStatusResponse = { data: { caConfigured?: boolean; nodesWithCerts?: number; totalNodes?: number; migrationEnabled?: boolean; phase?: string } };

const EXPIRY_WARN_DAYS = 30;

/** A count the API did not send is not a zero. `GetMTLSStatus` ignores its scan
 * errors (`_ = s.db.QueryRow(...)`), so a missing field must read as unreported
 * rather than as a measured 0. */
function count(value: number | undefined) {
  return typeof value === "number" && Number.isFinite(value) ? value : "Not reported";
}

function daysUntil(value?: string | null): number | null {
  if (!value) return null;
  const at = new Date(value).getTime();
  if (!Number.isFinite(at)) return null;
  return Math.round((at - Date.now()) / 86_400_000);
}

function certTone(cert: MTLSCert): "ok" | "warn" | "danger" {
  if (cert.revokedAt) return "danger";
  const days = daysUntil(cert.expiresAt);
  if (days === null) return "warn";
  if (days <= 0) return "danger";
  if (days <= EXPIRY_WARN_DAYS) return "warn";
  return "ok";
}

function certStatusText(cert: MTLSCert): string {
  if (cert.revokedAt) return "Revoked";
  const days = daysUntil(cert.expiresAt);
  if (days === null) return "Expiry unknown";
  if (days <= 0) return "Expired";
  if (days <= EXPIRY_WARN_DAYS) return `Expiring in ${days}d`;
  return "Valid";
}

const PHASE_LABELS: Record<string, string> = {
  not_started: "Not started",
  ca_ready: "CA ready",
  partial: "In progress",
  complete: "Complete",
};

export default function AdminMTLSPage() {
  const [confirm, renderConfirm] = useConfirm();
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [showGenerateCA, setShowGenerateCA] = useState(false);
  const [caOrg, setCAOrg] = useState("Forge");
  const [caCN, setCACN] = useState("Forge mTLS CA");

  const statusQuery = useQuery({
    queryKey: ["mtls-status"],
    queryFn: () => fetchJSON<MTLSStatusResponse>("/mtls/status"),
    refetchInterval: 10_000,
  });

  const migrationQuery = useQuery({
    queryKey: ["mtls-migration-status"],
    queryFn: () => fetchJSON<MTLSMigrationStatusResponse>("/mtls/migration/status"),
    refetchInterval: 15_000,
  });

  const certsQuery = useQuery({
    queryKey: ["mtls-certs"],
    queryFn: () => fetchJSON<MTLSCertListResponse>("/mtls/certificates?limit=100"),
  });

  const caCertsQuery = useQuery({
    queryKey: ["mtls-certs-ca"],
    queryFn: () => fetchJSON<MTLSCertListResponse>("/mtls/certificates?type=ca"),
  });

  const generateCAMutation = useMutation({
    mutationFn: () => postJSON<MTLSCertResponse>("/mtls/certificates/generate-ca", { organization: caOrg.trim(), commonName: caCN.trim() }),
    onSuccess: () => {
      setShowGenerateCA(false);
      queryClient.invalidateQueries({ queryKey: ["mtls-status"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs-ca"] });
      toast({ tone: "success", title: "CA certificate generated" });
    },
    onError: (err) => toast({ tone: "error", title: "CA generation failed", message: errorMessage(err) }),
  });

  const revokeMutation = useMutation({
    mutationFn: async (id: string) => {
      const result = await postJSON<{ ok: boolean }>(`/mtls/certificates/${id}/revoke`);
      if (!result.ok) throw new Error("The server reported the revocation did not complete.");
      return result;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mtls-status"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs-ca"] });
    },
    onError: (err) => toast({ tone: "error", title: "Failed to revoke certificate", message: errorMessage(err) }),
  });

  const migrationMutation = useMutation({
    mutationFn: async () => {
      const result = await postJSON<{ ok: boolean }>("/mtls/migration/run");
      if (!result.ok) throw new Error("The server reported the migration did not complete.");
      return result;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mtls-migration-status"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-status"] });
      queryClient.invalidateQueries({ queryKey: ["mtls-certs"] });
    },
    onError: (err) => toast({ tone: "error", title: "Migration failed", message: errorMessage(err) }),
  });

  const status = statusQuery.data?.data;
  const migration = migrationQuery.data?.data;
  const migrationEnabled = migration?.migrationEnabled === true;
  const certs = Array.isArray(certsQuery.data?.data) ? certsQuery.data.data : [];
  const caCerts = Array.isArray(caCertsQuery.data?.data) ? caCertsQuery.data.data : [];
  const caExpiryDays = daysUntil(status?.caExpiresAt);

  const revokeCert = async (cert: MTLSCert, kind: string) => {
    const ok = await confirm({
      title: `Revoke ${kind}?`,
      description: `${cert.commonName} will stop being trusted immediately and cannot be un-revoked. Traffic that presents it — including a Beacon using its node identity — will fail mTLS until it is reissued.`,
      danger: true,
      confirmLabel: "Revoke",
    });
    if (ok) revokeMutation.mutate(cert.id);
  };

  const runMigration = async () => {
    const ok = await confirm({
      title: "Run the token-to-mTLS migration?",
      description: "The control plane will issue mTLS certificates for the nodes it can reach and record them. Existing token-authenticated identities are left in place, but node agents may be reconfigured as they pick up the new identity.",
      danger: true,
      confirmLabel: "Run migration",
    });
    if (ok) migrationMutation.mutate();
  };

  return (
    <AdminPageLayout>
      <SectionHeader
        action={
          <div className="flex items-center gap-2">
            <Btn tone="ghost" size="sm" onClick={() => void certsQuery.refetch()} loading={certsQuery.isFetching}>
              <RefreshCw size={14} /> Refresh
            </Btn>
            <Btn size="sm" onClick={() => setShowGenerateCA((v) => !v)}>
              <Plus size={14} /> Generate CA
            </Btn>
          </div>
        }
        status={<FreshnessBadge state={sourceState(certsQuery, 30_000)} />}
      />

      {showGenerateCA ? (
        <Card>
          <CardHeader title="Generate new CA certificate" icon={Plus} />
          <div className="space-y-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <Input label="Organization" value={caOrg} onChange={setCAOrg} placeholder="Forge" required />
              <Input label="Common name" value={caCN} onChange={setCACN} placeholder="Forge mTLS CA" required />
            </div>
            <p className="ui-hint">
              These strings are written into every certificate this CA issues, and the control plane cannot rename them afterwards. The migration
              runner uses its own built-in defaults, so check those before running it.
            </p>
            <div className="flex gap-2">
              <Btn
                onClick={() => {
                  void (async () => {
                    const ok = await confirm({
                      title: "Generate a new CA certificate?",
                      description: `A CA "${caCN.trim()}" for organization "${caOrg.trim()}" will be created and trusted by every node that enrolls from now on.`,
                      confirmLabel: "Generate",
                    });
                    if (ok) generateCAMutation.mutate();
                  })();
                }}
                disabled={generateCAMutation.isPending || !caOrg.trim() || !caCN.trim()}
                loading={generateCAMutation.isPending}
              >
                {generateCAMutation.isPending ? "Generating…" : "Generate"}
              </Btn>
              <Btn tone="ghost" onClick={() => setShowGenerateCA(false)}>Cancel</Btn>
            </div>
            {generateCAMutation.isError ? (
              <AdminErrorState message={errorMessage(generateCAMutation.error, "CA generation failed.")} retry={() => generateCAMutation.mutate()} />
            ) : null}
          </div>
        </Card>
      ) : null}

      {statusQuery.isLoading ? (
        <AdminLoadingState label="Loading mTLS status…" />
      ) : statusQuery.isError ? (
        <AdminErrorState message={errorMessage(statusQuery.error, "mTLS status could not be loaded.")} retry={() => void statusQuery.refetch()} />
      ) : status ? (
        <StatsRow
          items={[
            { label: "CA Configured", value: status.caConfigured ? "Yes" : "No", icon: status.caConfigured ? ShieldCheck : ShieldX, tone: status.caConfigured ? "ok" : "warn" },
            { label: "Server Certs", value: count(status.serverCertCount), icon: Shield },
            { label: "Client Certs", value: count(status.clientCertCount), icon: Shield },
            { label: "Active Certs", value: count(status.activeCerts), icon: CheckCircle, tone: "ok" },
            { label: "Revoked", value: count(status.revokedCount), icon: XCircle, tone: "danger" },
          ]}
        />
      ) : (
        <AdminErrorState message="The mTLS status endpoint returned no status. Nothing on this page is measured." retry={() => void statusQuery.refetch()} />
      )}

      {status?.caExpiresAt ? (
        <Card>
          <CardHeader title="CA certificate expiry" icon={AlertTriangle} />
          <div className="flex flex-wrap items-center gap-3">
            <Pill tone={caExpiryDays === null ? "unknown" : caExpiryDays <= 0 ? "danger" : caExpiryDays <= EXPIRY_WARN_DAYS ? "warn" : "ok"}>
              {caExpiryDays === null
                ? "Expiry unreadable"
                : caExpiryDays <= 0
                  ? "Expired"
                  : caExpiryDays <= EXPIRY_WARN_DAYS
                    ? `Expires in ${caExpiryDays} days`
                    : `Valid`}
            </Pill>
            <span className="text-xs text-text-subtle">
              Issued CA expires {formatDate(status.caExpiresAt, "Unknown")} — {caExpiryDays === null ? "remaining days unreadable" : `${caExpiryDays} days remaining`} (this device&apos;s clock).
            </span>
          </div>
        </Card>
      ) : null}

      <Card>
        <CardHeader title="CA certificates" icon={ShieldCheck} />
        {caCertsQuery.isLoading ? (
          <AdminLoadingRows rows={2} cols={5} label="Loading CA certificates" />
        ) : caCertsQuery.isError ? (
          <div className="p-4"><AdminErrorState message={errorMessage(caCertsQuery.error, "CA certificates could not be loaded.")} retry={() => void caCertsQuery.refetch()} /></div>
        ) : caCerts.length === 0 ? (
          <EmptyState icon={ShieldCheck} title="No CA certificate" message="No issuing certificate exists. Generate one to enable node mTLS enrolment." />
        ) : (
          <AdminTable label="CA certificates">
            <AdminTHead>
              <AdminTh>Common Name</AdminTh>
              <AdminTh>Organization</AdminTh>
              <AdminTh>Serial</AdminTh>
              <AdminTh>Expires</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {caCerts.map((cert) => (
                <AdminTr key={cert.id}>
                  <AdminTd title={cert.commonName}>{cert.commonName}</AdminTd>
                  <AdminTd>{cert.organization || "—"}</AdminTd>
                  <AdminTd className="font-mono">{cert.serialNumber ? `${cert.serialNumber.slice(0, 16)}…` : "—"}</AdminTd>
                  <AdminTd>{formatDate(cert.expiresAt, "Unknown")}</AdminTd>
                  <AdminTd><Pill tone={certTone(cert)}>{certStatusText(cert)}</Pill></AdminTd>
                  <AdminTd className="text-right">
                    <div className="flex justify-end">
                      <AdminIconButton label={`Revoke CA certificate ${cert.commonName}`} tone="danger" disabled={Boolean(cert.revokedAt)} onClick={() => void revokeCert(cert, "CA certificate")}>
                        <Trash2 size={14} />
                      </AdminIconButton>
                    </div>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      <Card>
        <CardHeader
          title={certsQuery.isSuccess ? `Issued certificates (${certs.length})` : "Issued certificates"}
          icon={Shield}
        />
        {certsQuery.isLoading ? (
          <AdminLoadingRows rows={3} cols={5} label="Loading certificates" />
        ) : certsQuery.isError ? (
          <div className="p-4"><AdminErrorState message={errorMessage(certsQuery.error, "Certificates could not be loaded.")} retry={() => void certsQuery.refetch()} /></div>
        ) : certs.length === 0 ? (
          <EmptyState icon={Shield} title="No certificates issued yet" message="The certificate list loaded and is empty. Generate a CA, then issue node certificates to enable mTLS." />
        ) : (
          <AdminTable label="Issued certificates">
            <AdminTHead>
              <AdminTh>Type</AdminTh>
              <AdminTh>Common Name</AdminTh>
              <AdminTh>Node</AdminTh>
              <AdminTh>Expires</AdminTh>
              <AdminTh>Status</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {certs.map((cert) => (
                <AdminTr key={cert.id}>
                  <AdminTd><Pill tone={cert.certType === "ca" ? "warn" : cert.certType === "server" ? "info" : "neutral"}>{cert.certType}</Pill></AdminTd>
                  <AdminTd title={cert.commonName}>{cert.commonName}</AdminTd>
                  <AdminTd className="font-mono">{cert.nodeId ? cert.nodeId.slice(0, 8) : "—"}</AdminTd>
                  <AdminTd>{formatDate(cert.expiresAt, "Unknown")}</AdminTd>
                  <AdminTd><Pill tone={certTone(cert)}>{certStatusText(cert)}</Pill></AdminTd>
                  <AdminTd className="text-right">
                    <div className="flex justify-end">
                      {cert.revokedAt ? (
                        <span className="t-meta">already revoked</span>
                      ) : (
                        <AdminIconButton label={`Revoke ${cert.certType} certificate ${cert.commonName}`} tone="danger" onClick={() => void revokeCert(cert, `${cert.certType} certificate`)}>
                          <Trash2 size={14} />
                        </AdminIconButton>
                      )}
                    </div>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
      </Card>

      <Card>
        <CardHeader title="Token-to-mTLS migration" icon={RefreshCw} />
        {migrationQuery.isLoading ? (
          <div className="p-4"><AdminLoadingState label="Loading migration status…" /></div>
        ) : migrationQuery.isError ? (
          <div className="p-4"><AdminErrorState message={errorMessage(migrationQuery.error, "Migration status could not be loaded.")} retry={() => void migrationQuery.refetch()} /></div>
        ) : (
          <div className="space-y-4 p-4">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
              <div className="ui-well p-4 text-center">
                <div className="t-readout">{typeof migration?.totalNodes === "number" ? migration.totalNodes : "Not reported"}</div>
                <div className="t-meta mt-1">Total nodes</div>
              </div>
              <div className="ui-well p-4 text-center">
                <div className="t-readout">{typeof migration?.nodesWithCerts === "number" ? migration.nodesWithCerts : "Not reported"}</div>
                <div className="t-meta mt-1">Nodes with certs</div>
              </div>
              <div className="ui-well p-4 text-center">
                <div className="t-title">{migration?.phase ? PHASE_LABELS[migration.phase] ?? migration.phase : "Not reported"}</div>
                <div className="t-meta mt-1">Phase</div>
              </div>
            </div>

            {migrationEnabled ? (
              migration?.phase === "complete" ? (
                <div className="ui-alert ui-alert-success">
                  <CheckCircle aria-hidden="true" size={14} className="mt-0.5 shrink-0" />
                  <span>Every reported node holds an mTLS certificate. Enforcement can be enabled for this fleet.</span>
                </div>
              ) : (
                <Btn onClick={() => void runMigration()} disabled={migrationMutation.isPending} loading={migrationMutation.isPending}>
                  <RefreshCw size={14} /> {migrationMutation.isPending ? "Migrating…" : "Run migration"}
                </Btn>
              )
            ) : (
              <div className="ui-alert ui-alert-warning">
                <AlertTriangle aria-hidden="true" size={14} className="mt-0.5 shrink-0" />
                <span>
                  The control plane reports <code className="ui-code-inline">migrationEnabled: false</code> — no migration service is wired, so this
                  action cannot run. Run Migration is disabled, not hidden.
                </span>
              </div>
            )}
            {migrationMutation.isError ? (
              <AdminErrorState message={errorMessage(migrationMutation.error, "The migration run failed.")} />
            ) : null}
          </div>
        )}
      </Card>

      {renderConfirm()}
    </AdminPageLayout>
  );
}
