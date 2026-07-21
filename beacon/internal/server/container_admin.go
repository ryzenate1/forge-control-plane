package server

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"gamepanel/beacon/internal/remote"
)

func (s *Server) handleContainerList(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	// Check if user has admin scope
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:list", "container", nil, map[string]interface{}{"all": r.URL.Query().Get("all")})

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	all := r.URL.Query().Get("all") == "true"
	containers, err := docker.ContainerList(r.Context(), container.ListOptions{All: all})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter out managed containers for non-infra admins
	filteredContainers := make([]types.Container, 0, len(containers))
	for _, c := range containers {
		if userInfo.IsInfraAdmin || c.Labels["modern-game-panel.server_id"] == "" {
			filteredContainers = append(filteredContainers, c)
		}
	}

	type containerSummary struct {
		ID      string             `json:"id"`
		Names   []string           `json:"names"`
		Image   string             `json:"image"`
		ImageID string             `json:"imageId"`
		Created int64              `json:"created"`
		State   string             `json:"state"`
		Status  string             `json:"status"`
		Ports   []types.Port       `json:"ports"`
		Labels  map[string]string  `json:"labels"`
		Mounts  []types.MountPoint `json:"mounts"`
		Managed bool               `json:"managed"`
	}
	result := make([]containerSummary, 0, len(filteredContainers))
	for _, c := range filteredContainers {
		managed := c.Labels["modern-game-panel.server_id"] != ""
		result = append(result, containerSummary{
			ID: c.ID, Names: c.Names, Image: c.Image, ImageID: c.ImageID,
			Created: c.Created, State: c.State, Status: c.Status,
			Ports: c.Ports, Labels: c.Labels, Mounts: c.Mounts,
			Managed: managed,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	inspect, err := docker.ContainerInspect(r.Context(), id)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if this is a managed container and user is not infra admin
	if !userInfo.IsInfraAdmin && inspect.Config != nil && inspect.Config.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot inspect managed containers")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:inspect", "container", &id, map[string]interface{}{"name": inspect.Name})

	// Filter sensitive data from response
	filteredInspect := filterContainerInspect(inspect)
	writeJSON(w, http.StatusOK, filteredInspect)
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	// Check if container is managed and user is not infra admin
	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Quick check if container exists and is managed
	inspect, inspectErr := docker.ContainerInspect(r.Context(), id)
	if inspectErr == nil && inspect.Config != nil && !userInfo.IsInfraAdmin && inspect.Config.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot view logs of managed containers")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:logs", "container", &id, map[string]interface{}{"tail": r.URL.Query().Get("tail")})

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}
	reader, err := docker.ContainerLogs(r.Context(), id, container.LogsOptions{
		ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: tail,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, io.LimitReader(reader, 512*1024))
}

func (s *Server) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	s.adminContainerAction(w, r, "start")
}
func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	s.adminContainerAction(w, r, "stop")
}
func (s *Server) handleContainerRestart(w http.ResponseWriter, r *http.Request) {
	s.adminContainerAction(w, r, "restart")
}

func (s *Server) adminContainerAction(w http.ResponseWriter, r *http.Request, action string) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if container is managed and user is not infra admin
	inspect, inspectErr := docker.ContainerInspect(r.Context(), id)
	if inspectErr == nil && inspect.Config != nil && !userInfo.IsInfraAdmin && inspect.Config.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot control managed containers")
		return
	}

	// Check if container is running before stopping/restarting
	if action == "stop" || action == "restart" {
		if inspectErr == nil && !inspect.State.Running {
			writeError(w, http.StatusConflict, "container is not running")
			return
		}
	}

	// Check if container is already running before starting
	if action == "start" {
		if inspectErr == nil && inspect.State.Running {
			writeError(w, http.StatusConflict, "container is already running")
			return
		}
	}

	switch action {
	case "start":
		err = docker.ContainerStart(r.Context(), id, container.StartOptions{})
	case "stop":
		timeout := 30
		err = docker.ContainerStop(r.Context(), id, container.StopOptions{Timeout: &timeout})
	case "restart":
		timeout := 30
		err = docker.ContainerRestart(r.Context(), id, container.StopOptions{Timeout: &timeout})
	}
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:"+action, "container", &id, map[string]interface{}{"status": "success"})

	writeJSON(w, http.StatusOK, map[string]any{"id": id, "action": action, "status": "ok"})
}

func (s *Server) handleContainerDelete(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	// Check for explicit confirmation header
	if r.Header.Get("X-Confirm-Destructive") != "true" {
		writeError(w, http.StatusBadRequest, "destructive operation requires X-Confirm-Destructive: true header")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	inspect, err := docker.ContainerInspect(r.Context(), id)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if container is managed
	if inspect.Config != nil && inspect.Config.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot delete managed containers through this endpoint")
		return
	}

	// Check if container is running - require force flag
	if inspect.State.Running {
		force := r.URL.Query().Get("force") == "true"
		if !force {
			writeError(w, http.StatusConflict, "container is running, use force=true to delete")
			return
		}
	}

	// Check if container has dependent containers
	if len(inspect.Mounts) > 0 {
		removeVolumes := r.URL.Query().Get("v") == "true"
		if !removeVolumes {
			writeError(w, http.StatusConflict, "container has mounts, use v=true to remove volumes")
			return
		}
	}

	force := r.URL.Query().Get("force") == "true"
	removeVolumes := r.URL.Query().Get("v") == "true"

	err = docker.ContainerRemove(r.Context(), id, container.RemoveOptions{Force: force, RemoveVolumes: removeVolumes})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Audit log
	containerName := ""
	if inspect.Name != "" {
		containerName = inspect.Name
	}
	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:delete", "container", &id, map[string]interface{}{
		"name":        containerName,
		"force":       fmt.Sprintf("%t", force),
		"remove_vols": fmt.Sprintf("%t", removeVolumes),
	})

	writeJSON(w, http.StatusOK, map[string]any{"id": id, "action": "delete", "status": "ok"})
}

func (s *Server) handleImageList(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "image:list", "image", nil, map[string]interface{}{"all": "true"})

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	images, err := docker.ImageList(r.Context(), image.ListOptions{All: true})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type imageSummary struct {
		ID         string            `json:"id"`
		RepoTags   []string          `json:"repoTags"`
		Created    int64             `json:"created"`
		Size       int64             `json:"size"`
		Labels     map[string]string `json:"labels"`
		Containers int64             `json:"containers"`
		Managed    bool              `json:"managed"`
	}
	result := make([]imageSummary, 0, len(images))
	for _, img := range images {
		managed := img.Labels["modern-game-panel.server_id"] != ""
		result = append(result, imageSummary{
			ID: img.ID, RepoTags: img.RepoTags, Created: img.Created,
			Size: img.Size, Labels: img.Labels, Containers: img.Containers,
			Managed: managed,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleImagePull(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var body struct {
		Image        string `json:"image"`
		RegistryAuth string `json:"registryAuth,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}

	// Validate image name (basic security check)
	if strings.Contains(body.Image, "..") || strings.HasPrefix(body.Image, "/") {
		writeError(w, http.StatusBadRequest, "invalid image name")
		return
	}

	opts := image.PullOptions{}
	if body.RegistryAuth != "" {
		opts.RegistryAuth = body.RegistryAuth
	}

	pull, err := docker.ImagePull(r.Context(), body.Image, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer pull.Close()
	_, _ = io.Copy(io.Discard, pull)

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "image:pull", "image", &body.Image, map[string]interface{}{
		"registry_auth": fmt.Sprintf("%t", body.RegistryAuth != ""),
	})

	writeJSON(w, http.StatusOK, map[string]any{"image": body.Image, "status": "pulled"})
}

func (s *Server) handleImageDelete(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	// Check for explicit confirmation header
	if r.Header.Get("X-Confirm-Destructive") != "true" {
		writeError(w, http.StatusBadRequest, "destructive operation requires X-Confirm-Destructive: true header")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "image id is required")
		return
	}

	// Check if image is in use by containers
	images, err := docker.ImageList(r.Context(), image.ListOptions{All: true})
	if err == nil {
		for _, img := range images {
			if img.ID == id {
				if img.Containers > 0 {
					writeError(w, http.StatusConflict, "image is in use by containers, use force=true to remove")
					return
				}
				break
			}
		}
	}

	force := r.URL.Query().Get("force") == "true"
	_, err = docker.ImageRemove(r.Context(), id, image.RemoveOptions{Force: force, PruneChildren: true})
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "image:delete", "image", &id, map[string]interface{}{
		"force": fmt.Sprintf("%t", force),
	})

	writeJSON(w, http.StatusOK, map[string]any{"image": id, "status": "removed"})
}

func (s *Server) handleImagePrune(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	// Check for explicit confirmation header for destructive operation
	if r.Header.Get("X-Confirm-Destructive") != "true" {
		writeError(w, http.StatusBadRequest, "destructive operation requires X-Confirm-Destructive: true header")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	report, err := docker.ImagesPrune(r.Context(), filters.NewArgs())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "image:prune", "image", nil, map[string]interface{}{
		"space_reclaimed": fmt.Sprintf("%d", report.SpaceReclaimed),
	})

	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleNetworkList(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "network:list", "network", nil, nil)

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	networks, err := docker.NetworkList(r.Context(), network.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter out platform networks for non-infra admins
	filteredNetworks := make([]network.Summary, 0, len(networks))
	for _, net := range networks {
		if userInfo.IsInfraAdmin || !isPlatformNetwork(net.Name) {
			filteredNetworks = append(filteredNetworks, net)
		}
	}

	writeJSON(w, http.StatusOK, filteredNetworks)
}

func (s *Server) handleNetworkInspect(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "network id is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	net, err := docker.NetworkInspect(r.Context(), id, network.InspectOptions{Verbose: true})
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "network not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if this is a platform network and user is not infra admin
	if !userInfo.IsInfraAdmin && isPlatformNetwork(net.Name) {
		writeError(w, http.StatusForbidden, "cannot inspect platform networks")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "network:inspect", "network", &id, map[string]interface{}{"name": net.Name})

	// Filter sensitive data
	filteredNet := filterNetworkInspect(net)
	writeJSON(w, http.StatusOK, filteredNet)
}

func (s *Server) handleVolumeList(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "volume:list", "volume", nil, nil)

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	vols, err := docker.VolumeList(r.Context(), volume.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type volumeEntry struct {
		Name       string                 `json:"name"`
		Driver     string                 `json:"driver"`
		Mountpoint string                 `json:"mountpoint"`
		CreatedAt  string                 `json:"createdAt"`
		Status     map[string]interface{} `json:"status"`
		Labels     map[string]string      `json:"labels"`
		Scope      string                 `json:"scope"`
		Managed    bool                   `json:"managed"`
	}
	result := make([]volumeEntry, 0, len(vols.Volumes))
	for _, v := range vols.Volumes {
		managed := v.Labels["modern-game-panel.server_id"] != ""
		result = append(result, volumeEntry{
			Name: v.Name, Driver: v.Driver, Mountpoint: v.Mountpoint,
			CreatedAt: v.CreatedAt, Status: v.Status, Labels: v.Labels, Scope: v.Scope,
			Managed: managed,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"volumes": result, "warnings": vols.Warnings})
}

func (s *Server) handleVolumeInspect(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "volume name is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	v, err := docker.VolumeInspect(r.Context(), id)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "volume not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if this is a managed volume and user is not infra admin
	if !userInfo.IsInfraAdmin && v.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot inspect managed volumes")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "volume:inspect", "volume", &id, map[string]interface{}{"name": v.Name})

	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleVolumeUsage(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "volume:usage", "volume", nil, nil)

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	du, err := docker.DiskUsage(r.Context(), types.DiskUsageOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type volumeUsage struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		RefCount int64  `json:"refCount"`
	}
	usages := make([]volumeUsage, 0, len(du.Volumes))
	for _, v := range du.Volumes {
		usages = append(usages, volumeUsage{
			Name: v.Name, Size: v.UsageData.Size, RefCount: v.UsageData.RefCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"volumes": usages, "layersSize": du.LayersSize})
}

func (s *Server) handleContainerExec(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization - exec requires infra admin
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsInfraAdmin {
		writeError(w, http.StatusForbidden, "infra admin access required for exec")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	// Validate command (basic security check)
	var body struct {
		Cmd          []string `json:"cmd"`
		AttachStdout bool     `json:"attachStdout"`
		AttachStderr bool     `json:"attachStderr"`
		Tty          bool     `json:"tty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Cmd) == 0 {
		writeError(w, http.StatusBadRequest, "cmd is required")
		return
	}

	// Allowlist of safe commands — only these may be used
	allowedCommands := map[string]bool{
		"ls": true, "cat": true, "echo": true, "pwd": true, "ps": true,
		"top": true, "df": true, "du": true, "free": true, "uname": true,
		"whoami": true, "id": true, "env": true, "head": true, "tail": true,
		"grep": true, "find": true, "stat": true, "file": true, "tree": true,
		"readlink": true, "basename": true, "dirname": true, "sort": true,
		"wc": true, "cut": true, "tr": true, "tee": true, "date": true,
		"cal": true, "bc": true, "expr": true, "test": true, "true": true,
		"false": true, "sleep": true, "uptime": true, "dmesg": true,
		"lscpu": true, "lsblk": true, "lspci": true, "lsusb": true,
		"lsof": true, "ss": true, "netstat": true, "ip": true, "ifconfig": true,
		"ping": true, "traceroute": true, "nslookup": true, "dig": true,
		"curl": true, "wget": true, "git": true, "npm": true, "node": true,
		"python": true, "python3": true, "pip": true, "pip3": true,
		"make": true, "gcc": true, "g++": true, "java": true, "jar": true,
		"mvn": true, "gradle": true, "docker": true, "docker-compose": true,
		"nano": true, "vim": true, "vi": true, "less": true, "more": true,
		"diff": true, "patch": true, "tar": true, "gzip": true, "gunzip": true,
		"zip": true, "unzip": true, "bzip2": true, "xz": true, "zstd": true,
	}
	if !allowedCommands[body.Cmd[0]] {
		writeError(w, http.StatusForbidden, "command not allowed")
		return
	}

	// Block dangerous patterns in command arguments
	for _, cmd := range body.Cmd[1:] {
		dangerousPatterns := []string{"rm -rf", "dd ", "mkfs", ":(){ :;}", "sh -c", "bash -c"}
		for _, pattern := range dangerousPatterns {
			if strings.Contains(cmd, pattern) {
				writeError(w, http.StatusForbidden, "dangerous command pattern detected")
				return
			}
		}
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if container is managed
	inspect, inspectErr := docker.ContainerInspect(r.Context(), id)
	if inspectErr == nil && inspect.Config != nil && inspect.Config.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot exec into managed containers")
		return
	}

	execConfig := container.ExecOptions{
		Cmd: body.Cmd, AttachStdout: body.AttachStdout,
		AttachStderr: body.AttachStderr, Tty: body.Tty,
	}
	execResp, err := docker.ContainerExecCreate(r.Context(), id, execConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp, err := docker.ContainerExecAttach(r.Context(), execResp.ID, container.ExecAttachOptions{Tty: body.Tty})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Close()

	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	_, copyErr := stdcopy.StdCopy(stdout, stderr, resp.Reader)
	output := map[string]any{
		"execId": execResp.ID,
		"stdout": stdout.String(),
		"stderr": stderr.String(),
	}
	if copyErr != nil {
		output["error"] = copyErr.Error()
	}
	if inspectResp, inspectErr := docker.ContainerExecInspect(r.Context(), execResp.ID); inspectErr == nil {
		output["exitCode"] = inspectResp.ExitCode
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:exec", "container", &id, map[string]interface{}{
		"cmd": strings.Join(body.Cmd, " "),
		"tty": fmt.Sprintf("%t", body.Tty),
	})

	writeJSON(w, http.StatusOK, output)
}

func (s *Server) handleContainerTop(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	// Check if container is managed and user is not infra admin
	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	inspect, inspectErr := docker.ContainerInspect(r.Context(), id)
	if inspectErr == nil && inspect.Config != nil && !userInfo.IsInfraAdmin && inspect.Config.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot view processes of managed containers")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:top", "container", &id, nil)

	top, err := docker.ContainerTop(r.Context(), id, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, top)
}

func (s *Server) handleContainerChanges(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	// Check if container is managed and user is not infra admin
	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	inspect, inspectErr := docker.ContainerInspect(r.Context(), id)
	if inspectErr == nil && inspect.Config != nil && !userInfo.IsInfraAdmin && inspect.Config.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot view changes of managed containers")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:changes", "container", &id, nil)

	changes, err := docker.ContainerDiff(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, changes)
}

func (s *Server) handleContainerStats(w http.ResponseWriter, r *http.Request) {
	// Authentication and authorization
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	// Check if container is managed and user is not infra admin
	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	inspect, inspectErr := docker.ContainerInspect(r.Context(), id)
	if inspectErr == nil && inspect.Config != nil && !userInfo.IsInfraAdmin && inspect.Config.Labels["modern-game-panel.server_id"] != "" {
		writeError(w, http.StatusForbidden, "cannot view stats of managed containers")
		return
	}

	// Audit log
	s.logAdminAction(r.Context(), userInfo.UserID, "container:stats", "container", &id, nil)

	stats, err := docker.ContainerStatsOneShot(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer stats.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(w, stats.Body)
}

func (s *Server) adminDockerClient() (*client.Client, error) {
	if s.runtime == nil {
		return nil, errRuntimeUnavailable
	}
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

// isPlatformNetwork checks if a network is a platform-managed network
func isPlatformNetwork(name string) bool {
	platformNetworks := []string{
		"bridge", "host", "none", "overlay",
		"docker_gwbridge", "ingress",
	}
	for _, prefix := range platformNetworks {
		if name == prefix || strings.HasPrefix(name, prefix+"-") {
			return true
		}
	}
	return false
}

// filterNetworkInspect removes sensitive information from network inspect results
func filterNetworkInspect(net network.Inspect) network.Inspect {
	// Remove sensitive configuration
	for i := range net.IPAM.Config {
		// Remove subnet details that might be sensitive
		if net.IPAM.Config[i].Subnet != "" {
			// Keep only the network prefix, not full subnet
			parts := strings.Split(net.IPAM.Config[i].Subnet, ".")
			if len(parts) >= 3 {
				net.IPAM.Config[i].Subnet = strings.Join(parts[:3], ".") + ".0/24"
			}
		}
	}
	return net
}

// filterContainerInspect removes sensitive information from container inspect results
func filterContainerInspect(inspect types.ContainerJSON) types.ContainerJSON {
	// Remove sensitive environment variables
	if inspect.Config != nil {
		filteredEnv := make([]string, 0, len(inspect.Config.Env))
		for _, env := range inspect.Config.Env {
			// Keep only non-sensitive environment variables
			if !strings.HasPrefix(env, "PASSWORD=") &&
				!strings.HasPrefix(env, "SECRET=") &&
				!strings.HasPrefix(env, "TOKEN=") &&
				!strings.HasPrefix(env, "API_KEY=") {
				filteredEnv = append(filteredEnv, env)
			}
		}
		inspect.Config.Env = filteredEnv
	}
	return inspect
}

// AdminUserInfo contains information about the authenticated admin user
type AdminUserInfo struct {
	UserID       string
	IsAdmin      bool
	IsInfraAdmin bool
	ServerID     string
	Scope        string
}

// getAdminUserInfo extracts user information from the request context
func (s *Server) getAdminUserInfo(r *http.Request) (*AdminUserInfo, error) {
	// Check for token in header (used by Forge API)
	tokenStr := r.Header.Get("Authorization")
	if tokenStr != "" {
		tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
		if tokenStr != "" {
			// Try to validate as JWT token from Forge
			if s.tokenGenerator != nil {
				claims, err := s.tokenGenerator.Validate(tokenStr)
				if err == nil {
					return &AdminUserInfo{
						UserID:       claims.User,
						IsAdmin:      true, // Forge tokens for admin endpoints are admin
						IsInfraAdmin: true,
						ServerID:     claims.ServerID,
						Scope:        string(claims.Scope),
					}, nil
				}
			}
		}
	}

	// Fallback to beacon's own token authentication
	if s.token != "" {
		timestamp := r.Header.Get("X-Panel-Timestamp")
		signature := r.Header.Get("X-Panel-Signature")

		if timestamp != "" && signature != "" {
			parsed, err := time.Parse(time.RFC3339, timestamp)
			if err != nil || time.Since(parsed) > 5*time.Minute || time.Until(parsed) > 5*time.Minute {
				return nil, fmt.Errorf("invalid signature timestamp")
			}
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			expected := sign(s.token, r.Method, r.URL.RequestURI(), timestamp, body)
			if hmac.Equal([]byte(signature), []byte(expected)) {
				// This is beacon's own admin token - full access
				return &AdminUserInfo{
					UserID:       "beacon-admin",
					IsAdmin:      true,
					IsInfraAdmin: true,
					ServerID:     "",
					Scope:        "admin",
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("admin authentication required")
}

// logAdminAction logs an admin action to the audit system
func (s *Server) logAdminAction(ctx context.Context, userID, action, resourceType string, resourceID *string, details map[string]interface{}) {
	// If we have a panel client, send audit log
	if s.panelClient != nil {
		// Create activity entry
		meta := make(map[string]interface{}, len(details))
		for k, v := range details {
			meta[k] = v
		}
		activity := remote.Activity{
			Event:     action,
			User:      userID,
			Server:    "", // No specific server for admin actions
			IP:        "127.0.0.1",
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Metadata:  meta,
		}

		// Send to Forge API for audit logging
		go func() {
			_ = s.panelClient.SendActivityLogs(ctx, []remote.Activity{activity})
		}()
	}
}

// --- Image Build ---

func (s *Server) handleImageBuild(w http.ResponseWriter, r *http.Request) {
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	var body struct {
		Dockerfile string `json:"dockerfile"`
		Tag        string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Dockerfile == "" {
		writeError(w, http.StatusBadRequest, "dockerfile content is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: "Dockerfile",
		Size: int64(len(body.Dockerfile)),
		Mode: 0o644,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := tw.Write([]byte(body.Dockerfile)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tw.Close()

	opts := types.ImageBuildOptions{
		Dockerfile: "Dockerfile",
		Remove:     true,
	}
	if body.Tag != "" {
		opts.Tags = []string{body.Tag}
	}

	resp, err := docker.ImageBuild(r.Context(), &buf, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	s.logAdminAction(r.Context(), userInfo.UserID, "image:build", "image", nil, map[string]interface{}{
		"tag": body.Tag,
	})

	writeJSON(w, http.StatusOK, map[string]any{"status": "built", "tag": body.Tag})
}

// --- Image Tag ---

func (s *Server) handleImageTag(w http.ResponseWriter, r *http.Request) {
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "image id is required")
		return
	}

	var body struct {
		Repo string `json:"repo"`
		Tag  string `json:"tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Repo == "" {
		writeError(w, http.StatusBadRequest, "repo is required")
		return
	}
	ref := body.Repo
	if body.Tag != "" {
		ref = body.Repo + ":" + body.Tag
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := docker.ImageTag(r.Context(), id, ref); err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "image not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logAdminAction(r.Context(), userInfo.UserID, "image:tag", "image", &id, map[string]interface{}{
		"repo": body.Repo, "tag": body.Tag,
	})

	writeJSON(w, http.StatusOK, map[string]any{"status": "tagged", "ref": ref})
}

// --- Image Search ---

func (s *Server) handleImageSearch(w http.ResponseWriter, r *http.Request) {
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	term := r.URL.Query().Get("term")
	if term == "" {
		writeError(w, http.StatusBadRequest, "term query parameter is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	results, err := docker.ImageSearch(r.Context(), term, registry.SearchOptions{Limit: 25})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type searchResult struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		StarCount   int    `json:"starCount"`
		IsOfficial  bool   `json:"isOfficial"`
		IsAutomated bool   `json:"isAutomated"`
	}
	out := make([]searchResult, 0, len(results))
	for _, r := range results {
		out = append(out, searchResult{
			Name:        r.Name,
			Description: r.Description,
			StarCount:   r.StarCount,
			IsOfficial:  r.IsOfficial,
			IsAutomated: r.IsAutomated,
		})
	}

	s.logAdminAction(r.Context(), userInfo.UserID, "image:search", "image", nil, map[string]interface{}{
		"term": term, "count": len(out),
	})

	writeJSON(w, http.StatusOK, out)
}

// --- Container Files List ---

func (s *Server) handleContainerFilesList(w http.ResponseWriter, r *http.Request) {
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		filePath = "/"
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	tarReader, _, err := docker.CopyFromContainer(r.Context(), id, filePath)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "path not found in container")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tarReader.Close()

	type fileEntry struct {
		Name      string `json:"name"`
		Path      string `json:"path"`
		Directory bool   `json:"directory"`
		Size      int64  `json:"size"`
		Mode      string `json:"mode"`
	}
	var entries []fileEntry
	tr := tar.NewReader(tarReader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		entries = append(entries, fileEntry{
			Name:      path.Base(hdr.Name),
			Path:      hdr.Name,
			Directory: hdr.FileInfo().IsDir(),
			Size:      hdr.Size,
			Mode:      hdr.FileInfo().Mode().Perm().String(),
		})
	}

	writeJSON(w, http.StatusOK, entries)
}

// --- Container Files Read ---

func (s *Server) handleContainerFilesRead(w http.ResponseWriter, r *http.Request) {
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	tarReader, _, err := docker.CopyFromContainer(r.Context(), id, body.Path)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "file not found in container")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tarReader.Close()

	tr := tar.NewReader(tarReader)
	hdr, err := tr.Next()
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if hdr.FileInfo().IsDir() {
		writeError(w, http.StatusBadRequest, "cannot read a directory")
		return
	}

	maxSize := int64(10 * 1024 * 1024)
	if hdr.Size > maxSize {
		writeError(w, http.StatusRequestEntityTooLarge, "file too large to read")
		return
	}
	data, err := io.ReadAll(tr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// --- Container Files Upload ---

func (s *Server) handleContainerFilesUpload(w http.ResponseWriter, r *http.Request) {
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	destPath := r.URL.Query().Get("path")
	if destPath == "" {
		destPath = "/"
	}

	maxSize := int64(512 * 1024 * 1024)
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(maxSize); err != nil {
		writeError(w, http.StatusBadRequest, "failed to parse multipart form: "+err.Error())
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "no file provided in 'file' field")
		return
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	type uploadFailure struct {
		File  string `json:"file"`
		Error string `json:"error"`
	}
	var failures []uploadFailure
	var uploaded int
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			failures = append(failures, uploadFailure{File: fh.Filename, Error: err.Error()})
			continue
		}
		hdr := &tar.Header{
			Name: fh.Filename,
			Size: fh.Size,
			Mode: 0o644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			f.Close()
			failures = append(failures, uploadFailure{File: fh.Filename, Error: err.Error()})
			continue
		}
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			failures = append(failures, uploadFailure{File: fh.Filename, Error: err.Error()})
			continue
		}
		f.Close()
		uploaded++
	}
	tw.Close()

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := docker.CopyToContainer(r.Context(), id, destPath, &buf, container.CopyToContainerOptions{}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logAdminAction(r.Context(), userInfo.UserID, "container:files:upload", "container", &id, nil)

	if len(failures) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": "partial", "uploaded": uploaded, "failed": failures})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "uploaded"})
}

// --- Container Files Delete ---

func (s *Server) handleContainerFilesDelete(w http.ResponseWriter, r *http.Request) {
	userInfo, err := s.getAdminUserInfo(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !userInfo.IsAdmin {
		writeError(w, http.StatusForbidden, "admin access required")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container id is required")
		return
	}

	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	docker, err := s.adminDockerClient()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	execConfig := container.ExecOptions{
		Cmd:          []string{"rm", "-rf", body.Path},
		AttachStdout: false,
		AttachStderr: true,
	}
	execResp, err := docker.ContainerExecCreate(r.Context(), id, execConfig)
	if err != nil {
		if client.IsErrNotFound(err) {
			writeError(w, http.StatusNotFound, "container not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp, err := docker.ContainerExecAttach(r.Context(), execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer resp.Close()

	s.logAdminAction(r.Context(), userInfo.UserID, "container:files:delete", "container", &id, map[string]interface{}{
		"path": body.Path,
	})

	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}
