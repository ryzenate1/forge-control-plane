package cloud

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type BeaconBackupConfig struct {
	Adapter      string
	Bucket       string
	Region       string
	Endpoint     string
	Prefix       string
	UsePathStyle string
}

// BeaconCloudInit returns Ubuntu-compatible cloud-init that installs Docker
// from Docker's apt repository and starts Beacon with host networking/storage.
func BeaconCloudInit(nodeID, nodeCredential, panelAPIURL, beaconImage string, backupConfigs ...BeaconBackupConfig) (string, error) {
	for name, value := range map[string]string{"node ID": nodeID, "node credential": nodeCredential, "panel API URL": panelAPIURL, "Beacon image": beaconImage} {
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("%s is required for Beacon bootstrap", name)
		}
	}
	parsed, err := url.Parse(panelAPIURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("Beacon panel API URL must be an absolute http(s) URL")
	}
	if strings.ContainsAny(beaconImage, "\r\n\t ") {
		return "", errors.New("Beacon image must be a single container image reference")
	}
	encode := func(value string) string { return base64.StdEncoding.EncodeToString([]byte(value)) }
	backup := BeaconBackupConfig{Adapter: "local", UsePathStyle: "true"}
	if len(backupConfigs) > 0 {
		backup = backupConfigs[0]
		if strings.TrimSpace(backup.Adapter) == "" {
			backup.Adapter = "local"
		}
		if strings.TrimSpace(backup.UsePathStyle) == "" {
			backup.UsePathStyle = "true"
		}
	}
	if backup.Adapter != "local" && backup.Adapter != "s3" {
		return "", errors.New("Beacon backup adapter must be local or s3")
	}
	if backup.Adapter == "s3" && strings.TrimSpace(backup.Bucket) == "" {
		return "", errors.New("S3 bucket is required for Beacon S3 backups")
	}
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail
install -m 0755 -d /etc/apt/keyrings /srv/game-panel/servers
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
cat >/etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${UBUNTU_CODENAME:-$(. /etc/os-release && echo "$VERSION_CODENAME")}
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
node_id=$(printf '%%s' '%s' | base64 -d)
node_token=$(printf '%%s' '%s' | base64 -d)
panel_url=$(printf '%%s' '%s' | base64 -d)
backup_adapter=$(printf '%%s' '%s' | base64 -d)
s3_bucket=$(printf '%%s' '%s' | base64 -d)
s3_region=$(printf '%%s' '%s' | base64 -d)
s3_endpoint=$(printf '%%s' '%s' | base64 -d)
s3_prefix=$(printf '%%s' '%s' | base64 -d)
s3_path_style=$(printf '%%s' '%s' | base64 -d)
docker pull %s
docker rm -f gamepanel-beacon 2>/dev/null || true
docker run -d --name gamepanel-beacon --restart unless-stopped --network host \
  -e APP_ENV=production -e DAEMON_ADDR=:9090 -e DAEMON_SFTP_ADDR=:2022 \
  -e DAEMON_DATA_DIR=/srv/game-panel/servers -e DAEMON_NODE_ID="$node_id" \
  -e DAEMON_NODE_TOKEN="$node_token" -e PANEL_API_URL="$panel_url" \
  -e DAEMON_ALLOW_MOCK_RUNTIME=false -e DAEMON_ALLOW_INSECURE_NO_AUTH=false \
  -e BACKUP_ADAPTER="$backup_adapter" -e S3_BUCKET="$s3_bucket" \
  -e S3_REGION="$s3_region" -e S3_ENDPOINT="$s3_endpoint" \
  -e S3_PREFIX="$s3_prefix" -e S3_USE_PATH_STYLE="$s3_path_style" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /srv/game-panel/servers:/srv/game-panel/servers %s
`, encode(nodeID), encode(nodeCredential), encode(panelAPIURL), encode(backup.Adapter), encode(backup.Bucket), encode(backup.Region), encode(backup.Endpoint), encode(backup.Prefix), encode(backup.UsePathStyle), beaconImage, beaconImage)
	return script, nil
}
