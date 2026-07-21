package domains

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/events"

	"github.com/google/uuid"
)

type DomainRow struct {
	ID                string
	ServerID          string
	Domain            string
	Wildcard          bool
	Verified          bool
	VerifiedAt        *time.Time
	VerificationToken *string
	CreatedAt         time.Time
}

type DomainRecord struct {
	ID                string     `json:"id"`
	ServerID          string     `json:"serverId"`
	Domain            string     `json:"domain"`
	Wildcard          bool       `json:"wildcard"`
	Verified          bool       `json:"verified"`
	VerifiedAt        *time.Time `json:"verifiedAt,omitempty"`
	VerificationToken *string    `json:"verificationToken,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type VerificationResult struct {
	Domain      string   `json:"domain"`
	Verified    bool     `json:"verified"`
	DNSResolved bool     `json:"dnsResolved"`
	ExpectedIP  string   `json:"expectedIp,omitempty"`
	ResolvedIPs []string `json:"resolvedIps,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type DNSResult struct {
	Domain     string   `json:"domain"`
	Resolved   bool     `json:"resolved"`
	IPs        []string `json:"ips,omitempty"`
	ExpectedIP string   `json:"expectedIp,omitempty"`
	Match      bool     `json:"match"`
	Error      string   `json:"error,omitempty"`
}

type domainStore interface {
	CheckServerExists(ctx context.Context, id string) (bool, error)
	ListDomainsByServer(ctx context.Context, serverID string) ([]DomainRow, error)
	ListAllDomains(ctx context.Context) ([]DomainRow, error)
	GetDomain(ctx context.Context, id string) (*DomainRow, error)
	GetDomainByDomain(ctx context.Context, domain string) (*DomainRow, error)
	CreateDomain(ctx context.Context, row DomainRow) error
	UpdateDomainVerification(ctx context.Context, id string, verified bool, verifiedAt *time.Time) error
	SetDomainVerificationToken(ctx context.Context, id string, token string) error
	DeleteDomain(ctx context.Context, id string) error
	ListUnverifiedDomains(ctx context.Context) ([]DomainRow, error)
	ListVerifiedDomains(ctx context.Context) ([]DomainRow, error)
}

type caddyUpdater interface {
	UpdateDomainRoutes(ctx context.Context, domains []VerifiedDomainRoute) error
}

type VerifiedDomainRoute struct {
	Domain     string `json:"domain"`
	Wildcard   bool   `json:"wildcard"`
	ServerID   string `json:"serverId"`
	TargetHost string `json:"targetHost,omitempty"`
	TargetPort int    `json:"targetPort,omitempty"`
}

type NodeResolver interface {
	ResolveServerTarget(ctx context.Context, serverID string) (host string, port int, err error)
}

type Service struct {
	store         domainStore
	caddy         caddyUpdater
	publisher     events.Publisher
	mu            sync.RWMutex
	httpClient    *http.Client
	verfiyTicker  *time.Ticker
	done          chan struct{}
	panelIP       string
	reverifyEvery time.Duration
	nodeResolver  NodeResolver
}

func New(store domainStore, caddy caddyUpdater, panelIP string, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{
		store:         store,
		caddy:         caddy,
		publisher:     publisher,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		panelIP:       panelIP,
		reverifyEvery: 24 * time.Hour,
	}
}

func (s *Service) SetNodeResolver(r NodeResolver) {
	s.nodeResolver = r
}

func (s *Service) SetReverifyInterval(d time.Duration) {
	s.reverifyEvery = d
}

func (s *Service) StartReverify(ctx context.Context) {
	if s.verfiyTicker != nil {
		return
	}
	s.verfiyTicker = time.NewTicker(s.reverifyEvery)
	s.done = make(chan struct{})
	go func() {
		for {
			select {
			case <-s.verfiyTicker.C:
				s.reverifyAll(ctx)
			case <-s.done:
				return
			}
		}
	}()
}

func (s *Service) StopReverify() {
	if s.verfiyTicker != nil {
		s.verfiyTicker.Stop()
		s.verfiyTicker = nil
	}
	if s.done != nil {
		close(s.done)
		s.done = nil
	}
}

func (s *Service) AddDomain(ctx context.Context, serverID, domain string) (*DomainRecord, error) {
	if serverID == "" {
		return nil, fmt.Errorf("serverId is required")
	}
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}

	domain = strings.TrimSpace(strings.ToLower(domain))

	exists, err := s.store.CheckServerExists(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("server not found: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("server not found")
	}

	existing, err := s.store.GetDomainByDomain(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("check existing domain: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("domain %s is already registered", domain)
	}

	wildcard := isWildcardDomain(domain)
	normalizedDomain := normalizeDomain(domain)
	token := uuid.NewString()

	now := time.Now().UTC()
	row := DomainRow{
		ID:                uuid.NewString(),
		ServerID:          serverID,
		Domain:            normalizedDomain,
		Wildcard:          wildcard,
		Verified:          false,
		VerificationToken: &token,
		CreatedAt:         now,
	}

	if err := s.store.CreateDomain(ctx, row); err != nil {
		return nil, fmt.Errorf("create domain: %w", err)
	}

	record := domainRowToRecord(row)

	go func() {
		verifyCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.verifyOwnership(verifyCtx, &record)
	}()

	return &record, nil
}

func (s *Service) RemoveDomain(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("domain id is required")
	}

	domain, err := s.store.GetDomain(ctx, id)
	if err != nil {
		return fmt.Errorf("get domain: %w", err)
	}
	if domain == nil {
		return fmt.Errorf("domain not found")
	}

	if err := s.store.DeleteDomain(ctx, id); err != nil {
		return fmt.Errorf("delete domain: %w", err)
	}

	if s.caddy != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s.syncCaddyRoutes(ctx)
		}()
	}

	return nil
}

func (s *Service) ListDomains(ctx context.Context, serverID string) ([]DomainRecord, error) {
	if serverID == "" {
		return nil, fmt.Errorf("serverId is required")
	}

	rows, err := s.store.ListDomainsByServer(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}

	records := make([]DomainRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, domainRowToRecord(row))
	}
	return records, nil
}

func (s *Service) GetDomain(ctx context.Context, id string) (*DomainRecord, error) {
	row, err := s.store.GetDomain(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get domain: %w", err)
	}
	if row == nil {
		return nil, fmt.Errorf("domain not found")
	}
	record := domainRowToRecord(*row)
	return &record, nil
}

func (s *Service) FindDomainByHost(ctx context.Context, host string) (*DomainRecord, error) {
	host = normalizeDomain(host)
	allRows, err := s.store.ListAllDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}

	for _, row := range allRows {
		if row.Domain == host {
			record := domainRowToRecord(row)
			return &record, nil
		}
		if row.Wildcard {
			suffix := strings.TrimPrefix(row.Domain, "*")
			if strings.HasSuffix(host, suffix) {
				record := domainRowToRecord(row)
				return &record, nil
			}
		}
	}
	return nil, nil
}

func (s *Service) VerifyOwnership(ctx context.Context, id string) (*VerificationResult, error) {
	row, err := s.store.GetDomain(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get domain: %w", err)
	}
	if row == nil {
		return nil, fmt.Errorf("domain not found")
	}

	record := domainRowToRecord(*row)
	result := s.verifyOwnership(ctx, &record)
	return &result, nil
}

func (s *Service) CheckDNS(ctx context.Context, domain string, expectedIP string) (*DNSResult, error) {
	result := &DNSResult{
		Domain:     domain,
		ExpectedIP: expectedIP,
	}

	resolvedIPs := s.resolveDomain(domain)
	result.IPs = resolvedIPs
	result.Resolved = len(resolvedIPs) > 0

	if expectedIP != "" {
		for _, ip := range resolvedIPs {
			if ip == expectedIP {
				result.Match = true
				break
			}
		}
	}

	return result, nil
}

func (s *Service) verifyOwnership(ctx context.Context, record *DomainRecord) VerificationResult {
	result := VerificationResult{
		Domain:     record.Domain,
		ExpectedIP: s.panelIP,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if record.VerificationToken == nil || *record.VerificationToken == "" {
		result.Error = "no verification token"
		return result
	}

	token := *record.VerificationToken

	resolvedIPs := s.resolveDomain(record.Domain)
	result.ResolvedIPs = resolvedIPs
	result.DNSResolved = len(resolvedIPs) > 0

	if !result.DNSResolved {
		result.Error = "DNS not resolved"
		return result
	}

	checkURL := fmt.Sprintf("http://%s/.well-known/forge-verify", record.Domain)

	req, err := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("create request: %v", err)
		return result
	}
	req.Header.Set("Host", record.Domain)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("http request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("verification endpoint returned %d", resp.StatusCode)
		return result
	}

	var body [256]byte
	n, _ := resp.Body.Read(body[:])
	content := strings.TrimSpace(string(body[:n]))

	if content == token {
		result.Verified = true
		now := time.Now().UTC()
		if err := s.store.UpdateDomainVerification(ctx, record.ID, true, &now); err != nil {
			result.Error = fmt.Sprintf("update verification status: %v", err)
			return result
		}
		record.Verified = true
		record.VerifiedAt = &now

		if s.caddy != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				s.syncCaddyRoutes(ctx)
			}()
		}
	} else {
		result.Error = "verification token mismatch"
	}

	return result
}

func (s *Service) reverifyAll(ctx context.Context) {
	rows, err := s.store.ListAllDomains(ctx)
	if err != nil {
		return
	}

	for _, row := range rows {
		record := domainRowToRecord(row)
		s.verifyOwnership(ctx, &record)
	}
}

func (s *Service) resolveDomain(domain string) []string {
	if isWildcardDomain(domain) {
		domain = "test." + strings.TrimPrefix(domain, "*.")
	}
	ips, err := net.LookupHost(domain)
	if err != nil {
		return nil
	}
	return ips
}

func (s *Service) syncCaddyRoutes(ctx context.Context) error {
	rows, err := s.store.ListVerifiedDomains(ctx)
	if err != nil {
		return err
	}

	routes := make([]VerifiedDomainRoute, 0, len(rows))
	for _, row := range rows {
		dr := VerifiedDomainRoute{
			Domain:     row.Domain,
			Wildcard:   row.Wildcard,
			ServerID:   row.ServerID,
			TargetHost: "localhost",
			TargetPort: 8080,
		}
		if s.nodeResolver != nil && row.ServerID != "" {
			host, port, err := s.nodeResolver.ResolveServerTarget(ctx, row.ServerID)
			if err == nil {
				dr.TargetHost = host
				dr.TargetPort = port
			}
		}
		routes = append(routes, dr)
	}

	if s.caddy == nil {
		return nil
	}

	return s.caddy.UpdateDomainRoutes(ctx, routes)
}

func domainRowToRecord(row DomainRow) DomainRecord {
	return DomainRecord{
		ID:                row.ID,
		ServerID:          row.ServerID,
		Domain:            row.Domain,
		Wildcard:          row.Wildcard,
		Verified:          row.Verified,
		VerifiedAt:        row.VerifiedAt,
		VerificationToken: row.VerificationToken,
		CreatedAt:         row.CreatedAt,
	}
}

func isWildcardDomain(domain string) bool {
	return strings.HasPrefix(domain, "*.")
}

func normalizeDomain(domain string) string {
	if domain == "" {
		return domain
	}
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(domain)), ".")
}
