import { fetchJSON, postJSON } from "./http";

export type KubernetesNode = {
  id: string;
  name: string;
  baseUrl: string;
  runtimeProvider: string;
  runtimeStatus?: string;
  status: string;
  regionId?: string;
};

export type K8sPod = {
  name: string;
  namespace: string;
  status: string;
  podIp?: string;
  nodeName?: string;
  restartCount: number;
  labels?: Record<string, string>;
  containers: string[];
  createdAt: string;
};

export type K8sDeployment = {
  name: string;
  namespace: string;
  replicas: number;
  readyReplicas: number;
  available: number;
  image: string;
  labels?: Record<string, string>;
  createdAt: string;
};

export type K8sService = {
  name: string;
  namespace: string;
  type: string;
  clusterIp: string;
  ports: number[];
  labels?: Record<string, string>;
};

export type K8sEvent = {
  reason: string;
  message: string;
  type: string;
  involved: string;
  count: number;
  lastSeen: string;
  firstSeen: string;
};

function q(nodeId?: string) {
  return nodeId ? `?nodeId=${encodeURIComponent(nodeId)}` : "";
}

export function fetchKubernetesNodes() {
  return fetchJSON<{ nodes: KubernetesNode[] }>("/admin/kubernetes/nodes").then((r) => r.nodes);
}

export function fetchK8sPods(nodeId?: string) {
  return fetchJSON<{ pods: K8sPod[] }>(`/admin/kubernetes/pods${q(nodeId)}`).then((r) => r.pods);
}

export function fetchK8sDeployments(nodeId?: string) {
  return fetchJSON<{ deployments: K8sDeployment[] }>(`/admin/kubernetes/deployments${q(nodeId)}`).then((r) => r.deployments);
}

export function fetchK8sServices(nodeId?: string) {
  return fetchJSON<{ services: K8sService[] }>(`/admin/kubernetes/services${q(nodeId)}`).then((r) => r.services);
}

export function fetchK8sEvents(nodeId?: string) {
  return fetchJSON<{ events: K8sEvent[] }>(`/admin/kubernetes/events${q(nodeId)}`).then((r) => r.events);
}

export function scaleK8sDeployment(name: string, replicas: number, nodeId?: string) {
  return postJSON<{ ok: boolean }>(`/admin/kubernetes/deployments/${encodeURIComponent(name)}/scale${q(nodeId)}`, { replicas });
}
