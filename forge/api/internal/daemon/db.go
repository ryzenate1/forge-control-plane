package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func (c *Client) BackupDatabase(ctx context.Context, baseURL, nodeToken, containerID, engine string) error {
	body, _ := json.Marshal(map[string]string{"containerId": containerID, "engine": engine})
	endpoint := strings.TrimRight(baseURL, "/") + "/database/backup"
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
		return fmt.Errorf("beacon backup database failed status=%d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
