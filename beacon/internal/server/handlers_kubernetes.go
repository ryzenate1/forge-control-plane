package server

import (
	"encoding/json"
	"io"
	"net/http"

	"gamepanel/beacon/internal/runtime"
)

// kubernetesProxy returns the KubernetesProxy capability of the current runtime,
// or writes an error response and returns nil. Every handler below calls this so
// a Docker-only node never fabricates cluster data.
func (s *Server) kubernetesProxy(w http.ResponseWriter) runtime.KubernetesProxy {
	proxy, ok := s.runtime.(runtime.KubernetesProxy)
	if !ok {
		http.Error(w, `{"error":"kubernetes proxy not available on this node (runtime is not kubernetes)"}`, http.StatusServiceUnavailable)
		return nil
	}
	return proxy
}

func (s *Server) handleKubernetesListPods(w http.ResponseWriter, r *http.Request) {
	proxy := s.kubernetesProxy(w)
	if proxy == nil {
		return
	}
	raw, err := proxy.ListPods(r.Context())
	if err != nil {
		http.Error(w, string(raw)+"\n"+err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

func (s *Server) handleKubernetesListDeployments(w http.ResponseWriter, r *http.Request) {
	proxy := s.kubernetesProxy(w)
	if proxy == nil {
		return
	}
	raw, err := proxy.ListDeployments(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

func (s *Server) handleKubernetesListServices(w http.ResponseWriter, r *http.Request) {
	proxy := s.kubernetesProxy(w)
	if proxy == nil {
		return
	}
	raw, err := proxy.ListServices(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

func (s *Server) handleKubernetesListEvents(w http.ResponseWriter, r *http.Request) {
	proxy := s.kubernetesProxy(w)
	if proxy == nil {
		return
	}
	raw, err := proxy.ListEvents(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

func (s *Server) handleKubernetesScaleDeployment(w http.ResponseWriter, r *http.Request) {
	proxy := s.kubernetesProxy(w)
	if proxy == nil {
		return
	}
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, `{"error":"deployment name required in path"}`, http.StatusBadRequest)
		return
	}
	var body struct {
		Replicas int32 `json:"replicas"`
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err := json.Unmarshal(raw, &body); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	if err := proxy.ScaleDeployment(r.Context(), name, body.Replicas); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "deployment": name, "replicas": body.Replicas})
}
