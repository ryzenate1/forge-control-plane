package store

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (s *Store) ListNodes(ctx context.Context) ([]Node, error) {
	return s.ListNodesPaginated(ctx, 0, 0)
}

func (s *Store) ListNodesPaginated(ctx context.Context, offset, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := s.db.Query(ctx, `
		SELECT n.id::text, COALESCE(n.uuid, n.id)::text, n.name, COALESCE(l.long, n.region), n.base_url,
		       COALESCE(n.fqdn, ''), COALESCE(n.scheme, 'http'), n.behind_proxy, n.status, n.maintenance_mode,
		       COALESCE(n.memory_mb, 0), COALESCE(n.disk_mb, 0), COALESCE(n.upload_size_mb, 100),
		       COALESCE(n.daemon_base, '/var/lib/forge/volumes'),
		       COALESCE(n.daemon_listen, 8080), COALESCE(n.daemon_sftp, 2022),
		       COALESCE(n.daemon_token_id, ''),
		       n.last_seen_at,
		       n.version, n.os, n.architecture, n.cpu_threads, n.docker_status, n.node_memory_mb, n.node_disk_mb, n.heartbeat_error,
		       COALESCE(n.display_name, ''), COALESCE(n.public_hostname, ''),
		       COALESCE(n.listen_port_min, 0), COALESCE(n.listen_port_max, 0),
		       COALESCE(n.allowed_ips, '{}'), COALESCE(n.network_interface, ''),
		       COALESCE(n.daemon_ssl_cert, ''), COALESCE(n.daemon_ssl_key, ''),
		       n.auto_connect, COALESCE(n.connection_retries, 3), COALESCE(n.heartbeat_interval, 15),
		       COALESCE(n.cpu_cores, 0), n.cpu_threads,
		       COALESCE(n.memory_overallocate, 0), COALESCE(n.disk_overallocate, 0),
		       COALESCE(n.reserved_memory_mb, 0), COALESCE(n.reserved_disk_mb, 0),
		       COALESCE(n.default_allocation_ip, '0.0.0.0'),
		       COALESCE(n.allocation_port_min, 25565), COALESCE(n.allocation_port_max, 26565),
		       n.auto_allocate,
		       COALESCE(n.backup_directory, ''), COALESCE(n.transfer_directory, ''),
		       COALESCE(n.mount_points, '[]'), COALESCE(n.token_rotation_policy, 'manual'),
		       COALESCE(n.firewall_rules, '[]'), COALESCE(n.tls_setting, 'auto'),
		       n.enable_health_checks, n.enable_metrics,
		       COALESCE(n.prometheus_endpoint, ''),
		       COALESCE(n.alert_threshold_cpu, 90), COALESCE(n.alert_threshold_memory, 90), COALESCE(n.alert_threshold_disk, 90),
		   COALESCE(n.maintenance_message, ''), n.drain_before_maintenance,
		   COALESCE(n.labels, '[]'), COALESCE(n.cluster_group_id, ''),
		   COALESCE(n.public, true),
		       COALESCE(n.description, ''), n.location_id::text, n.region_id::text,
		   COALESCE(n.draining, false), COALESCE(n.desired_state, 'active'), COALESCE(n.actual_state, 'offline'),
	       COALESCE(n.heartbeat_state::text, ''), COALESCE(n.heartbeat_recovery_count, 0),
	       COALESCE(n.daemon_sftp_alias, ''), COALESCE(n.daemon_connect, 8080), COALESCE(n.cpu_overallocate, 0),
	       COALESCE(n.tags, '[]')
		FROM nodes n
		LEFT JOIN locations l ON l.id = n.location_id
		ORDER BY n.name
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := []Node{}
	for rows.Next() {
		var node Node
		if err := rows.Scan(
			&node.ID, &node.UUID, &node.Name, &node.Region, &node.BaseURL,
			&node.FQDN, &node.Scheme, &node.BehindProxy, &node.Status, &node.Maintenance,
			&node.MemoryMB, &node.DiskMB, &node.UploadSizeMB, &node.DaemonBase,
			&node.DaemonListen, &node.DaemonSFTP, &node.TokenID, &node.LastSeenAt,
			&node.Version, &node.OS, &node.Architecture, &node.CPUThreads, &node.DockerStatus,
			&node.NodeMemoryMB, &node.NodeDiskMB, &node.HeartbeatErr,
			&node.DisplayName, &node.PublicHostname,
			&node.ListenPortMin, &node.ListenPortMax, &node.AllowedIPs, &node.NetworkInterface,
			&node.DaemonSSLCert, &node.DaemonSSLKey,
			&node.AutoConnect, &node.ConnectionRetries, &node.HeartbeatInterval,
			&node.CPUCores, &node.CPUThreads,
			&node.MemoryOverallocate, &node.DiskOverallocate,
			&node.ReservedMemoryMB, &node.ReservedDiskMB,
			&node.DefaultAllocationIP, &node.AllocationPortMin, &node.AllocationPortMax,
			&node.AutoAllocate,
			&node.BackupDirectory, &node.TransferDirectory,
			&node.MountPoints, &node.TokenRotationPolicy,
			&node.FirewallRules, &node.TLSSetting,
			&node.EnableHealthChecks, &node.EnableMetrics, &node.PrometheusEndpoint,
			&node.AlertThresholdCPU, &node.AlertThresholdMemory, &node.AlertThresholdDisk,
			&node.MaintenanceMessage, &node.DrainBeforeMaintenance,
			&node.Labels, &node.ClusterGroupID, &node.Public,
			&node.Description, &node.LocationID, &node.RegionID, &node.Draining, &node.DesiredState, &node.ActualState,
			&node.HeartbeatState, &node.HeartbeatRecoveryCount,
			&node.HeartbeatState, &node.HeartbeatRecoveryCount,
			&node.DaemonSFTPAlias, &node.DaemonConnect, &node.CPUOverallocate,
			&node.Tags,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func (s *Store) GetNode(ctx context.Context, nodeID string) (Node, error) {
	var node Node
	err := s.db.QueryRow(ctx, `
		SELECT n.id::text, COALESCE(n.uuid, n.id)::text, n.name, COALESCE(l.long, n.region), n.base_url,
		       COALESCE(n.fqdn, ''), COALESCE(n.scheme, 'http'), n.behind_proxy, n.status, n.maintenance_mode,
		       COALESCE(n.memory_mb, 0), COALESCE(n.disk_mb, 0), COALESCE(n.upload_size_mb, 100),
		       COALESCE(n.daemon_base, '/var/lib/forge/volumes'),
		       COALESCE(n.daemon_listen, 8080), COALESCE(n.daemon_sftp, 2022),
		       COALESCE(n.daemon_token_id, ''),
		       n.last_seen_at,
		       n.version, n.os, n.architecture, n.cpu_threads, n.docker_status, n.node_memory_mb, n.node_disk_mb, n.heartbeat_error,
		       n.region_id::text, n.draining,
		       COALESCE(n.desired_state, 'active'), COALESCE(n.actual_state, 'offline'),
	       COALESCE(n.heartbeat_state::text, ''), COALESCE(n.heartbeat_recovery_count, 0),
		       COALESCE(n.display_name, ''), COALESCE(n.public_hostname, ''),
		       COALESCE(n.listen_port_min, 0), COALESCE(n.listen_port_max, 0),
		       COALESCE(n.allowed_ips, '{}'), COALESCE(n.network_interface, ''),
		       COALESCE(n.daemon_ssl_cert, ''), COALESCE(n.daemon_ssl_key, ''),
		       n.auto_connect, COALESCE(n.connection_retries, 3), COALESCE(n.heartbeat_interval, 15),
		       COALESCE(n.cpu_cores, 0),
		       COALESCE(n.memory_overallocate, 0), COALESCE(n.disk_overallocate, 0),
		       COALESCE(n.reserved_memory_mb, 0), COALESCE(n.reserved_disk_mb, 0),
		       COALESCE(n.default_allocation_ip, '0.0.0.0'),
		       COALESCE(n.allocation_port_min, 25565), COALESCE(n.allocation_port_max, 26565),
		       n.auto_allocate,
		       COALESCE(n.backup_directory, ''), COALESCE(n.transfer_directory, ''),
		       COALESCE(n.mount_points, '[]'), COALESCE(n.token_rotation_policy, 'manual'),
		       COALESCE(n.firewall_rules, '[]'), COALESCE(n.tls_setting, 'auto'),
		       n.enable_health_checks, n.enable_metrics,
		       COALESCE(n.prometheus_endpoint, ''),
		       COALESCE(n.alert_threshold_cpu, 90), COALESCE(n.alert_threshold_memory, 90), COALESCE(n.alert_threshold_disk, 90),
		   n.public,
		   COALESCE(n.maintenance_message, ''), n.drain_before_maintenance,
		   COALESCE(n.labels, '[]'), COALESCE(n.cluster_group_id, ''),
		   COALESCE(n.description, ''), n.location_id::text,
	       COALESCE(n.daemon_sftp_alias, ''), COALESCE(n.daemon_connect, 8080), COALESCE(n.cpu_overallocate, 0),
	       COALESCE(n.tags, '[]')
	FROM nodes n
	LEFT JOIN locations l ON l.id = n.location_id
	WHERE n.id = $1
	`, nodeID).
		Scan(
			&node.ID, &node.UUID, &node.Name, &node.Region, &node.BaseURL,
			&node.FQDN, &node.Scheme, &node.BehindProxy, &node.Status, &node.Maintenance,
			&node.MemoryMB, &node.DiskMB, &node.UploadSizeMB, &node.DaemonBase,
			&node.DaemonListen, &node.DaemonSFTP, &node.TokenID, &node.LastSeenAt,
			&node.Version, &node.OS, &node.Architecture, &node.CPUThreads, &node.DockerStatus,
			&node.NodeMemoryMB, &node.NodeDiskMB, &node.HeartbeatErr,
			&node.RegionID, &node.Draining, &node.DesiredState, &node.ActualState,
			&node.HeartbeatState, &node.HeartbeatRecoveryCount,
			&node.DisplayName, &node.PublicHostname,
			&node.ListenPortMin, &node.ListenPortMax, &node.AllowedIPs, &node.NetworkInterface,
			&node.DaemonSSLCert, &node.DaemonSSLKey,
			&node.AutoConnect, &node.ConnectionRetries, &node.HeartbeatInterval,
			&node.CPUCores,
			&node.MemoryOverallocate, &node.DiskOverallocate,
			&node.ReservedMemoryMB, &node.ReservedDiskMB,
			&node.DefaultAllocationIP, &node.AllocationPortMin, &node.AllocationPortMax,
			&node.AutoAllocate,
			&node.BackupDirectory, &node.TransferDirectory,
			&node.MountPoints, &node.TokenRotationPolicy,
			&node.FirewallRules, &node.TLSSetting,
			&node.EnableHealthChecks, &node.EnableMetrics, &node.PrometheusEndpoint,
			&node.AlertThresholdCPU, &node.AlertThresholdMemory, &node.AlertThresholdDisk,
			&node.MaintenanceMessage, &node.DrainBeforeMaintenance,
			&node.Labels, &node.ClusterGroupID, &node.Public, &node.Description, &node.LocationID,
			&node.DaemonSFTPAlias, &node.DaemonConnect, &node.CPUOverallocate,
			&node.Tags,
		)
	if err != nil {
		return Node{}, err
	}
	return node, nil
}

func (s *Store) CreateNode(ctx context.Context, req CreateNodeRequest, actorID *string) (Node, string, error) {
	req = normalizeNodeCreate(req)
	if strings.TrimSpace(req.LocationID) == "" {
		return Node{}, "", errors.New("locationId is required")
	}
	location, err := s.GetLocation(ctx, req.LocationID)
	if err != nil {
		return Node{}, "", err
	}
	if strings.TrimSpace(req.RegionID) != "" {
		if _, err := s.GetRegion(ctx, req.RegionID); err != nil {
			return Node{}, "", err
		}
	}
	req.Region = location.Short
	if err := validateNodeEndpoint(req.Name, req.BaseURL, req.FQDN, req.Scheme, req.MemoryMB, req.DiskMB, req.UploadSizeMB, req.DaemonListen, req.DaemonSFTP); err != nil {
		return Node{}, "", err
	}
	if strings.TrimSpace(req.DaemonBase) == "" {
		return Node{}, "", errors.New("daemonBase is required")
	}
	id := uuid.NewString()
	nodeUUID := uuid.NewString()
	tokenID := newDaemonTokenID()
	token := newDaemonToken()
	encryptedToken, err := s.encryptSecret(token, secretAAD("nodes", id, "daemon_token"))
	if err != nil {
		return Node{}, "", err
	}
	tokenHash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return Node{}, "", errors.New("hash node credential")
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO nodes (
			id, uuid, name, description, region, region_id, base_url, fqdn, scheme, behind_proxy, status, maintenance_mode,
			memory_mb, disk_mb, upload_size_mb, daemon_base, daemon_listen, daemon_sftp,
			token_hash, daemon_token_id, daemon_token, daemon_token_encrypted, last_seen_at, location_id,
			display_name, public_hostname, listen_port_min, listen_port_max, allowed_ips, network_interface,
			daemon_ssl_cert, daemon_ssl_key, auto_connect, connection_retries, heartbeat_interval,
			cpu_cores, memory_overallocate, disk_overallocate, reserved_memory_mb, reserved_disk_mb,
			default_allocation_ip, allocation_port_min, allocation_port_max, auto_allocate,
			backup_directory, transfer_directory, mount_points, token_rotation_policy,
			firewall_rules, tls_setting, enable_health_checks, enable_metrics, prometheus_endpoint,
			alert_threshold_cpu, alert_threshold_memory, alert_threshold_disk,
			maintenance_message, drain_before_maintenance, labels, cluster_group_id,
			public,
			daemon_sftp_alias, daemon_connect, cpu_overallocate, tags
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'offline', $11,
			$12, $13, $14, $15, $16, $17,
			$18, $19, '', $20, NULL, $21,
			$22, $23, $24, $25, $26, $27,
			$28, $29, $30, $31, $32,
			$33, $34, $35, $36, $37,
			$38, $39, $40, $41,
			$42, $43, $44, $45,
			$46, $47, $48, $49, $50,
			$51, $52, $53,
			$54, $55, $56, $57,
			$58, $59, $60, $61, $62
		)
	`, id, nodeUUID, req.Name, strings.TrimSpace(req.Description), req.Region, nullableUUID(req.RegionID), req.BaseURL, req.FQDN, req.Scheme, req.BehindProxy,
		req.Maintenance,
		req.MemoryMB, req.DiskMB, req.UploadSizeMB, req.DaemonBase, req.DaemonListen, req.DaemonSFTP,
		string(tokenHash), tokenID, encryptedToken, nullableUUID(req.LocationID),
		req.DisplayName, req.PublicHostname, req.ListenPortMin, req.ListenPortMax, req.AllowedIPs, req.NetworkInterface,
		req.DaemonSSLCert, req.DaemonSSLKey, req.AutoConnect, req.ConnectionRetries, req.HeartbeatInterval,
		req.CPUCores, req.MemoryOverallocate, req.DiskOverallocate, req.ReservedMemoryMB, req.ReservedDiskMB,
		req.DefaultAllocationIP, req.AllocationPortMin, req.AllocationPortMax, req.AutoAllocate,
		req.BackupDirectory, req.TransferDirectory, req.MountPoints, req.TokenRotationPolicy,
		req.FirewallRules, req.TLSSetting, req.EnableHealthChecks, req.EnableMetrics, req.PrometheusEndpoint,
		req.AlertThresholdCPU, req.AlertThresholdMem, req.AlertThresholdDisk,
		req.MaintenanceMessage, req.DrainBeforeMaint, req.Labels, req.ClusterGroupID,
		req.Public,
		req.DaemonSFTPAlias, req.DaemonConnect, req.CPUOverallocate, req.Tags)
	if err != nil {
		return Node{}, "", err
	}
	_ = s.AppendAudit(ctx, actorID, "node created", "node", &id, fmt.Sprintf(`{"baseUrl":"%s","tokenId":"%s"}`, req.BaseURL, tokenID))
	node, err := s.GetNode(ctx, id)
	if err != nil {
		return Node{}, "", err
	}
	return node, tokenID + "." + token, nil
}

func (s *Store) UpdateNode(ctx context.Context, nodeID string, req UpdateNodeRequest, actorID *string) (Node, error) {
	req = normalizeNodeUpdate(req)
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Region) == "" || strings.TrimSpace(req.BaseURL) == "" {
		return Node{}, errors.New("name, region, and baseUrl are required")
	}
	if strings.TrimSpace(req.LocationID) != "" {
		location, err := s.GetLocation(ctx, req.LocationID)
		if err != nil {
			return Node{}, err
		}
		req.LocationID = location.ID
		req.Region = location.Short
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "offline"
	}
	desiredState := strings.TrimSpace(string(req.DesiredState))
	if desiredState == "" {
		desiredState = "active"
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE nodes
		SET name = $1, region = $2, region_id = $3, base_url = $4, fqdn = $5, scheme = $6, behind_proxy = $7,
		    maintenance_mode = $8, draining = $9, desired_state = $10, memory_mb = $11, disk_mb = $12, upload_size_mb = $13,
		    daemon_base = $14, daemon_listen = $15, daemon_sftp = $16, status = $17,
		    display_name = $19, public_hostname = $20,
		    listen_port_min = $21, listen_port_max = $22, allowed_ips = $23, network_interface = $24,
		    daemon_ssl_cert = $25, daemon_ssl_key = $26,
		    auto_connect = $27, connection_retries = $28, heartbeat_interval = $29,
		    cpu_cores = $30, memory_overallocate = $31, disk_overallocate = $32,
		    reserved_memory_mb = $33, reserved_disk_mb = $34,
		    default_allocation_ip = $35, allocation_port_min = $36, allocation_port_max = $37,
		    auto_allocate = $38,
		    backup_directory = $39, transfer_directory = $40,
		    mount_points = $41, token_rotation_policy = $42,
		    firewall_rules = $43, tls_setting = $44,
		    enable_health_checks = $45, enable_metrics = $46, prometheus_endpoint = $47,
		    alert_threshold_cpu = $48, alert_threshold_memory = $49, alert_threshold_disk = $50,
		    maintenance_message = $51, drain_before_maintenance = $52,
		public = $56,
		labels = $53, cluster_group_id = $54, location_id = $55,
		daemon_sftp_alias = $57, daemon_connect = $58, cpu_overallocate = $59, tags = $60
	WHERE id = $18
`, req.Name, req.Region, nullableUUID(req.RegionID), req.BaseURL, req.FQDN, req.Scheme, req.BehindProxy, req.Maintenance,
		req.Draining, desiredState,
		req.MemoryMB, req.DiskMB, req.UploadSizeMB, req.DaemonBase, req.DaemonListen, req.DaemonSFTP,
		status, nodeID,
		req.DisplayName, req.PublicHostname,
		req.ListenPortMin, req.ListenPortMax, req.AllowedIPs, req.NetworkInterface,
		req.DaemonSSLCert, req.DaemonSSLKey,
		req.AutoConnect, req.ConnectionRetries, req.HeartbeatInterval,
		req.CPUCores, req.MemoryOverallocate, req.DiskOverallocate,
		req.ReservedMemoryMB, req.ReservedDiskMB,
		req.DefaultAllocationIP, req.AllocationPortMin, req.AllocationPortMax,
		req.AutoAllocate,
		req.BackupDirectory, req.TransferDirectory,
		req.MountPoints, req.TokenRotationPolicy,
		req.FirewallRules, req.TLSSetting,
		req.EnableHealthChecks, req.EnableMetrics, req.PrometheusEndpoint,
		req.AlertThresholdCPU, req.AlertThresholdMemory, req.AlertThresholdDisk,
		req.MaintenanceMessage, req.DrainBeforeMaint,
		req.Labels, req.ClusterGroupID, nullableUUID(req.LocationID),
		req.Public,
		req.DaemonSFTPAlias, req.DaemonConnect, req.CPUOverallocate, req.Tags)
	if err != nil {
		return Node{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return Node{}, errors.New("node not found")
	}
	_ = s.AppendAudit(ctx, actorID, "node updated", "node", &nodeID, fmt.Sprintf(`{"status":"%s"}`, status))
	return s.GetNode(ctx, nodeID)
}

// PatchNode updates only explicitly supplied fields. This is intentionally separate
// from the legacy full update path so a PATCH can never reset advanced settings.
func (s *Store) PatchNode(ctx context.Context, nodeID string, patch NodePatch, actorID *string) (Node, error) {
	current, err := s.GetNode(ctx, nodeID)
	if err != nil {
		return Node{}, errors.New("node not found")
	}
	name, baseURL, fqdn, scheme := current.Name, current.BaseURL, current.FQDN, current.Scheme
	memoryMB, diskMB, uploadMB := current.MemoryMB, current.DiskMB, current.UploadSizeMB
	listen, sftp := current.DaemonListen, current.DaemonSFTP
	displayName, publicHostname := current.DisplayName, current.PublicHostname
	if patch.Name != nil {
		name = strings.TrimSpace(*patch.Name)
	}
	if patch.BaseURL != nil {
		baseURL = strings.TrimRight(strings.TrimSpace(*patch.BaseURL), "/")
	}
	if patch.FQDN != nil {
		fqdn = strings.TrimSpace(*patch.FQDN)
	}
	if patch.Scheme != nil {
		scheme = strings.ToLower(strings.TrimSpace(*patch.Scheme))
	}
	if patch.MemoryMB != nil {
		memoryMB = *patch.MemoryMB
	}
	if patch.DiskMB != nil {
		diskMB = *patch.DiskMB
	}
	if patch.UploadSizeMB != nil {
		uploadMB = *patch.UploadSizeMB
	}
	if patch.DaemonListen != nil {
		listen = *patch.DaemonListen
	}
	if patch.DaemonSFTP != nil {
		sftp = *patch.DaemonSFTP
	}
	if patch.DisplayName != nil {
		displayName = strings.TrimSpace(*patch.DisplayName)
	}
	if patch.PublicHostname != nil {
		publicHostname = strings.TrimSpace(*patch.PublicHostname)
	}
	if err := validateNodeEndpoint(name, baseURL, fqdn, scheme, memoryMB, diskMB, uploadMB, listen, sftp); err != nil {
		return Node{}, err
	}
	if patch.DaemonBase != nil && strings.TrimSpace(*patch.DaemonBase) == "" {
		return Node{}, errors.New("daemonBase is required")
	}
	if patch.DesiredState != nil && *patch.DesiredState != NodeDesiredStateActive && *patch.DesiredState != NodeDesiredStateMaintenance && *patch.DesiredState != NodeDesiredStateDraining {
		return Node{}, errors.New("desiredState must be active, maintenance, or draining")
	}
	if patch.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*patch.Status))
		if status != "online" && status != "offline" && status != "degraded" {
			return Node{}, errors.New("status must be online, offline, or degraded")
		}
		*patch.Status = status
	}

	sets, args := []string{}, []any{}
	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if patch.Name != nil {
		add("name", name)
	}
	if patch.Description != nil {
		add("description", strings.TrimSpace(*patch.Description))
	}
	if patch.LocationID != nil {
		locationID := strings.TrimSpace(*patch.LocationID)
		if locationID == "" {
			return Node{}, errors.New("locationId is required")
		}
		location, err := s.GetLocation(ctx, locationID)
		if err != nil {
			return Node{}, err
		}
		add("location_id", location.ID)
		add("region", location.Short)
	}
	if patch.BaseURL != nil {
		add("base_url", baseURL)
	}
	if patch.FQDN != nil {
		add("fqdn", fqdn)
	}
	if patch.Scheme != nil {
		add("scheme", scheme)
	}
	if patch.BehindProxy != nil {
		add("behind_proxy", *patch.BehindProxy)
	}
	if patch.DesiredState != nil {
		// Desired state is the lifecycle source of truth. Keep the legacy
		// maintenance/draining columns synchronized for existing consumers.
		add("desired_state", *patch.DesiredState)
		add("maintenance_mode", *patch.DesiredState == NodeDesiredStateMaintenance)
		add("draining", *patch.DesiredState == NodeDesiredStateDraining)
	} else {
		if patch.Maintenance != nil {
			add("maintenance_mode", *patch.Maintenance)
		}
		if patch.Draining != nil {
			add("draining", *patch.Draining)
		}
	}
	if patch.MemoryMB != nil {
		add("memory_mb", *patch.MemoryMB)
	}
	if patch.DiskMB != nil {
		add("disk_mb", *patch.DiskMB)
	}
	if patch.UploadSizeMB != nil {
		add("upload_size_mb", *patch.UploadSizeMB)
	}
	if patch.DaemonBase != nil {
		add("daemon_base", strings.TrimSpace(*patch.DaemonBase))
	}
	if patch.DaemonListen != nil {
		add("daemon_listen", *patch.DaemonListen)
	}
	if patch.DaemonSFTP != nil {
		add("daemon_sftp", *patch.DaemonSFTP)
	}
	if patch.MemoryOverallocate != nil {
		add("memory_overallocate", *patch.MemoryOverallocate)
	}
	if patch.DiskOverallocate != nil {
		add("disk_overallocate", *patch.DiskOverallocate)
	}
	if patch.CPUCores != nil {
		add("cpu_cores", *patch.CPUCores)
	}
	if patch.DaemonSFTPAlias != nil {
		add("daemon_sftp_alias", *patch.DaemonSFTPAlias)
	}
	if patch.DaemonConnect != nil {
		add("daemon_connect", *patch.DaemonConnect)
	}
	if patch.CPUOverallocate != nil {
		add("cpu_overallocate", *patch.CPUOverallocate)
	}
	if patch.Tags != nil {
		add("tags", *patch.Tags)
	}
	if patch.DisplayName != nil {
		add("display_name", displayName)
	}
	if patch.PublicHostname != nil {
		add("public_hostname", publicHostname)
	}
	if patch.Public != nil {
		add("public", *patch.Public)
	}
	if patch.Status != nil {
		add("status", strings.TrimSpace(*patch.Status))
	}
	if len(sets) == 0 {
		return current, nil
	}
	args = append(args, nodeID)
	tag, err := s.db.Exec(ctx, "UPDATE nodes SET "+strings.Join(sets, ", ")+fmt.Sprintf(" WHERE id = $%d", len(args)), args...)
	if err != nil {
		return Node{}, err
	}
	if tag.RowsAffected() == 0 {
		return Node{}, errors.New("node not found")
	}
	_ = s.AppendAudit(ctx, actorID, "node updated", "node", &nodeID, `{"patch":true}`)
	return s.GetNode(ctx, nodeID)
}

func validateNodeEndpoint(name, baseURL, fqdn, scheme string, memoryMB, diskMB, uploadMB, listen, sftp int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 100 {
		return errors.New("name must be at most 100 characters")
	}
	for _, r := range name {
		if r > 126 {
			return errors.New("name must contain only printable ASCII characters")
		}
	}
	if scheme != "http" && scheme != "https" {
		return errors.New("scheme must be http or https")
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != scheme || u.Hostname() == "" || u.User != nil {
		return errors.New("baseUrl must be an absolute http or https URL without credentials")
	}
	fqdn = strings.TrimSpace(fqdn)
	if fqdn == "" || !strings.EqualFold(u.Hostname(), fqdn) {
		return errors.New("fqdn must match the baseUrl host")
	}
	lower := strings.ToLower(fqdn)
	if lower == "localhost" || lower == "0.0.0.0" || lower == "127.0.0.1" || strings.HasPrefix(lower, "127.") {
		return errors.New("fqdn must be a publicly routable hostname, not a loopback address")
	}
	if memoryMB < 0 || diskMB < 0 || uploadMB < 0 {
		return errors.New("resource limits cannot be negative")
	}
	if listen < 1 || listen > 65535 || sftp < 1 || sftp > 65535 || listen == sftp {
		return errors.New("daemon and SFTP ports must be distinct values between 1 and 65535")
	}
	return nil
}

func (s *Store) DeleteNode(ctx context.Context, nodeID string, actorID *string) error {
	var servers, allocations int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM servers WHERE node_id = $1`, nodeID).Scan(&servers); err != nil {
		return err
	}
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM allocations WHERE node_id = $1`, nodeID).Scan(&allocations); err != nil {
		return err
	}
	if servers > 0 || allocations > 0 {
		return fmt.Errorf("node has %d server(s) and %d allocation(s); evacuate or remove them before deletion", servers, allocations)
	}
	commandTag, err := s.db.Exec(ctx, `DELETE FROM nodes WHERE id = $1`, nodeID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("node not found")
	}
	return s.AppendAudit(ctx, actorID, "node deleted", "node", &nodeID, `{"reason":"admin delete"}`)
}

func (s *Store) RotateNodeToken(ctx context.Context, nodeID string, actorID *string) (string, error) {
	tokenID := newDaemonTokenID()
	token := newDaemonToken()
	encryptedToken, err := s.encryptSecret(token, secretAAD("nodes", nodeID, "daemon_token"))
	if err != nil {
		return "", err
	}
	tokenHash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", errors.New("hash node credential")
	}
	commandTag, err := s.db.Exec(ctx, `UPDATE nodes SET token_hash = $1, daemon_token_id = $2, daemon_token = '', daemon_token_encrypted = $3 WHERE id = $4`, string(tokenHash), tokenID, encryptedToken, nodeID)
	if err != nil {
		return "", err
	}
	if commandTag.RowsAffected() == 0 {
		return "", errors.New("node not found")
	}
	_ = s.AppendAudit(ctx, actorID, "node token rotated", "node", &nodeID, `{"action":"rotate-token"}`)
	return tokenID + "." + token, nil
}

func (s *Store) VerifyNodeToken(ctx context.Context, nodeID, token string) (bool, error) {
	if strings.TrimSpace(nodeID) == "" || strings.TrimSpace(token) == "" {
		return false, nil
	}
	var stored, tokenID, plaintextToken, encryptedToken string
	if err := s.db.QueryRow(ctx, `SELECT token_hash, COALESCE(daemon_token_id, ''), COALESCE(daemon_token, ''), COALESCE(daemon_token_encrypted, '') FROM nodes WHERE id = $1`, nodeID).Scan(&stored, &tokenID, &plaintextToken, &encryptedToken); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if strings.Contains(token, ".") && tokenID != "" {
		parts := strings.SplitN(token, ".", 2)
		if parts[0] != tokenID {
			return false, nil
		}
		if strings.HasPrefix(stored, "$2") {
			return bcrypt.CompareHashAndPassword([]byte(stored), []byte(parts[1])) == nil, nil
		}
		storedToken, err := s.decryptSecret(encryptedToken, plaintextToken, secretAAD("nodes", nodeID, "daemon_token"))
		return err == nil && hmac.Equal([]byte(parts[1]), []byte(storedToken)), err
	}
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(token)) == nil, nil
	}
	return stored == token, nil
}

func (s *Store) AuthenticateRemoteNode(ctx context.Context, bearer string) (Node, error) {
	parts := strings.SplitN(strings.TrimSpace(bearer), ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Node{}, errors.New("invalid daemon authorization")
	}
	var nodeID, storedHash string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, token_hash
		FROM nodes
		WHERE daemon_token_id = $1
	`, parts[0]).Scan(&nodeID, &storedHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Node{}, errors.New("invalid daemon authorization")
		}
		return Node{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(parts[1])) != nil {
		return Node{}, errors.New("invalid daemon authorization")
	}
	return s.GetNode(ctx, nodeID)
}

func (s *Store) UpdateNodeHeartbeat(ctx context.Context, nodeID string, req NodeHeartbeatRequest) (Node, error) {
	status := "online"
	if strings.TrimSpace(req.Error) != "" || strings.ToLower(strings.TrimSpace(req.DockerStatus)) == "error" || strings.ToLower(strings.TrimSpace(req.RuntimeStatus)) == "error" {
		status = "degraded"
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE nodes
		SET status = $1,
		    last_seen_at = now(),
		    version = NULLIF($2, ''),
		    os = NULLIF($3, ''),
		    architecture = NULLIF($4, ''),
		    cpu_threads = NULLIF($5, 0),
		    node_memory_mb = NULLIF($6, 0),
		    node_disk_mb = NULLIF($7, 0),
		    docker_status = NULLIF($8, ''),
		    runtime_status = NULLIF($9, ''),
		    runtime_provider = NULLIF($10, ''),
		    heartbeat_error = NULLIF($11, '')
		WHERE id = $12
	`, status, req.Version, req.OS, req.Architecture, req.CPUThreads, req.MemoryMB, req.DiskMB, req.DockerStatus, req.RuntimeStatus, req.RuntimeProvider, req.Error, nodeID)
	if err != nil {
		return Node{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return Node{}, errors.New("node not found")
	}
	return s.GetNode(ctx, nodeID)
}

func (s *Store) RemoteServerConfigurations(ctx context.Context, nodeID string) ([]ServerProvisionTarget, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id::text, s.egg_id::text, s.name, n.base_url,
		       n.id::text,
		       COALESCE(n.daemon_token_id, ''),
		       COALESCE(n.daemon_token, ''),
		       COALESCE(n.daemon_token_encrypted, ''),
		       COALESCE(NULLIF(s.docker_image, ''), (SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1), ''),
		       COALESCE(NULLIF(s.startup_command, ''), e.startup),
		       e.install_script, e.install_container, e.install_entrypoint, e.config::text, e.file_denylist::text,
		       s.memory_mb, s.swap_mb, s.cpu_shares, s.cpu_limit, s.disk_mb, s.io_weight,
		       COALESCE(s.threads, ''), s.oom_disabled, host(a.ip), a.port,
		       s.suspended, s.installed, s.status,
		       s.primary_allocation_id::text
		FROM servers s
		JOIN nodes n ON n.id = s.node_id
		JOIN eggs e ON e.id = s.egg_id
		LEFT JOIN allocations a ON a.id = s.primary_allocation_id
		WHERE s.node_id = $1 AND s.status <> 'deleted'
		ORDER BY s.created_at
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type serverInfo struct {
		serverID          string
		eggID             string
		name              string
		nodeURL           string
		rowNodeID         string
		tokenID           string
		daemonToken       string
		daemonTokenEnc    string
		image             string
		startupCommand    string
		installScript     string
		installContainer  string
		installEntrypoint string
		configJSON        string
		fileDenylist      string
		memoryMB          int64
		swapMB            int64
		cpuShares         int64
		cpuLimit          int64
		diskMB            int64
		ioWeight          int64
		threads           string
		oomDisabled       bool
		allocationIP      sql.NullString
		allocationPort    sql.NullInt64
		suspended         bool
		installed         bool
		status            string
		primaryAllocID    sql.NullString
	}

	var infos []serverInfo
	for rows.Next() {
		var info serverInfo
		if err := rows.Scan(
			&info.serverID,
			&info.eggID,
			&info.name,
			&info.nodeURL,
			&info.rowNodeID,
			&info.tokenID,
			&info.daemonToken,
			&info.daemonTokenEnc,
			&info.image,
			&info.startupCommand,
			&info.installScript,
			&info.installContainer,
			&info.installEntrypoint,
			&info.configJSON,
			&info.fileDenylist,
			&info.memoryMB,
			&info.swapMB,
			&info.cpuShares,
			&info.cpuLimit,
			&info.diskMB,
			&info.ioWeight,
			&info.threads,
			&info.oomDisabled,
			&info.allocationIP,
			&info.allocationPort,
			&info.suspended,
			&info.installed,
			&info.status,
			&info.primaryAllocID,
		); err != nil {
			return nil, err
		}
		infos = append(infos, info)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	if len(infos) == 0 {
		return []ServerProvisionTarget{}, nil
	}

	// Decrypt node token (same node for all servers).
	token, err := s.decryptSecret(infos[0].daemonTokenEnc, infos[0].daemonToken, secretAAD("nodes", infos[0].rowNodeID, "daemon_token"))
	if err != nil {
		return nil, err
	}
	nodeToken := ""
	if infos[0].tokenID != "" && token != "" {
		nodeToken = infos[0].tokenID + "." + token
	}

	// Batch-fetch environment variables for all servers on this node.
	envByServer := map[string]map[string]string{}
	{
		variableRows, err := s.db.Query(ctx, `
			SELECT srv.id::text, ev.env_variable, COALESCE(sv.variable_value, ev.default_value)
			FROM servers srv
			JOIN egg_variables ev ON ev.egg_id = srv.egg_id
			LEFT JOIN server_variables sv ON sv.server_id = srv.id AND sv.variable_id = ev.id
			WHERE srv.node_id = $1 AND srv.status <> 'deleted'
			ORDER BY ev.env_variable
		`, nodeID)
		if err != nil {
			return nil, err
		}
		for variableRows.Next() {
			var sid, key, value string
			if err := variableRows.Scan(&sid, &key, &value); err != nil {
				variableRows.Close()
				return nil, err
			}
			if _, ok := envByServer[sid]; !ok {
				envByServer[sid] = map[string]string{}
			}
			envByServer[sid][key] = value
		}
		if err := variableRows.Err(); err != nil {
			variableRows.Close()
			return nil, err
		}
		variableRows.Close()
	}

	// Batch-fetch mounts for all servers on this node.
	mountsByServer := map[string][]ServerMount{}
	{
		mountRows, err := s.db.Query(ctx, `
			SELECT ms.server_id::text, m.id::text, m.name, m.source, m.target, m.read_only
			FROM mounts m
			JOIN mount_server ms ON ms.mount_id = m.id
			WHERE ms.server_id IN (SELECT id FROM servers WHERE node_id = $1 AND status <> 'deleted')
			ORDER BY m.name
		`, nodeID)
		if err != nil {
			return nil, err
		}
		for mountRows.Next() {
			var sid string
			var mount ServerMount
			if err := mountRows.Scan(&sid, &mount.ID, &mount.Name, &mount.Source, &mount.Target, &mount.ReadOnly); err != nil {
				mountRows.Close()
				return nil, err
			}
			mountsByServer[sid] = append(mountsByServer[sid], mount)
		}
		if err := mountRows.Err(); err != nil {
			mountRows.Close()
			return nil, err
		}
		mountRows.Close()
	}

	// Batch-fetch allocations for all servers on this node.
	allocsByServer := map[string][]ServerRuntimeAllocation{}
	{
		allocRows, err := s.db.Query(ctx, `
			SELECT a.id::text, a.server_id::text, host(a.ip), a.port, a.container_port, a.protocol
			FROM allocations a
			WHERE a.server_id IN (SELECT id FROM servers WHERE node_id = $1 AND status <> 'deleted')
			ORDER BY a.port
		`, nodeID)
		if err != nil {
			return nil, err
		}
		for allocRows.Next() {
			var sid string
			var alloc ServerRuntimeAllocation
			if err := allocRows.Scan(&alloc.ID, &sid, &alloc.IP, &alloc.Port, &alloc.ContainerPort, &alloc.Protocol); err != nil {
				allocRows.Close()
				return nil, err
			}
			allocsByServer[sid] = append(allocsByServer[sid], alloc)
		}
		if err := allocRows.Err(); err != nil {
			allocRows.Close()
			return nil, err
		}
		allocRows.Close()
	}

	targets := make([]ServerProvisionTarget, 0, len(infos))
	for _, info := range infos {
		env := envByServer[info.serverID]
		if env == nil {
			env = map[string]string{}
		}

		target := ServerProvisionTarget{
			ServerID:          info.serverID,
			EggID:             info.eggID,
			Name:              info.name,
			NodeURL:           info.nodeURL,
			NodeToken:         nodeToken,
			Image:             info.image,
			StartupCommand:    info.startupCommand,
			InstallScript:     info.installScript,
			InstallContainer:  info.installContainer,
			InstallEntrypoint: info.installEntrypoint,
			ConfigJSON:        info.configJSON,
			FileDenylist:      info.fileDenylist,
			Environment:       env,
			Mounts:            mountsByServer[info.serverID],
			MemoryMB:          info.memoryMB,
			SwapMB:            info.swapMB,
			CPUShares:         info.cpuShares,
			CPULimit:          info.cpuLimit,
			DiskMB:            info.diskMB,
			IOWeight:          info.ioWeight,
			Threads:           info.threads,
			OOMDisabled:       info.oomDisabled,
			Suspended:         info.suspended,
			Installed:         info.installed,
			Status:            info.status,
		}
		if info.allocationIP.Valid {
			target.AllocationIP = info.allocationIP.String
		}
		if info.allocationPort.Valid {
			target.AllocationPort = int(info.allocationPort.Int64)
		}
		target.StartupCommand = resolveStartupCommand(target.StartupCommand, target.Environment)

		allocs := allocsByServer[info.serverID]
		if info.primaryAllocID.Valid {
			sort.Slice(allocs, func(i, j int) bool {
				if allocs[i].ID == info.primaryAllocID.String {
					return true
				}
				if allocs[j].ID == info.primaryAllocID.String {
					return false
				}
				return allocs[i].Port < allocs[j].Port
			})
		} else {
			sort.Slice(allocs, func(i, j int) bool {
				return allocs[i].Port < allocs[j].Port
			})
		}
		target.Allocations = allocs

		targets = append(targets, target)
	}

	return targets, nil
}

func (s *Store) ResetNodeServerStates(ctx context.Context, nodeID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE servers
		SET status = 'stopped',
		    installed = CASE WHEN status = 'installing' THEN true ELSE installed END,
		    install_completed_at = CASE WHEN status = 'installing' THEN now() ELSE install_completed_at END
		WHERE node_id = $1
		  AND status IN ('installing', 'restoring_backup')
	`, nodeID)
	return err
}

func (s *Store) ServerBelongsToNode(ctx context.Context, serverID, nodeID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM servers WHERE id = $1 AND node_id = $2)`, serverID, nodeID).Scan(&exists)
	return exists, err
}

type NodeConfiguration struct {
	Debug   bool   `json:"debug"`
	UUID    string `json:"uuid"`
	TokenID string `json:"token_id"`
	// Token is intentionally never populated by this read endpoint. Complete
	// credentials are revealed only by creation and rotation responses.
	Token          string         `json:"token,omitempty"`
	API            map[string]any `json:"api"`
	System         map[string]any `json:"system"`
	Remote         string         `json:"remote"`
	AllowedMounts  []string       `json:"allowed_mounts"`
	AllowedOrigins []string       `json:"allowed_origins"`
	RemoteQuery    map[string]int `json:"remote_query"`
}

func (s *Store) NodeConfiguration(ctx context.Context, nodeID, panelURL string) (NodeConfiguration, error) {
	var node Node
	err := s.db.QueryRow(ctx, `
		SELECT id::text, COALESCE(uuid, id)::text, name, region, base_url,
		       COALESCE(fqdn, ''), scheme, behind_proxy, status, maintenance_mode,
		       memory_mb, disk_mb, upload_size_mb, daemon_base, daemon_listen, daemon_sftp,
		       COALESCE(daemon_token_id, '')
		FROM nodes
		WHERE id = $1
	`, nodeID).Scan(
		&node.ID, &node.UUID, &node.Name, &node.Region, &node.BaseURL,
		&node.FQDN, &node.Scheme, &node.BehindProxy, &node.Status, &node.Maintenance,
		&node.MemoryMB, &node.DiskMB, &node.UploadSizeMB, &node.DaemonBase, &node.DaemonListen,
		&node.DaemonSFTP, &node.TokenID,
	)
	if err != nil {
		return NodeConfiguration{}, err
	}
	certFQDN := strings.ToLower(node.FQDN)
	allowedMounts, err := s.AllowedMountSourcesForNode(ctx, nodeID)
	if err != nil {
		return NodeConfiguration{}, err
	}
	return NodeConfiguration{
		Debug:   false,
		UUID:    node.UUID,
		TokenID: node.TokenID,
		API: map[string]any{
			"host": "0.0.0.0",
			"port": node.DaemonListen,
			"ssl": map[string]any{
				"enabled": !node.BehindProxy && node.Scheme == "https",
				"cert":    "/etc/letsencrypt/live/" + certFQDN + "/fullchain.pem",
				"key":     "/etc/letsencrypt/live/" + certFQDN + "/privkey.pem",
			},
			"upload_limit": node.UploadSizeMB,
		},
		System: map[string]any{
			"data": node.DaemonBase,
			"sftp": map[string]any{
				"bind_port": node.DaemonSFTP,
			},
		},
		Remote:         strings.TrimRight(panelURL, "/"),
		AllowedMounts:  allowedMounts,
		AllowedOrigins: []string{strings.TrimRight(panelURL, "/")},
		RemoteQuery: map[string]int{
			"timeout":               30,
			"boot_servers_per_page": 50,
		},
	}, nil
}

func newDaemonTokenID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
}

func newDaemonToken() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "") + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func nullableUUID(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func normalizeNodeCreate(req CreateNodeRequest) CreateNodeRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.Region = strings.TrimSpace(req.Region)
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	req.FQDN = strings.TrimSpace(req.FQDN)
	req.Scheme = normalizeScheme(req.Scheme)
	req.PublicHostname = strings.TrimSpace(req.PublicHostname)
	req.NetworkInterface = strings.TrimSpace(req.NetworkInterface)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.FQDN == "" {
		req.FQDN = fqdnFromBaseURL(req.BaseURL)
	}
	if req.PublicHostname == "" {
		req.PublicHostname = req.FQDN
	}
	if req.DaemonListen == 0 {
		req.DaemonListen = portFromBaseURL(req.BaseURL, req.Scheme)
	}
	if req.DaemonSFTP == 0 {
		req.DaemonSFTP = 2022
	}
	if req.UploadSizeMB == 0 {
		req.UploadSizeMB = 100
	}
	if req.DaemonBase == "" {
		req.DaemonBase = "/var/lib/forge/volumes"
	}
	if req.AllowedIPs == nil {
		req.AllowedIPs = []string{}
	}
	if req.ConnectionRetries == 0 {
		req.ConnectionRetries = 3
	}
	if req.HeartbeatInterval == 0 {
		req.HeartbeatInterval = 15
	}
	if req.DefaultAllocationIP == "" {
		req.DefaultAllocationIP = "0.0.0.0"
	}
	if req.AllocationPortMin == 0 {
		req.AllocationPortMin = 25565
	}
	if req.AllocationPortMax == 0 {
		req.AllocationPortMax = 26565
	}
	if req.BackupDirectory == "" {
		req.BackupDirectory = req.DaemonBase + "/backups"
	}
	if req.TransferDirectory == "" {
		req.TransferDirectory = req.DaemonBase + "/transfers"
	}
	if req.TokenRotationPolicy == "" {
		req.TokenRotationPolicy = "manual"
	}
	if req.TLSSetting == "" {
		req.TLSSetting = "auto"
	}
	if req.AlertThresholdCPU == 0 {
		req.AlertThresholdCPU = 90
	}
	if req.AlertThresholdMem == 0 {
		req.AlertThresholdMem = 90
	}
	if req.AlertThresholdDisk == 0 {
		req.AlertThresholdDisk = 90
	}
	if req.Labels == nil {
		req.Labels = []LabelPair{}
	}
	if req.DaemonConnect == 0 {
		req.DaemonConnect = 8080
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	return req
}

func normalizeNodeUpdate(req UpdateNodeRequest) UpdateNodeRequest {
	create := normalizeNodeCreate(CreateNodeRequest{
		Name: req.Name, Region: req.Region, BaseURL: req.BaseURL, FQDN: req.FQDN, Scheme: req.Scheme,
		BehindProxy: req.BehindProxy, Public: req.Public, MemoryMB: req.MemoryMB, DiskMB: req.DiskMB, UploadSizeMB: req.UploadSizeMB,
		DaemonBase: req.DaemonBase, DaemonListen: req.DaemonListen, DaemonSFTP: req.DaemonSFTP,
		DisplayName: req.DisplayName, PublicHostname: req.PublicHostname,
		ListenPortMin: req.ListenPortMin, ListenPortMax: req.ListenPortMax,
		AllowedIPs: req.AllowedIPs, NetworkInterface: req.NetworkInterface,
		DaemonSSLCert: req.DaemonSSLCert, DaemonSSLKey: req.DaemonSSLKey,
		AutoConnect: req.AutoConnect, ConnectionRetries: req.ConnectionRetries, HeartbeatInterval: req.HeartbeatInterval,
		CPUCores: req.CPUCores, MemoryOverallocate: req.MemoryOverallocate, DiskOverallocate: req.DiskOverallocate,
		ReservedMemoryMB: req.ReservedMemoryMB, ReservedDiskMB: req.ReservedDiskMB,
		DefaultAllocationIP: req.DefaultAllocationIP,
		AllocationPortMin:   req.AllocationPortMin, AllocationPortMax: req.AllocationPortMax,
		AutoAllocate:    req.AutoAllocate,
		BackupDirectory: req.BackupDirectory, TransferDirectory: req.TransferDirectory,
		MountPoints: req.MountPoints, TokenRotationPolicy: req.TokenRotationPolicy,
		FirewallRules: req.FirewallRules, TLSSetting: req.TLSSetting,
		EnableHealthChecks: req.EnableHealthChecks, EnableMetrics: req.EnableMetrics,
		PrometheusEndpoint: req.PrometheusEndpoint,
		AlertThresholdCPU:  req.AlertThresholdCPU, AlertThresholdMem: req.AlertThresholdMemory, AlertThresholdDisk: req.AlertThresholdDisk,
		MaintenanceMessage: req.MaintenanceMessage, DrainBeforeMaint: req.DrainBeforeMaint,
		Labels: req.Labels, ClusterGroupID: req.ClusterGroupID,
	})
	req.Name = create.Name
	req.Region = create.Region
	req.BaseURL = create.BaseURL
	req.FQDN = create.FQDN
	req.Scheme = create.Scheme
	req.MemoryMB = create.MemoryMB
	req.DiskMB = create.DiskMB
	req.UploadSizeMB = create.UploadSizeMB
	req.DaemonBase = create.DaemonBase
	req.DaemonListen = create.DaemonListen
	req.DaemonSFTP = create.DaemonSFTP
	req.DisplayName = create.DisplayName
	req.PublicHostname = create.PublicHostname
	req.ListenPortMin = create.ListenPortMin
	req.ListenPortMax = create.ListenPortMax
	req.AllowedIPs = create.AllowedIPs
	req.NetworkInterface = create.NetworkInterface
	req.DaemonSSLCert = create.DaemonSSLCert
	req.DaemonSSLKey = create.DaemonSSLKey
	req.AutoConnect = create.AutoConnect
	req.ConnectionRetries = create.ConnectionRetries
	req.HeartbeatInterval = create.HeartbeatInterval
	req.CPUCores = create.CPUCores
	req.MemoryOverallocate = create.MemoryOverallocate
	req.DiskOverallocate = create.DiskOverallocate
	req.ReservedMemoryMB = create.ReservedMemoryMB
	req.ReservedDiskMB = create.ReservedDiskMB
	req.DefaultAllocationIP = create.DefaultAllocationIP
	req.AllocationPortMin = create.AllocationPortMin
	req.AllocationPortMax = create.AllocationPortMax
	req.AutoAllocate = create.AutoAllocate
	req.BackupDirectory = create.BackupDirectory
	req.TransferDirectory = create.TransferDirectory
	req.MountPoints = create.MountPoints
	req.TokenRotationPolicy = create.TokenRotationPolicy
	req.FirewallRules = create.FirewallRules
	req.TLSSetting = create.TLSSetting
	req.EnableHealthChecks = create.EnableHealthChecks
	req.EnableMetrics = create.EnableMetrics
	req.PrometheusEndpoint = create.PrometheusEndpoint
	req.AlertThresholdCPU = create.AlertThresholdCPU
	req.AlertThresholdMemory = create.AlertThresholdMem
	req.AlertThresholdDisk = create.AlertThresholdDisk
	req.MaintenanceMessage = create.MaintenanceMessage
	req.DrainBeforeMaint = create.DrainBeforeMaint
	req.Labels = create.Labels
	req.ClusterGroupID = create.ClusterGroupID
	req.Public = create.Public
	req.DaemonSFTPAlias = create.DaemonSFTPAlias
	req.DaemonConnect = create.DaemonConnect
	req.CPUOverallocate = create.CPUOverallocate
	req.Tags = create.Tags
	return req
}

func normalizeScheme(scheme string) string {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme == "" {
		return "http"
	}
	return scheme
}

func fqdnFromBaseURL(baseURL string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	if index := strings.Index(trimmed, "/"); index >= 0 {
		trimmed = trimmed[:index]
	}
	if index := strings.Index(trimmed, ":"); index >= 0 {
		trimmed = trimmed[:index]
	}
	return trimmed
}

func portFromBaseURL(baseURL, scheme string) int {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
	if index := strings.Index(trimmed, "/"); index >= 0 {
		trimmed = trimmed[:index]
	}
	if index := strings.LastIndex(trimmed, ":"); index >= 0 && index+1 < len(trimmed) {
		var port int
		if _, err := fmt.Sscanf(trimmed[index+1:], "%d", &port); err == nil && port > 0 {
			return port
		}
	}
	if scheme == "https" {
		return 443
	}
	return 8080
}

func (s *Store) ListServersForNode(ctx context.Context, nodeID string) ([]Server, error) {
	rows, err := s.db.Query(ctx, `
		SELECT s.id::text, s.name, s.status, s.memory_mb, s.cpu_shares, s.disk_mb, n.name, u.email, e.name,
		       COALESCE(s.desired_state::text, ''), COALESCE(s.actual_state::text, '')
				FROM servers s
				JOIN nodes n ON n.id = s.node_id
				JOIN users u ON u.id = s.owner_id
				JOIN eggs e ON e.id = s.egg_id
		WHERE s.node_id = $1
		ORDER BY s.created_at DESC
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := []Server{}
	for rows.Next() {
		var server Server
		if err := rows.Scan(&server.ID, &server.Name, &server.Status, &server.MemoryMB, &server.CPUShares, &server.DiskMB, &server.Node, &server.Owner, &server.Template, &server.DesiredState, &server.ActualState); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Store) ListAllocationsForNode(ctx context.Context, nodeID string) ([]Allocation, error) {
	rows, err := s.db.Query(ctx, `
		SELECT a.id::text, n.name, s.name, a.ip::text, a.port, a.container_port, a.protocol, a.alias, COALESCE(a.notes, '')
		FROM allocations a
		JOIN nodes n ON n.id = a.node_id
		LEFT JOIN servers s ON s.id = a.server_id
		WHERE a.node_id = $1
		ORDER BY a.port
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allocations := []Allocation{}
	for rows.Next() {
		var allocation Allocation
		var server, alias sql.NullString
		if err := rows.Scan(&allocation.ID, &allocation.Node, &server, &allocation.IP, &allocation.Port, &allocation.ContainerPort, &allocation.Protocol, &alias, &allocation.Notes); err != nil {
			return nil, err
		}
		if server.Valid {
			allocation.Server = &server.String
		}
		if alias.Valid && alias.String != "" {
			allocation.Alias = &alias.String
		}
		allocations = append(allocations, allocation)
	}
	return allocations, rows.Err()
}
