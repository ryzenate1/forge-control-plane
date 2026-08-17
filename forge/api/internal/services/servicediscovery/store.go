package servicediscovery

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EndpointStore interface {
	SaveEndpoint(ctx context.Context, ep ServiceEndpoint) error
	ListEndpoints(ctx context.Context) ([]ServiceEndpoint, error)
	DeleteEndpoint(ctx context.Context, id string) error
}

type endpointStoreAdapter struct {
	pool *pgxpool.Pool
}

func NewEndpointStore(pool *pgxpool.Pool) EndpointStore {
	return &endpointStoreAdapter{pool: pool}
}

var _ EndpointStore = (*endpointStoreAdapter)(nil)

func (a *endpointStoreAdapter) SaveEndpoint(ctx context.Context, ep ServiceEndpoint) error {
	metadataJSON, err := json.Marshal(ep.Metadata)
	if err != nil {
		return err
	}

	_, err = a.pool.Exec(ctx, `
		INSERT INTO service_discovery_endpoints
			(id, service_name, service_id, node_id, node_name, region_id,
			 address, port, protocol, status, replica_index, tenant_id,
			 last_heartbeat, created_at, updated_at, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (id) DO UPDATE SET
			service_name   = EXCLUDED.service_name,
			service_id     = EXCLUDED.service_id,
			node_id        = EXCLUDED.node_id,
			node_name      = EXCLUDED.node_name,
			region_id      = EXCLUDED.region_id,
			address        = EXCLUDED.address,
			port           = EXCLUDED.port,
			protocol       = EXCLUDED.protocol,
			status         = EXCLUDED.status,
			replica_index  = EXCLUDED.replica_index,
			tenant_id      = EXCLUDED.tenant_id,
			last_heartbeat = EXCLUDED.last_heartbeat,
			updated_at     = EXCLUDED.updated_at,
			metadata       = EXCLUDED.metadata
	`,
		ep.ID, ep.ServiceName, ep.ServiceID, ep.NodeID, ep.NodeName, nullIfEmpty(ep.RegionID),
		ep.Address.String(), ep.Port, string(ep.Protocol), string(ep.Status),
		ep.ReplicaIndex, nullIfEmpty(ep.TenantID),
		ep.LastHeartbeat, ep.CreatedAt, ep.UpdatedAt, metadataJSON,
	)
	return err
}

func (a *endpointStoreAdapter) ListEndpoints(ctx context.Context) ([]ServiceEndpoint, error) {
	rows, err := a.pool.Query(ctx, `
		SELECT id, service_name, service_id, node_id, node_name, region_id,
		       address, port, protocol, status, replica_index, tenant_id,
		       last_heartbeat, created_at, updated_at, metadata
		FROM service_discovery_endpoints
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var endpoints []ServiceEndpoint
	for rows.Next() {
		ep, err := scanEndpoint(rows)
		if err != nil {
			return nil, err
		}
		endpoints = append(endpoints, ep)
	}
	return endpoints, rows.Err()
}

func (a *endpointStoreAdapter) DeleteEndpoint(ctx context.Context, id string) error {
	tag, err := a.pool.Exec(ctx, `DELETE FROM service_discovery_endpoints WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("endpoint not found")
	}
	return nil
}

type scanable interface {
	Scan(dest ...any) error
}

func scanEndpoint(row scanable) (ServiceEndpoint, error) {
	var (
		ep          ServiceEndpoint
		addressStr  string
		protocolStr string
		statusStr   string
		regionID    *string
		tenantID    *string
		metadataJSON []byte
	)

	err := row.Scan(
		&ep.ID, &ep.ServiceName, &ep.ServiceID, &ep.NodeID, &ep.NodeName,
		&regionID, &addressStr, &ep.Port, &protocolStr, &statusStr,
		&ep.ReplicaIndex, &tenantID,
		&ep.LastHeartbeat, &ep.CreatedAt, &ep.UpdatedAt, &metadataJSON,
	)
	if err != nil {
		return ep, err
	}

	ep.RegionID = coalesceStr(regionID)
	ep.TenantID = coalesceStr(tenantID)
	ep.Protocol = EndpointProtocol(protocolStr)
	ep.Status = EndpointStatus(statusStr)

	addr, err := netip.ParseAddr(addressStr)
	if err != nil {
		return ep, err
	}
	ep.Address = addr

	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &ep.Metadata)
	}
	if ep.Metadata == nil {
		ep.Metadata = make(map[string]string)
	}

	return ep, nil
}

func nullIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func coalesceStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
