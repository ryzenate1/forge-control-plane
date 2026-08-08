package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type DBContainerProvisionRequest struct {
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

type DBContainerProvisionResponse struct {
	ContainerID      string `json:"containerId"`
	ConnectionString string `json:"connectionString"`
	Port             int    `json:"port"`
	VolumeID         string `json:"volumeId"`
}

type DBContainerDeProvisionRequest struct {
	ContainerID string `json:"containerId"`
	VolumeID    string `json:"volumeId"`
}

type DatabaseBackupResponse struct {
	OK       bool   `json:"ok"`
	BackupID string `json:"backupId"`
	File     string `json:"file"`
	Name     string `json:"name"`
	Engine   string `json:"engine"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

type DatabaseStatusResponse struct {
	Status      string `json:"status"`
	Running     bool   `json:"running"`
	ContainerID string `json:"containerId"`
}

func (c *Client) ProvisionDatabase(ctx context.Context, baseURL, nodeToken string, req DBContainerProvisionRequest) (DBContainerProvisionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return DBContainerProvisionResponse{}, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/database/provision"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return DBContainerProvisionResponse{}, err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return DBContainerProvisionResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return DBContainerProvisionResponse{}, fmt.Errorf("beacon provision database failed status=%d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var resp DBContainerProvisionResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return DBContainerProvisionResponse{}, err
	}
	return resp, nil
}

func (c *Client) DeProvisionDatabase(ctx context.Context, baseURL, nodeToken, containerID, volumeID string) error {
	body, _ := json.Marshal(DBContainerDeProvisionRequest{ContainerID: containerID, VolumeID: volumeID})
	endpoint := strings.TrimRight(baseURL, "/") + "/database/provision"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodDelete, endpoint, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("beacon deprovision database failed status=%d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) BackupDatabase(ctx context.Context, baseURL, nodeToken, containerID, engine, backupID string) (BackupEntry, error) {
	body, _ := json.Marshal(map[string]string{"containerId": containerID, "engine": engine, "backupId": backupID})
	endpoint := strings.TrimRight(baseURL, "/") + "/database/backup"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return BackupEntry{}, err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return BackupEntry{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return BackupEntry{}, fmt.Errorf("beacon backup database failed status=%d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var resp DatabaseBackupResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return BackupEntry{}, fmt.Errorf("decode database backup response: %w", err)
	}
	return BackupEntry{
		BackupID: resp.BackupID,
		Name:     resp.Name,
		Checksum: resp.Checksum,
		Size:     resp.Size,
	}, nil
}

func (c *Client) DownloadDatabaseBackup(ctx context.Context, baseURL, nodeToken, backupID string) (io.ReadCloser, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/database/backups/" + url.PathEscape(backupID)
	req, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		_ = res.Body.Close()
		return nil, fmt.Errorf("beacon database backup download failed status=%d", res.StatusCode)
	}
	return res.Body, nil
}

func (c *Client) DeleteDatabaseBackup(ctx context.Context, baseURL, nodeToken, backupID string) error {
	endpoint := strings.TrimRight(baseURL, "/") + "/database/backups/" + url.PathEscape(backupID)
	req, err := c.newRequest(ctx, nodeToken, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("beacon database backup delete failed status=%d", res.StatusCode)
	}
	return nil
}

func (c *Client) DatabaseStatus(ctx context.Context, baseURL, nodeToken, containerID string) (DatabaseStatusResponse, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/database/status/" + containerID
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodGet, endpoint, nil)
	if err != nil {
		return DatabaseStatusResponse{}, err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return DatabaseStatusResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return DatabaseStatusResponse{}, fmt.Errorf("beacon database status failed status=%d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var status DatabaseStatusResponse
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		return DatabaseStatusResponse{}, fmt.Errorf("decode database status response: %w", err)
	}
	return status, nil
}

func (c *Client) RestoreDatabase(ctx context.Context, baseURL, nodeToken, containerID, engine, backupID string) error {
	body, _ := json.Marshal(map[string]string{"containerId": containerID, "engine": engine, "backupId": backupID})
	endpoint := strings.TrimRight(baseURL, "/") + "/database/restore"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("beacon restore database failed status=%d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) BackupVolume(ctx context.Context, baseURL, nodeToken, serverID, volumeName string) (BackupEntry, error) {
	body, _ := json.Marshal(map[string]string{"volumeName": volumeName})
	endpoint := strings.TrimRight(baseURL, "/") + "/servers/" + serverID + "/backups/volume"
	httpReq, err := c.newRequest(ctx, nodeToken, http.MethodPost, endpoint, body)
	if err != nil {
		return BackupEntry{}, err
	}
	res, err := c.httpClient.Do(httpReq)
	if err != nil {
		return BackupEntry{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return BackupEntry{}, fmt.Errorf("beacon backup volume failed status=%d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var entry BackupEntry
	if err := json.NewDecoder(res.Body).Decode(&entry); err != nil {
		return BackupEntry{}, fmt.Errorf("decode volume backup entry: %w", err)
	}
	return entry, nil
}
