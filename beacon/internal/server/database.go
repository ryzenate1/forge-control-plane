package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type databaseProvisionRequest struct {
	ServerID   string `json:"serverId"`
	Engine     string `json:"engine"`
	Version    string `json:"version"`
	MemoryMB   int    `json:"memoryMb"`
	CPUShares  int    `json:"cpuShares"`
	DBName     string `json:"dbName"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Port       int    `json:"port"`
	VolumeName string `json:"volumeName"`
}

type databaseProvisionResponse struct {
	ContainerID string `json:"containerId"`
	Port        int    `json:"port"`
	VolumeID    string `json:"volumeId"`
}

type databaseDeProvisionRequest struct {
	ContainerID string `json:"containerId"`
	VolumeID    string `json:"volumeId"`
}

const databaseLabel = "modern-game-panel.database"

func databaseImageName(engine, version string) string {
	images := map[string]string{
		"postgresql": "postgres",
		"mysql":      "mysql",
		"mariadb":    "mariadb",
		"redis":      "redis",
		"mongodb":    "mongo",
	}
	base, ok := images[strings.ToLower(engine)]
	if !ok {
		base = engine
	}
	return base + ":" + version
}

func databaseContainerName(dbID string) string {
	return "mgp-db-" + dbID
}

func databaseEnvVars(engine, dbName, username, password string) []string {
	switch strings.ToLower(engine) {
	case "postgresql":
		return []string{
			"POSTGRES_DB=" + dbName,
			"POSTGRES_USER=" + username,
			"POSTGRES_PASSWORD=" + password,
		}
	case "mysql", "mariadb":
		return []string{
			"MYSQL_DATABASE=" + dbName,
			"MYSQL_USER=" + username,
			"MYSQL_PASSWORD=" + password,
			"MYSQL_ROOT_PASSWORD=" + password,
		}
	case "mongodb":
		return []string{
			"MONGO_INITDB_DATABASE=" + dbName,
			"MONGO_INITDB_ROOT_USERNAME=" + username,
			"MONGO_INITDB_ROOT_PASSWORD=" + password,
		}
	case "redis":
		return []string{
			"REDIS_PASSWORD=" + password,
		}
	default:
		return nil
	}
}

func databaseDefaultPort(engine string) int {
	ports := map[string]int{
		"postgresql": 5432,
		"mysql":      3306,
		"mariadb":    3306,
		"redis":      6379,
		"mongodb":    27017,
	}
	if port, ok := ports[strings.ToLower(engine)]; ok {
		return port
	}
	return 0
}

func getDockerClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

func getDefaultNetwork() string {
	val := strings.TrimSpace(os.Getenv("DAEMON_DOCKER_NETWORK"))
	if val == "" {
		return "gamepanel"
	}
	return val
}

func (s *Server) handleDatabaseProvision(w http.ResponseWriter, r *http.Request) {
	var req databaseProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.Engine == "" || req.Version == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "engine and version are required"})
		return
	}

	cli, err := getDockerClient()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "docker client unavailable: " + err.Error()})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	engine := strings.ToLower(strings.TrimSpace(req.Engine))
	imageName := databaseImageName(engine, req.Version)
	containerName := databaseContainerName(req.ServerID + "-" + engine)
	volumeName := req.VolumeName
	if volumeName == "" {
		volumeName = "mgp-db-data-" + req.ServerID + "-" + engine
	}

	containerPort := req.Port
	if containerPort == 0 {
		containerPort = databaseDefaultPort(engine)
	}

	if existing, err := cli.ContainerInspect(ctx, containerName); err == nil {
		_ = existing
		if existing.State != nil && existing.State.Running {
			resp := databaseProvisionResponse{
				ContainerID: existing.ID,
				VolumeID:    volumeName,
			}
			if existing.NetworkSettings != nil && len(existing.NetworkSettings.Ports) > 0 {
				for _, bindings := range existing.NetworkSettings.Ports {
					if len(bindings) > 0 {
						if p, err := strconv.Atoi(bindings[0].HostPort); err == nil {
							resp.Port = p
							break
						}
					}
				}
			}
			if resp.Port == 0 {
				resp.Port = containerPort
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}
		timeout := 10
		_ = cli.ContainerStop(ctx, existing.ID, container.StopOptions{Timeout: &timeout})
		_ = cli.ContainerRemove(ctx, existing.ID, container.RemoveOptions{RemoveVolumes: false})
	}

	if _, _, err := cli.ImageInspectWithRaw(ctx, imageName); err != nil {
		pull, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "pull image " + imageName + ": " + err.Error()})
			return
		}
		_, _ = io.Copy(io.Discard, pull)
		_ = pull.Close()
	}

	networkName := getDefaultNetwork()
	networks, err := cli.NetworkList(ctx, network.ListOptions{Filters: filters.NewArgs(filters.Arg("name", "^"+networkName+"$"))})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "list networks: " + err.Error()})
		return
	}
	if len(networks) == 0 {
		_, err = cli.NetworkCreate(ctx, networkName, network.CreateOptions{
			Driver: "bridge",
			Labels: map[string]string{"modern-game-panel.managed": "true"},
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "create network: " + err.Error()})
			return
		}
	}

	hostPort, err := findFreePort()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "find free port: " + err.Error()})
		return
	}

	envVars := databaseEnvVars(engine, req.DBName, req.Username, req.Password)
	memoryMB := req.MemoryMB
	if memoryMB == 0 {
		memoryMB = 256
	}
	memory := int64(memoryMB) * 1024 * 1024

	created, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image: imageName,
			Env:   envVars,
			ExposedPorts: nat.PortSet{
				nat.Port(fmt.Sprintf("%d/tcp", containerPort)): struct{}{},
			},
			Labels: map[string]string{
				databaseLabel:                   "true",
				"modern-game-panel.server_id":   req.ServerID,
				"modern-game-panel.db_engine":   engine,
			},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: volumeName,
					Target: dataDirForEngine(engine),
				},
			},
			NetworkMode: container.NetworkMode(networkName),
			PortBindings: nat.PortMap{
				nat.Port(fmt.Sprintf("%d/tcp", containerPort)): []nat.PortBinding{
					{HostPort: strconv.Itoa(hostPort)},
				},
			},
			Resources: container.Resources{
				Memory: memory,
			},
			RestartPolicy: container.RestartPolicy{
				Name: "unless-stopped",
			},
			LogConfig: container.LogConfig{
				Type: "json-file",
				Config: map[string]string{
					"max-size": "10m",
					"max-file": "3",
				},
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkName: {},
			},
		},
		nil,
		containerName,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "create container: " + err.Error()})
		return
	}

	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "start container: " + err.Error()})
		return
	}

	if err := waitForDBReady(ctx, cli, created.ID, engine); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "database not ready: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, databaseProvisionResponse{
		ContainerID: created.ID,
		Port:        hostPort,
		VolumeID:    volumeName,
	})
}

func (s *Server) handleDatabaseDeProvision(w http.ResponseWriter, r *http.Request) {
	var req databaseDeProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	cli, err := getDockerClient()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "docker client unavailable: " + err.Error()})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if req.ContainerID != "" {
		timeout := 10
		_ = cli.ContainerStop(ctx, req.ContainerID, container.StopOptions{Timeout: &timeout})
		_ = cli.ContainerRemove(ctx, req.ContainerID, container.RemoveOptions{RemoveVolumes: false})
	}
	if req.VolumeID != "" {
		_ = cli.VolumeRemove(ctx, req.VolumeID, true)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDatabaseStatus(w http.ResponseWriter, r *http.Request) {
	containerID := r.PathValue("containerId")
	if containerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "containerId is required"})
		return
	}

	cli, err := getDockerClient()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "docker client unavailable: " + err.Error()})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"status": "not_found", "running": false})
		return
	}

	running := inspect.State != nil && inspect.State.Running
	status := "stopped"
	if running {
		if inspect.State.Health != nil {
			status = inspect.State.Health.Status
		} else {
			status = "running"
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      status,
		"running":     running,
		"containerId": inspect.ID,
	})
}

func (s *Server) handleDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ContainerID string `json:"containerId"`
		Engine      string `json:"engine"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if req.ContainerID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "containerId is required"})
		return
	}

	cli, err := getDockerClient()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "docker client unavailable: " + err.Error()})
		return
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Second)
	defer cancel()

	engine := strings.ToLower(strings.TrimSpace(req.Engine))
	cmd := backupCommandForEngine(engine)
	if len(cmd) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported engine for backup: " + engine})
		return
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	fileName := fmt.Sprintf("backup-%s-%s.sql.gz", engine, timestamp)

	execResp, err := cli.ContainerExecCreate(ctx, req.ContainerID, container.ExecOptions{
		Cmd:          []string{"sh", "-c", strings.Join(cmd, " ") + " | gzip > /tmp/" + fileName},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "exec create: " + err.Error()})
		return
	}

	if err := cli.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "exec start: " + err.Error()})
		return
	}

	for {
		inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "exec inspect: " + err.Error()})
			return
		}
		if !inspect.Running {
			if inspect.ExitCode != 0 {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("backup command exited with code %d", inspect.ExitCode)})
				return
			}
			break
		}
		select {
		case <-ctx.Done():
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "backup timed out"})
			return
		case <-time.After(1 * time.Second):
		}
	}

	rc, _, err := cli.CopyFromContainer(ctx, req.ContainerID, "/tmp/"+fileName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "copy backup: " + err.Error()})
		return
	}
	defer rc.Close()

	backupDir := filepath.Join(os.TempDir(), "mgp-db-backups")
	_ = os.MkdirAll(backupDir, 0o750)
	backupPath := filepath.Join(backupDir, fileName)
	f, err := os.Create(backupPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "create backup file: " + err.Error()})
		return
	}
	defer f.Close()
	_, _ = io.Copy(f, rc)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"file":   backupPath,
		"name":   fileName,
		"engine": engine,
	})
}

func dataDirForEngine(engine string) string {
	switch strings.ToLower(engine) {
	case "postgresql":
		return "/var/lib/postgresql/data"
	case "mysql", "mariadb":
		return "/var/lib/mysql"
	case "mongodb":
		return "/data/db"
	case "redis":
		return "/data"
	default:
		return "/data"
	}
}

func backupCommandForEngine(engine string) []string {
	switch strings.ToLower(engine) {
	case "postgresql":
		return []string{"pg_dumpall", "-U", "$POSTGRES_USER"}
	case "mysql", "mariadb":
		return []string{"mysqldump", "--all-databases", "-u", "root", "-p$MYSQL_ROOT_PASSWORD"}
	case "mongodb":
		return []string{"mongodump", "--archive"}
	case "redis":
		return []string{"redis-cli", "--rdb", "/tmp/backup.rdb", "SAVE"}
	default:
		return nil
	}
}

func findFreePort() (int, error) {
	dialer := &net.Dialer{Timeout: 50 * time.Millisecond}
	conn, err := dialer.Dial("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.TCPAddr)
	return addr.Port, nil
}

func waitForDBReady(ctx context.Context, cli *client.Client, containerID, engine string) error {
	deadline := time.Now().Add(60 * time.Second)
	engine = strings.ToLower(engine)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
			inspect, err := cli.ContainerInspect(ctx, containerID)
			if err != nil {
				continue
			}
			if inspect.State == nil || !inspect.State.Running {
				status := "unknown"
				if inspect.State != nil {
					status = inspect.State.Status
				}
				return fmt.Errorf("container stopped unexpectedly: %s", status)
			}
			if execHealthCheck(ctx, cli, containerID, engine) {
				return nil
			}
		}
	}
	return fmt.Errorf("timeout waiting for database to become healthy")
}

func execHealthCheck(ctx context.Context, cli *client.Client, containerID, engine string) bool {
	var cmd []string
	switch engine {
	case "postgresql":
		cmd = []string{"pg_isready", "-U", "postgres"}
	case "mysql", "mariadb":
		cmd = []string{"mysqladmin", "ping", "-h", "localhost"}
	case "mongodb":
		cmd = []string{"mongosh", "--eval", "db.adminCommand('ping')"}
	case "redis":
		cmd = []string{"redis-cli", "ping"}
	default:
		return true
	}

	execResp, err := cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return false
	}

	if err := cli.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{}); err != nil {
		return false
	}

	inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return false
	}
	return inspect.ExitCode == 0
}
