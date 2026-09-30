package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubernetesProxy is an optional capability implemented by KubernetesRuntime.
// It exposes read-only cluster inspection so the panel can render a dashboard
// without a direct kubeconfig on the API host.
type KubernetesProxy interface {
	ListPods(ctx context.Context) (json.RawMessage, error)
	ListDeployments(ctx context.Context) (json.RawMessage, error)
	ListServices(ctx context.Context) (json.RawMessage, error)
	ListEvents(ctx context.Context) (json.RawMessage, error)
	ScaleDeployment(ctx context.Context, name string, replicas int32) error
}

var _ KubernetesProxy = (*KubernetesRuntime)(nil)

func (r *KubernetesRuntime) ListPods(ctx context.Context) (json.RawMessage, error) {
	pods, err := r.client.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}
	type podInfo struct {
		Name      string    `json:"name"`
		Namespace string    `json:"namespace"`
		Status    string    `json:"status"`
		Node      string    `json:"node"`
		Restarts  int32     `json:"restarts"`
		CreatedAt time.Time `json:"createdAt"`
	}
	out := make([]podInfo, 0, len(pods.Items))
	for _, p := range pods.Items {
		restarts := int32(0)
		for _, cs := range p.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		out = append(out, podInfo{
			Name:      p.Name,
			Namespace: p.Namespace,
			Status:    string(p.Status.Phase),
			Node:      p.Spec.NodeName,
			Restarts:  restarts,
			CreatedAt: p.CreationTimestamp.Time,
		})
	}
	return json.Marshal(out)
}

func (r *KubernetesRuntime) ListDeployments(ctx context.Context) (json.RawMessage, error) {
	deps, err := r.client.AppsV1().Deployments(r.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	type depInfo struct {
		Name         string `json:"name"`
		Namespace    string `json:"namespace"`
		Replicas     int32  `json:"replicas"`
		ReadyReplicas int32 `json:"readyReplicas"`
		Image        string `json:"image"`
	}
	out := make([]depInfo, 0, len(deps.Items))
	for _, d := range deps.Items {
		image := ""
		if len(d.Spec.Template.Spec.Containers) > 0 {
			image = d.Spec.Template.Spec.Containers[0].Image
		}
		out = append(out, depInfo{
			Name:          d.Name,
			Namespace:     d.Namespace,
			Replicas:      d.Status.Replicas,
			ReadyReplicas: d.Status.ReadyReplicas,
			Image:         image,
		})
	}
	return json.Marshal(out)
}

func (r *KubernetesRuntime) ListServices(ctx context.Context) (json.RawMessage, error) {
	svcs, err := r.client.CoreV1().Services(r.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	type svcInfo struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Type      string `json:"type"`
		ClusterIP string `json:"clusterIp"`
		ExternalIP string `json:"externalIp"`
		Port      int32  `json:"port"`
	}
	out := make([]svcInfo, 0, len(svcs.Items))
	for _, s := range svcs.Items {
		port := int32(0)
		if len(s.Spec.Ports) > 0 {
			port = s.Spec.Ports[0].Port
		}
		externalIP := ""
		if len(s.Status.LoadBalancer.Ingress) > 0 {
			externalIP = s.Status.LoadBalancer.Ingress[0].IP
		} else if len(s.Spec.ExternalIPs) > 0 {
			externalIP = s.Spec.ExternalIPs[0]
		}
		out = append(out, svcInfo{
			Name:       s.Name,
			Namespace:  s.Namespace,
			Type:       string(s.Spec.Type),
			ClusterIP:  s.Spec.ClusterIP,
			ExternalIP: externalIP,
			Port:       port,
		})
	}
	return json.Marshal(out)
}

func (r *KubernetesRuntime) ListEvents(ctx context.Context) (json.RawMessage, error) {
	events, err := r.client.CoreV1().Events(r.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	type eventInfo struct {
		Type    string `json:"type"`
		Reason  string `json:"reason"`
		Object  string `json:"object"`
		Message string `json:"message"`
		Count   int32  `json:"count"`
	}
	out := make([]eventInfo, 0, len(events.Items))
	for _, e := range events.Items {
		out = append(out, eventInfo{
			Type:    e.Type,
			Reason:  e.Reason,
			Object:  fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name),
			Message: e.Message,
			Count:   e.Count,
		})
	}
	return json.Marshal(out)
}

func (r *KubernetesRuntime) ScaleDeployment(ctx context.Context, name string, replicas int32) error {
	dep, err := r.client.AppsV1().Deployments(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment %q: %w", name, err)
	}
	want := replicas
	dep.Spec.Replicas = &want
	_, err = r.client.AppsV1().Deployments(r.namespace).Update(ctx, dep, metav1.UpdateOptions{})
	return err
}

// unused import guard — compile-time check that corev1 is used
var _ = corev1.PodPending
