"use client";

import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Plus, X } from "lucide-react";
import { createContainer, type CreateContainerRequest } from "@/lib/api/docker";
import { Btn, Input, Modal, ModalFooter } from "@/components/admin/admin-ui";
import { Alert } from "@/components/ui/primitives";

type PortMapping = { hostPort: string; containerPort: string; protocol: string };
type EnvVar = { key: string; value: string };
type VolumeBinding = { hostPath: string; containerPath: string; readOnly: boolean };

export function ContainerCreateModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [image, setImage] = useState("");
  const [name, setName] = useState("");
  const [network, setNetwork] = useState("bridge");
  const [restartPolicy, setRestartPolicy] = useState("always");
  const [ports, setPorts] = useState<PortMapping[]>([{ hostPort: "", containerPort: "", protocol: "tcp" }]);
  const [envVars, setEnvVars] = useState<EnvVar[]>([{ key: "", value: "" }]);
  const [volumes, setVolumes] = useState<VolumeBinding[]>([{ hostPath: "", containerPath: "", readOnly: false }]);
  const [error, setError] = useState("");

  const createMut = useMutation({
    mutationFn: () => {
      const config: CreateContainerRequest = {
        image,
      };
      if (name) config.name = name;
      if (network) config.network = network;
      if (restartPolicy) config.restartPolicy = restartPolicy;

      const portBindings = ports
        .filter((p) => p.hostPort && p.containerPort)
        .map((p) => ({
          hostPort: parseInt(p.hostPort, 10),
          containerPort: parseInt(p.containerPort, 10),
          protocol: p.protocol,
        }));
      if (portBindings.length > 0) config.ports = portBindings;

      const envRecord: Record<string, string> = {};
      for (const e of envVars) {
        if (e.key) envRecord[e.key] = e.value;
      }
      if (Object.keys(envRecord).length > 0) config.env = envRecord;

      const binds = volumes
        .filter((v) => v.hostPath && v.containerPath)
        .map((v) => ({
          hostPath: v.hostPath,
          containerPath: v.containerPath,
          readOnly: v.readOnly,
        }));
      if (binds.length > 0) config.volumes = binds;

      return createContainer(config);
    },
    onSuccess: () => onCreated(),
    onError: (err: Error) => setError(err.message),
  });

  const addPort = () => setPorts([...ports, { hostPort: "", containerPort: "", protocol: "tcp" }]);
  const removePort = (i: number) => setPorts(ports.filter((_, idx) => idx !== i));
  const updatePort = (i: number, field: keyof PortMapping, value: string) => {
    const updated = ports.map((p, idx) => (idx === i ? { ...p, [field]: value } : p));
    setPorts(updated);
  };

  const addEnv = () => setEnvVars([...envVars, { key: "", value: "" }]);
  const removeEnv = (i: number) => setEnvVars(envVars.filter((_, idx) => idx !== i));
  const updateEnv = (i: number, field: keyof EnvVar, value: string) => {
    const updated = envVars.map((e, idx) => (idx === i ? { ...e, [field]: value } : e));
    setEnvVars(updated);
  };

  const addVolume = () => setVolumes([...volumes, { hostPath: "", containerPath: "", readOnly: false }]);
  const removeVolume = (i: number) => setVolumes(volumes.filter((_, idx) => idx !== i));
  const updateVolume = (i: number, field: keyof VolumeBinding, value: string | boolean) => {
    const updated = volumes.map((v, idx) => (idx === i ? { ...v, [field]: value } : v));
    setVolumes(updated);
  };

  return (
    <Modal onClose={onClose} title="Create Container" wide>
      <div className="space-y-5">
        {error && <Alert tone="error" title="Failed to create container">{error}</Alert>}

        <div className="grid gap-4 sm:grid-cols-2">
          <Input label="Image *" placeholder="nginx:latest" value={image} onChange={setImage} />
          <Input label="Container Name" placeholder="my-container" value={name} onChange={setName} />
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1.5 block text-sm font-medium text-slate-300">Network</label>
            <select className="ui-input" value={network} onChange={(e) => setNetwork(e.target.value)}>
              <option value="bridge">bridge</option>
              <option value="host">host</option>
              <option value="none">none</option>
            </select>
          </div>
          <div>
            <label className="mb-1.5 block text-sm font-medium text-slate-300">Restart Policy</label>
            <select className="ui-input" value={restartPolicy} onChange={(e) => setRestartPolicy(e.target.value)}>
              <option value="no">no</option>
              <option value="always">always</option>
              <option value="on-failure">on-failure</option>
              <option value="unless-stopped">unless-stopped</option>
            </select>
          </div>
        </div>

        {/* Port Mappings */}
        <div>
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-medium text-slate-300">Port Mappings</span>
            <Btn size="sm" tone="ghost" onClick={addPort}><Plus size={12} /> Add Port</Btn>
          </div>
          <div className="space-y-2">
            {ports.map((p, i) => (
              <div key={i} className="flex items-center gap-2">
                <input className="ui-input w-24" placeholder="Host" value={p.hostPort} onChange={(e) => updatePort(i, "hostPort", e.target.value)} type="number" />
                <span className="text-slate-500">:</span>
                <input className="ui-input w-24" placeholder="Container" value={p.containerPort} onChange={(e) => updatePort(i, "containerPort", e.target.value)} type="number" />
                <select className="ui-input w-24" value={p.protocol} onChange={(e) => updatePort(i, "protocol", e.target.value)}>
                  <option value="tcp">tcp</option>
                  <option value="udp">udp</option>
                </select>
                {ports.length > 1 && (
                  <button className="rounded p-1 text-slate-500 hover:text-red-400" onClick={() => removePort(i)} type="button"><X size={14} /></button>
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Environment Variables */}
        <div>
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-medium text-slate-300">Environment Variables</span>
            <Btn size="sm" tone="ghost" onClick={addEnv}><Plus size={12} /> Add Variable</Btn>
          </div>
          <div className="space-y-2">
            {envVars.map((e, i) => (
              <div key={i} className="flex items-center gap-2">
                <input className="ui-input flex-1" placeholder="KEY" value={e.key} onChange={(e2) => updateEnv(i, "key", e2.target.value)} />
                <input className="ui-input flex-1" placeholder="VALUE" value={e.value} onChange={(e2) => updateEnv(i, "value", e2.target.value)} />
                {envVars.length > 1 && (
                  <button className="rounded p-1 text-slate-500 hover:text-red-400" onClick={() => removeEnv(i)} type="button"><X size={14} /></button>
                )}
              </div>
            ))}
          </div>
        </div>

        {/* Volumes */}
        <div>
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-medium text-slate-300">Volumes</span>
            <Btn size="sm" tone="ghost" onClick={addVolume}><Plus size={12} /> Add Volume</Btn>
          </div>
          <div className="space-y-2">
            {volumes.map((v, i) => (
              <div key={i} className="flex items-center gap-2">
                <input className="ui-input flex-1" placeholder="Host Path" value={v.hostPath} onChange={(e) => updateVolume(i, "hostPath", e.target.value)} />
                <input className="ui-input flex-1" placeholder="Container Path" value={v.containerPath} onChange={(e) => updateVolume(i, "containerPath", e.target.value)} />
                <label className="flex items-center gap-1 text-xs text-slate-400">
                  <input checked={v.readOnly} onChange={(e) => updateVolume(i, "readOnly", e.target.checked)} type="checkbox" />
                  RO
                </label>
                {volumes.length > 1 && (
                  <button className="rounded p-1 text-slate-500 hover:text-red-400" onClick={() => removeVolume(i)} type="button"><X size={14} /></button>
                )}
              </div>
            ))}
          </div>
        </div>

        <ModalFooter
          onCancel={onClose}
          onConfirm={() => createMut.mutate()}
          confirmLabel={createMut.isPending ? "Creating..." : "Create"}
          disabled={!image || createMut.isPending}
        />
      </div>
    </Modal>
  );
}
