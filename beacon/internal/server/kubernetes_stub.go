package server

import "net/http"

// Stub Kubernetes handlers to satisfy server.go references when the
// full Kubernetes integration is not compiled. They return 501 Not Implemented.
func (s *Server) handleKubernetesPods(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "kubernetes not implemented", http.StatusNotImplemented)
}
func (s *Server) handleKubernetesDeployments(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "kubernetes not implemented", http.StatusNotImplemented)
}
func (s *Server) handleKubernetesServices(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "kubernetes not implemented", http.StatusNotImplemented)
}
func (s *Server) handleKubernetesEvents(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "kubernetes not implemented", http.StatusNotImplemented)
}
func (s *Server) handleKubernetesScale(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "kubernetes not implemented", http.StatusNotImplemented)
}
