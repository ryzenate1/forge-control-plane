"use client";
import { useNodesQuery } from "@/lib/admin/telemetry";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderLock, Globe, Info, KeyRound, PenLine, RefreshCw, Save, Server, Settings, Shield, ShieldAlert } from "lucide-react";
import {
  fetchSFTPGlobalConfig,
  updateSFTPGlobalConfig,
  fetchSFTPNodeConfigs,
  fetchSFTPNodeConfig,
  updateSFTPNodeConfig,
  type SFTPGlobalConfig,
  type SFTPNodeConfig,
} from "@/lib/api/sftp";
import { ApiError } from "@/lib/api/http";
import { useToast } from "@/components/ui/toast";
import { useConfirm } from "@/components/ui/confirm-dialog";
import { OfflineBanner } from "@/components/shared/states-offline";
import { formatDate, errorMessage } from "@/lib/utils";
import {
  AdminErrorState,
  AdminLoadingState,
  AdminPageLayout,
  AdminTBody,
  AdminTable,
  AdminTd,
  AdminTh,
  AdminTHead,
  AdminTr,
  Btn,
  Card,
  CardHeader,
  EmptyState,
  Input,
  Modal,
  ModalFooter,
  Pill,
  SectionHeader,
  selectStyle,
} from "./admin-ui";

/**
 * SFTP settings are rows in the control-plane database. Nothing on this page
 * reads a node's heartbeat, listener or port, so every "enabled" here is a
 * stored preference — the labels and tones say that instead of borrowing the
 * health vocabulary.
 */
const NOT_MEASURED = "Configuration only — the panel does not probe this node's SFTP listener.";

const LOG_LEVELS = ["error", "warn", "info", "debug", "trace"];

type NumericForm = Record<string, string>;

function parseNumber(raw: string | undefined, label: string, errors: string[]): number | null {
  const text = (raw ?? "").trim();
  if (text === "") {
    errors.push(`${label} needs a number. Leaving it blank is not the same as setting 0.`);
    return null;
  }
  const value = Number(text);
  if (!Number.isFinite(value) || value < 0 || !Number.isInteger(value)) {
    errors.push(`${label} must be a whole number of 0 or more (got "${text}").`);
    return null;
  }
  return value;
}

/** Splitting a comma list keeps the field round-trippable without inventing values. */
function splitList(raw: string | undefined): string[] {
  return (raw ?? "").split(",").map((item) => item.trim()).filter(Boolean);
}

/** A very small CIDR/IP check: clearing this field widens access, so it must not accept noise. */
function validateAllowedIps(raw: string, errors: string[]): string[] {
  const items = splitList(raw);
  for (const item of items) {
    const v4 = /^(\d{1,3}\.){3}\d{1,3}(\/(3[0-2]|[12]?\d))?$/.test(item);
    const v6 = /^[0-9a-fA-F:]+(\/(1[0-2][0-9]|[0-9]{1,3}))?$/.test(item) && item.includes(":");
    if (!v4 && !v6) errors.push(`"${item}" is not an IPv4/IPv6 address or CIDR range.`);
  }
  return items;
}

function globalFormFrom(config: SFTPGlobalConfig): NumericForm {
  return {
    _enabled: String(config.enabled),
    defaultPort: String(config.defaultPort),
    defaultMaxConnections: String(config.defaultMaxConnections),
    defaultIdleTimeout: String(config.defaultIdleTimeout),
    defaultRateLimit: String(config.defaultRateLimit),
    logLevel: config.logLevel,
    allowedCiphers: (config.allowedCiphers ?? []).join(", "),
    allowedMACs: (config.allowedMACs ?? []).join(", "),
    allowedKexAlgos: (config.allowedKexAlgos ?? []).join(", "),
    hostKeyAlgorithms: (config.hostKeyAlgorithms ?? []).join(", "),
  };
}

function nodeFormFrom(config: SFTPNodeConfig): NumericForm {
  return {
    _enabled: String(config.enabled),
    _readOnly: String(config.readOnly),
    listenIP: config.listenIP ?? "",
    listenPort: String(config.listenPort),
    maxConnections: String(config.maxConnections),
    maxAuthAttempts: String(config.maxAuthAttempts),
    idleTimeout: String(config.idleTimeout),
    rateLimit: String(config.rateLimit),
    allowedIps: (config.allowedIps ?? []).join(", "),
    banner: config.banner ?? "",
    logLevel: config.logLevel,
  };
}

/**
 * An override that does not exist yet is a 404 from the API. Starting the form
 * from the current global defaults (rather than an error page) is the flow the
 * page advertises, so the create path opens.
 */
function nodeFormDefaults(global: SFTPGlobalConfig | null): NumericForm {
  return {
    _enabled: String(global?.enabled ?? false),
    _readOnly: "false",
    listenIP: "",
    listenPort: global ? String(global.defaultPort) : "",
    maxConnections: global ? String(global.defaultMaxConnections) : "",
    maxAuthAttempts: "",
    idleTimeout: global ? String(global.defaultIdleTimeout) : "",
    rateLimit: global ? String(global.defaultRateLimit) : "",
    allowedIps: "",
    banner: "",
    logLevel: global?.logLevel ?? "info",
  };
}

export function AdminSftp() {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [confirmAction, renderConfirm] = useConfirm();

  const globalQ = useQuery({ queryKey: ["admin-sftp-settings"], queryFn: fetchSFTPGlobalConfig });
  const nodesQ = useQuery({ queryKey: ["admin-sftp-nodes"], queryFn: fetchSFTPNodeConfigs });
  const allNodesQ = useNodesQuery();

  const [globalForm, setGlobalForm] = useState<NumericForm | null>(null);
  const [globalErrors, setGlobalErrors] = useState<string[]>([]);
  const [editing, setEditing] = useState<{ nodeId: string; mode: "create" | "edit" } | null>(null);

  useEffect(() => {
    if (globalQ.data && !globalForm) setGlobalForm(globalFormFrom(globalQ.data));
  }, [globalQ.data, globalForm]);

  const nodeNameMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const node of allNodesQ.data ?? []) map.set(node.id, node.name);
    return map;
  }, [allNodesQ.data]);

  const overrides = nodesQ.data ?? [];
  const allNodes = allNodesQ.data ?? [];
  const configuredIds = new Set(overrides.map((config) => config.nodeId));
  const missing = allNodes.filter((node) => !configuredIds.has(node.id));

  const saveGlobalMut = useMutation({
    mutationFn: async (config: SFTPGlobalConfig) => {
      const result = await updateSFTPGlobalConfig(config);
      if (!result?.ok) throw new Error("The server did not confirm the global SFTP settings update.");
      return result;
    },
    onSuccess: () => {
      setGlobalErrors([]);
      void qc.invalidateQueries({ queryKey: ["admin-sftp-settings"] });
      toast({ tone: "success", title: "Global SFTP settings saved" });
    },
    onError: (err: Error) => toast({ tone: "error", title: "Save failed", message: errorMessage(err, "The settings were not saved.") }),
  });

  const handleSaveGlobal = async () => {
    if (!globalForm) return;
    const errors: string[] = [];
    const defaultPort = parseNumber(globalForm.defaultPort, "Default port", errors);
    const defaultMaxConnections = parseNumber(globalForm.defaultMaxConnections, "Max connections", errors);
    const defaultIdleTimeout = parseNumber(globalForm.defaultIdleTimeout, "Idle timeout", errors);
    const defaultRateLimit = parseNumber(globalForm.defaultRateLimit, "Rate limit", errors);
    setGlobalErrors(errors);
    if (defaultPort === null || defaultMaxConnections === null || defaultIdleTimeout === null || defaultRateLimit === null) {
      setGlobalErrors(errors.length ? errors : ["Check the highlighted numbers before saving."]);
      toast({ tone: "error", title: "Check the SFTP settings", message: errors[0] ?? "A required number is missing." });
      return;
    }

    const enabled = globalForm._enabled === "true";
    const notes = [
      `Applies to every node without an override: port ${defaultPort}, ${defaultMaxConnections} connections, ${defaultIdleTimeout}s idle, rate limit ${defaultRateLimit === 0 ? "0 = unlimited" : `${defaultRateLimit} KB/s`}.`,
      defaultRateLimit === 0 ? "Bandwidth is not capped." : "",
      defaultMaxConnections === 0 ? "0 connections allowed — no node will accept an SFTP session on these defaults." : "",
      enabled ? "" : "SFTP is being switched off globally.",
    ].filter(Boolean).join(" ");

    const ok = await confirmAction({
      confirmLabel: "Save global settings",
      danger: true,
      description: `${notes} This is a fleet-wide change to a file-transfer service; the panel does not verify that any node is listening.`,
      title: enabled ? "Save global SFTP settings?" : "Disable SFTP for the whole fleet?",
    });
    if (!ok) return;

    saveGlobalMut.mutate({
      allowedCiphers: splitList(globalForm.allowedCiphers),
      allowedKexAlgos: splitList(globalForm.allowedKexAlgos),
      allowedMACs: splitList(globalForm.allowedMACs),
      defaultIdleTimeout,
      defaultMaxConnections,
      defaultPort,
      defaultRateLimit,
      enabled,
      hostKeyAlgorithms: splitList(globalForm.hostKeyAlgorithms),
      logLevel: globalForm.logLevel,
    });
  };

  const refetchAll = () => {
    void globalQ.refetch();
    void nodesQ.refetch();
    void allNodesQ.refetch();
  };

  return (
    <AdminPageLayout>
      {renderConfirm()}
      <OfflineBanner onRetry={refetchAll} />
      <SectionHeader
        info={{
          description: "SFTP settings are stored control-plane configuration: fleet defaults plus per-node overrides. They describe what the panel should tell nodes to do, not what any node is currently doing.",
          eyebrow: "Architecture & Semantics",
          sections: [
            {
              content:
                "The global form sets fleet defaults for ports, connection caps, timeouts and crypto. A node without an override inherits them; opening one for the first time starts from those defaults and persists a new override row on save.",
              icon: Globe,
              title: "Global vs per-node",
            },
            {
              content:
                "Every save asks for confirmation, because these rows change how a file-transfer service accepts connections. A blank number is rejected rather than written as 0, and 0 is meaningful: it means unlimited rate, or, for connection counts, that nothing is accepted.",
              icon: KeyRound,
              title: "Numbers and blanks",
            },
            {
              content: NOT_MEASURED,
              icon: Shield,
              title: "What 'enabled' means here",
            },
          ],
          title: "SFTP access",
          triggerLabel: "About SFTP",
        }}
        action={
          <Btn onClick={refetchAll} size="sm" tone="ghost">
            <RefreshCw size={14} /> Refresh
          </Btn>
        }
      />

      {/* Global */}
      <Card>
        <CardHeader
          action={
            <Btn disabled={!globalForm} loading={saveGlobalMut.isPending} onClick={() => void handleSaveGlobal()} size="sm" tone="primary">
              <Save size={12} /> Save
            </Btn>
          }
          icon={Settings}
          title="Global settings"
        />
        <p className="mb-4 flex items-center gap-1.5 text-xs text-text-muted">
          <Info aria-hidden="true" size={13} />
          {NOT_MEASURED}
          {globalForm ? (
            <>
              {" "}Current setting: <Pill tone="neutral">{globalForm._enabled === "true" ? "enabled in config" : "disabled in config"}</Pill>
            </>
          ) : null}
        </p>
        {globalQ.isPending ? (
          <AdminLoadingState label="Loading SFTP settings…" />
        ) : globalQ.isError ? (
          <div className="p-1"><AdminErrorState message={errorMessage(globalQ.error, "The global SFTP settings could not be loaded.")} retry={() => void globalQ.refetch()} /></div>
        ) : globalForm ? (
          <div className="space-y-4">
            {globalErrors.length ? <AdminErrorState message={`Fix before saving — ${globalErrors.join(" ")}`} /> : null}

            <label className="flex items-center gap-2 text-sm">
              <input
                checked={globalForm._enabled === "true"}
                className="accent-[var(--brand)]"
                onChange={(event) => setGlobalForm({ ...globalForm, _enabled: String(event.target.checked) })}
                type="checkbox"
              />
              <span className="font-medium">Globally enabled</span>
            </label>

            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <Input label="Default port" mono onChange={(value) => setGlobalForm({ ...globalForm, defaultPort: value })} type="number" value={globalForm.defaultPort} />
              <Input label="Max connections" mono onChange={(value) => setGlobalForm({ ...globalForm, defaultMaxConnections: value })} type="number" value={globalForm.defaultMaxConnections} />
              <Input label="Idle timeout (s)" mono onChange={(value) => setGlobalForm({ ...globalForm, defaultIdleTimeout: value })} type="number" value={globalForm.defaultIdleTimeout} />
              <Input label="Rate limit (KB/s, 0=unlimited)" mono onChange={(value) => setGlobalForm({ ...globalForm, defaultRateLimit: value })} type="number" value={globalForm.defaultRateLimit} />
            </div>
            {globalForm.defaultRateLimit.trim() === "0" || globalForm.defaultMaxConnections.trim() === "0" ? (
              <p className="flex items-start gap-1.5 text-xs text-warn">
                <ShieldAlert aria-hidden="true" className="mt-0.5" size={13} />
                {globalForm.defaultRateLimit.trim() === "0" ? "Rate limit 0 lifts the bandwidth cap for every node without an override. " : ""}
                {globalForm.defaultMaxConnections.trim() === "0" ? "Max connections 0 means no SFTP sessions are accepted on these defaults." : ""}
              </p>
            ) : null}

            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-sm">
                <span className="ui-label mb-1.5">Log level</span>
                <select
                  className={selectStyle}
                  onChange={(event) => setGlobalForm({ ...globalForm, logLevel: event.target.value })}
                  value={LOG_LEVELS.includes(globalForm.logLevel) ? globalForm.logLevel : "info"}
                >
                  {!LOG_LEVELS.includes(globalForm.logLevel) ? <option value={globalForm.logLevel}>{globalForm.logLevel} (unrecognised)</option> : null}
                  {LOG_LEVELS.map((level) => <option key={level} value={level}>{level}</option>)}
                </select>
              </label>
              <div className="rounded-lg border border-line bg-overlay-subtle p-3">
                <div className="text-meta uppercase tracking-wider text-text-subtle">Crypto</div>
                <div className="mt-1 text-xs leading-5 text-text-subtle">
                  Ciphers: {globalForm.allowedCiphers || "default (node-selected)"}<br />
                  MACs: {globalForm.allowedMACs || "default (node-selected)"}<br />
                  KEX: {globalForm.allowedKexAlgos || "default (node-selected)"}
                </div>
              </div>
            </div>

            <div className="grid gap-3">
              <Input label="Allowed ciphers (comma-separated, blank = node default)" mono onChange={(value) => setGlobalForm({ ...globalForm, allowedCiphers: value })} placeholder="aes128-ctr, aes256-gcm@openssh.com" value={globalForm.allowedCiphers} />
              <Input label="Allowed MACs (comma-separated)" mono onChange={(value) => setGlobalForm({ ...globalForm, allowedMACs: value })} placeholder="hmac-sha2-256, hmac-sha2-512" value={globalForm.allowedMACs} />
              <Input label="Allowed KEX (comma-separated)" mono onChange={(value) => setGlobalForm({ ...globalForm, allowedKexAlgos: value })} placeholder="curve25519-sha256, diffie-hellman-group16-sha512" value={globalForm.allowedKexAlgos} />
              <Input label="Host key algorithms (comma-separated)" mono onChange={(value) => setGlobalForm({ ...globalForm, hostKeyAlgorithms: value })} placeholder="ssh-ed25519, rsa-sha2-256" value={globalForm.hostKeyAlgorithms} />
            </div>

            {saveGlobalMut.isError ? <AdminErrorState message={errorMessage(saveGlobalMut.error, "The settings were not saved.")} /> : null}
          </div>
        ) : null}
      </Card>

      {/* Per-node overrides */}
      <Card>
        <CardHeader
          action={
            <span className="text-xs text-text-subtle">
              {nodesQ.isPending || allNodesQ.isPending
                ? "Counts unavailable while loading"
                : nodesQ.isError
                  ? "Overrides not read"
                  : `${overrides.length} override${overrides.length === 1 ? "" : "s"} of ${allNodes.length} node${allNodes.length === 1 ? "" : "s"}`}
            </span>
          }
          icon={FolderLock}
          title="Per-node overrides"
        />
        {nodesQ.isPending ? (
          <AdminLoadingState label="Loading node overrides…" />
        ) : nodesQ.isError ? (
          <div className="p-1">
            <AdminErrorState
              message={`${errorMessage(nodesQ.error, "The per-node SFTP overrides could not be loaded.")} Without this read the page cannot say which nodes have an override.`}
              retry={() => void nodesQ.refetch()}
            />
          </div>
        ) : overrides.length === 0 ? (
          <EmptyState
            icon={Server}
            message={`No override rows exist, so all ${allNodes.length} registered node${allNodes.length === 1 ? "" : "s"} use the global defaults.`}
            title="No per-node overrides"
          />
        ) : (
          <AdminTable label="Per-node SFTP overrides">
            <AdminTHead>
              <AdminTh>Node</AdminTh>
              <AdminTh>Config</AdminTh>
              <AdminTh>Listen</AdminTh>
              <AdminTh>Limits</AdminTh>
              <AdminTh>Policy</AdminTh>
              <AdminTh>Updated</AdminTh>
              <AdminTh className="text-right">Actions</AdminTh>
            </AdminTHead>
            <AdminTBody>
              {overrides.map((config) => (
                <AdminTr key={config.nodeId}>
                  <AdminTd>
                    <div className="font-medium text-text">{nodeNameMap.get(config.nodeId) ?? "Unknown node"}</div>
                    <div className="font-mono text-meta text-text-muted">{config.nodeId.slice(0, 12)}…</div>
                  </AdminTd>
                  <AdminTd>
                    <Pill tone="neutral">{config.enabled ? "enabled in config" : "disabled in config"}</Pill>
                  </AdminTd>
                  <AdminTd className="font-mono">
                    {config.listenIP ? `${config.listenIP}:${config.listenPort}` : `all addresses:${config.listenPort}`}
                  </AdminTd>
                  <AdminTd>
                    <dl className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs text-text-subtle">
                      <dt>connections</dt><dd className="font-mono">{config.maxConnections === 0 ? "0 (none accepted)" : config.maxConnections}</dd>
                      <dt>auth attempts</dt><dd className="font-mono">{config.maxAuthAttempts}</dd>
                      <dt>idle</dt><dd className="font-mono">{config.idleTimeout}s</dd>
                      <dt>rate</dt><dd className="font-mono">{config.rateLimit === 0 ? "unlimited" : `${config.rateLimit} KB/s`}</dd>
                    </dl>
                  </AdminTd>
                  <AdminTd>
                    <div className="flex flex-wrap gap-1.5">
                      <Pill tone="neutral">{config.readOnly ? "read-only" : "read-write"}</Pill>
                      <Pill tone="neutral" className="font-mono">{config.logLevel}</Pill>
                    </div>
                    <p className="mt-1 max-w-64 break-words text-xs text-text-muted">
                      {config.banner ? `Banner: ${config.banner}` : "No banner set"}
                    </p>
                  </AdminTd>
                  <AdminTd className="font-mono">{config.updatedAt ? formatDate(config.updatedAt, "Not recorded") : "Not recorded"}</AdminTd>
                  <AdminTd className="text-right">
                    <Btn ariaLabel={`Edit the SFTP override for ${nodeNameMap.get(config.nodeId) ?? config.nodeId}`} onClick={() => setEditing({ mode: "edit", nodeId: config.nodeId })} size="sm" tone="ghost">
                      <PenLine aria-hidden="true" size={12} /> Edit
                    </Btn>
                  </AdminTd>
                </AdminTr>
              ))}
            </AdminTBody>
          </AdminTable>
        )}
        <p className="mt-3 border-t border-line pt-3 text-xs leading-5 text-text-subtle">
          Nodes without an override inherit the global defaults. Configure one below to create an override row;
          the values are written only when the save is confirmed.
        </p>
      </Card>

      {/* Nodes that have no override row */}
      {nodesQ.isError ? null : allNodesQ.isPending || allNodes.length === 0 ? (
        allNodesQ.isError ? (
          <Card>
            <CardHeader icon={Server} title="Nodes" />
            <div className="p-4"><AdminErrorState message={errorMessage(allNodesQ.error, "The node list could not be loaded, so nodes without an override cannot be listed.")} retry={() => void allNodesQ.refetch()} /></div>
          </Card>
        ) : null
      ) : (
        <Card>
          <CardHeader icon={Server} title={`Nodes without override — ${missing.length} of ${allNodes.length} using global defaults`} />
          {missing.length === 0 ? (
            <div className="p-4"><EmptyState icon={Server} message="Every registered node has its own override row." title="No inherited nodes" /></div>
          ) : (
            <div className="flex flex-wrap gap-2 p-4">
              {missing.map((node) => (
                <span className="inline-flex items-center gap-2 rounded-full border border-line bg-overlay-subtle px-3 py-1 text-xs" key={node.id}>
                  <span className="font-mono text-text">{node.name}</span>
                  <Btn onClick={() => setEditing({ mode: "create", nodeId: node.id })} size="sm" tone="ghost">Configure</Btn>
                </span>
              ))}
            </div>
          )}
        </Card>
      )}

      {editing ? (
        <SftpNodeEditor
          global={globalQ.data ?? null}
          // Keyed so switching nodes can never carry node A's numbers onto node B.
          key={`${editing.mode}:${editing.nodeId}`}
          mode={editing.mode}
          nodeName={nodeNameMap.get(editing.nodeId) ?? `${editing.nodeId.slice(0, 12)}…`}
          nodeId={editing.nodeId}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </AdminPageLayout>
  );
}

function SftpNodeEditor({
  nodeId,
  nodeName,
  mode,
  global,
  onClose,
}: {
  nodeId: string;
  nodeName: string;
  mode: "create" | "edit";
  global: SFTPGlobalConfig | null;
  onClose: () => void;
}) {
  const { toast } = useToast();
  const qc = useQueryClient();
  const [confirmAction, renderConfirm] = useConfirm();
  const [form, setForm] = useState<NumericForm | null>(null);
  const [errors, setErrors] = useState<string[]>([]);
  const [notFound, setNotFound] = useState(mode === "create");

  const query = useQuery({
    queryKey: ["admin-sftp-node", nodeId],
    queryFn: () => fetchSFTPNodeConfig(nodeId),
    retry: false,
  });

  useEffect(() => {
    if (query.data) {
      setForm(nodeFormFrom(query.data));
      setNotFound(false);
    }
  }, [query.data]);

  useEffect(() => {
    // A node with no override yet is the state the "Configure" rows represent:
    // the 404 is not an error page, it is the create path opening from the
    // current global defaults.
    if (query.error instanceof ApiError && query.error.status === 404) {
      setNotFound(true);
      setForm((current) => current ?? nodeFormDefaults(global));
    }
  }, [query.error, global, nodeId]);

  const saveMut = useMutation({
    mutationFn: async (config: SFTPNodeConfig) => {
      const result = await updateSFTPNodeConfig(nodeId, config);
      if (!result?.ok) throw new Error("The server did not confirm the node override update.");
      return result;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["admin-sftp-nodes"] });
      void qc.invalidateQueries({ queryKey: ["admin-sftp-node", nodeId] });
      toast({ tone: "success", title: mode === "create" ? `Override created for ${nodeName}` : `Override saved for ${nodeName}` });
      onClose();
    },
    onError: (err: Error) => toast({ tone: "error", title: "Save failed", message: errorMessage(err, "The override was not saved.") }),
  });

  const handleSave = async () => {
    if (!form) return;
    const nextErrors: string[] = [];
    const listenPort = parseNumber(form.listenPort, "Listen port", nextErrors);
    const maxConnections = parseNumber(form.maxConnections, "Max connections", nextErrors);
    const maxAuthAttempts = parseNumber(form.maxAuthAttempts, "Max auth attempts", nextErrors);
    const idleTimeout = parseNumber(form.idleTimeout, "Idle timeout", nextErrors);
    const rateLimit = parseNumber(form.rateLimit, "Rate limit", nextErrors);
    const allowedIps = validateAllowedIps(form.allowedIps, nextErrors);
    if (listenPort === null || maxConnections === null || maxAuthAttempts === null || idleTimeout === null || rateLimit === null || nextErrors.length) {
      if (!nextErrors.length) nextErrors.push("Every number field needs a whole number of 0 or more.");
      setErrors(nextErrors);
      toast({ tone: "error", title: "Check the override", message: nextErrors[0] });
      return;
    }

    const clearedAllowList = allowedIps.length === 0;
    const ok = await confirmAction({
      confirmLabel: mode === "create" ? "Create override" : "Save override",
      danger: true,
      description: [
        `${mode === "create" ? "Creates" : "Replaces"} the SFTP override for ${nodeName}: port ${listenPort}, ${maxConnections} connections, ${maxAuthAttempts} auth attempts, ${idleTimeout}s idle, rate limit ${rateLimit === 0 ? "0 = unlimited" : `${rateLimit} KB/s`}.`,
        clearedAllowList ? "Allowed IPs is empty, so any source address may connect." : `Allowed sources: ${allowedIps.join(", ")}.`,
        form._enabled === "true" ? "" : "The override is being stored disabled.",
      ].filter(Boolean).join(" "),
      title: mode === "create" ? `Create an SFTP override for ${nodeName}?` : `Save the SFTP override for ${nodeName}?`,
    });
    if (!ok) return;

    saveMut.mutate({
      allowedIps,
      banner: form.banner.trim(),
      enabled: form._enabled === "true",
      idleTimeout,
      listenIP: form.listenIP.trim(),
      listenPort,
      logLevel: form.logLevel,
      maxAuthAttempts,
      maxConnections,
      nodeId,
      rateLimit,
      readOnly: form._readOnly === "true",
      updatedAt: "",
    });
  };

  const field = (key: string, value: string) => setForm((prev) => (prev ? { ...prev, [key]: value } : prev));
  const boolField = (key: string, checked: boolean) => field(key, String(checked));

  return (
    <Modal
      description={mode === "create"
        ? `No override row exists for ${nodeName} yet${global ? `; the form starts from the current global defaults.` : " and the global defaults are still loading, so every field must be filled in."}`
        : `Editing the stored override row for ${nodeName}.`}
      onClose={onClose}
      title={mode === "create" ? `Configure SFTP override — ${nodeName}` : `Edit SFTP override — ${nodeName}`}
      wide
    >
      {renderConfirm()}
      {query.isPending ? (
        <AdminLoadingState label="Loading node override…" />
      ) : query.isError && !notFound ? (
        <AdminErrorState
          message={`${errorMessage(query.error, "This node's SFTP override could not be loaded.")} Saving now would overwrite a configuration this page has not read.`}
          retry={() => void query.refetch()}
        />
      ) : form ? (
        <>
          <p className="mb-3 flex items-center gap-1.5 text-xs text-text-muted">
            <Info aria-hidden="true" size={13} />
            {notFound ? "No override stored yet — saving creates one. " : ""}{NOT_MEASURED}
          </p>
          {errors.length ? <AdminErrorState message={`Fix before saving — ${errors.join(" ")}`} /> : null}

          <div className="space-y-4">
            <label className="flex items-center gap-2 text-sm">
              <input checked={form._enabled === "true"} onChange={(event) => boolField("_enabled", event.target.checked)} type="checkbox" />
              <span className="font-medium">Enabled in this node's configuration</span>
            </label>

            <div className="grid gap-3 sm:grid-cols-2">
              <Input label="Listen IP (blank = all addresses)" mono onChange={(value) => field("listenIP", value)} placeholder="0.0.0.0" value={form.listenIP} />
              <Input label="Listen port" mono onChange={(value) => field("listenPort", value)} type="number" value={form.listenPort} />
              <Input label="Max connections (0 = none accepted)" mono onChange={(value) => field("maxConnections", value)} type="number" value={form.maxConnections} />
              <Input label="Max auth attempts" mono onChange={(value) => field("maxAuthAttempts", value)} type="number" value={form.maxAuthAttempts} />
              <Input label="Idle timeout (s)" mono onChange={(value) => field("idleTimeout", value)} type="number" value={form.idleTimeout} />
              <Input label="Rate limit (KB/s, 0 = unlimited)" mono onChange={(value) => field("rateLimit", value)} type="number" value={form.rateLimit} />
            </div>

            <label className="flex items-center gap-2 text-sm">
              <input checked={form._readOnly === "true"} onChange={(event) => boolField("_readOnly", event.target.checked)} type="checkbox" />
              <span>Read-only</span>
            </label>

            <label className="block text-sm">
              <span className="ui-label mb-1.5">Log level</span>
              <select className={selectStyle} onChange={(event) => field("logLevel", event.target.value)} value={LOG_LEVELS.includes(form.logLevel) ? form.logLevel : "info"}>
                {!LOG_LEVELS.includes(form.logLevel) ? <option value={form.logLevel}>{form.logLevel} (unrecognised)</option> : null}
                {LOG_LEVELS.map((level) => <option key={level} value={level}>{level}</option>)}
              </select>
            </label>

            <Input
              label="Allowed IPs (comma-separated IPs or CIDR ranges)"
              mono
              onChange={(value) => field("allowedIps", value)}
              placeholder="10.0.0.0/8, 192.168.1.10"
              value={form.allowedIps}
            />
            {form.allowedIps.trim() === "" ? (
              <p className="flex items-start gap-1.5 text-xs text-warn">
                <ShieldAlert aria-hidden="true" className="mt-0.5" size={13} />
                Empty allow list means any source address may connect to this node over SFTP.
              </p>
            ) : null}

            <Input label="Banner" onChange={(value) => field("banner", value)} placeholder="Welcome to Forge SFTP" value={form.banner} />

            {saveMut.isError ? <AdminErrorState message={errorMessage(saveMut.error, "The override was not saved.")} /> : null}
          </div>
          <ModalFooter
            confirmLabel={mode === "create" ? "Create override" : "Save override"}
            destructive
            disabled={saveMut.isPending}
            onCancel={onClose}
            onConfirm={() => void handleSave()}
          />
        </>
      ) : null}
    </Modal>
  );
}
