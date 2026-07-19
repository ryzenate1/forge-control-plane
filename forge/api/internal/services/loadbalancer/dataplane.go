package loadbalancer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

const udpSessionTTL = 2 * time.Minute

// Start reconciles persisted target groups into live L4 listeners and health
// checks. It is inert unless LOAD_BALANCER_ENABLED=true.
func (s *Service) Start(parent context.Context) {
	if s == nil || !s.enabled {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			s.reconcileListeners(ctx)
			s.runHealthChecks(ctx)
			select {
			case <-ctx.Done():
				s.closeAll()
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) Shutdown() {
	if s != nil {
		s.mu.Lock()
		cancel := s.cancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
	}
}

func (s *Service) reconcileListeners(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, group := range s.groups {
		if group.Port < s.portMin || group.Port > s.portMax {
			continue
		}
		address := net.JoinHostPort(s.bindHost, fmt.Sprintf("%d", group.Port))
		if group.Protocol == "udp" {
			if s.packets[id] != nil {
				continue
			}
			pc, err := net.ListenPacket("udp", address)
			if err != nil {
				continue
			}
			s.packets[id] = pc
			go s.serveUDP(ctx, id, pc)
		} else {
			if s.listeners[id] != nil {
				continue
			}
			ln, err := net.Listen("tcp", address)
			if err != nil {
				continue
			}
			s.listeners[id] = ln
			go s.serveTCP(ctx, id, ln)
		}
	}
}

func (s *Service) closeListenerLocked(id string) {
	if ln := s.listeners[id]; ln != nil {
		_ = ln.Close()
		delete(s.listeners, id)
	}
	if pc := s.packets[id]; pc != nil {
		_ = pc.Close()
		delete(s.packets, id)
	}
}
func (s *Service) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.listeners {
		s.closeListenerLocked(id)
	}
	for id := range s.packets {
		s.closeListenerLocked(id)
	}
}

func (s *Service) serveTCP(ctx context.Context, groupID string, ln net.Listener) {
	for {
		client, err := ln.Accept()
		if err != nil {
			return
		}
		go s.proxyTCP(ctx, groupID, client)
	}
}

func (s *Service) proxyTCP(ctx context.Context, groupID string, client net.Conn) {
	defer client.Close()
	host, _, _ := net.SplitHostPort(client.RemoteAddr().String())
	target, err := s.NextTarget(ctx, groupID, host)
	if err != nil {
		return
	}
	backend, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(target.IP, fmt.Sprintf("%d", target.Port)))
	if err != nil {
		s.setTargetStatus(groupID, target.ID, TargetStatusUnhealthy)
		return
	}
	defer backend.Close()
	defer s.releaseConnection(target.ID)
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(backend, client)
		if c, ok := backend.(*net.TCPConn); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, backend)
		if c, ok := client.(*net.TCPConn); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}()
	select {
	case <-ctx.Done():
	case <-done:
	}
}

type udpSession struct {
	backend *net.UDPConn
	client  net.Addr
	last    time.Time
	mu      sync.Mutex
}

func (s *Service) serveUDP(ctx context.Context, groupID string, listener net.PacketConn) {
	sessions := map[string]*udpSession{}
	var sessionsMu sync.Mutex
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sessionsMu.Lock()
				for key, session := range sessions {
					session.mu.Lock()
					expired := time.Since(session.last) > udpSessionTTL
					session.mu.Unlock()
					if expired {
						_ = session.backend.Close()
						delete(sessions, key)
					}
				}
				sessionsMu.Unlock()
			}
		}
	}()
	buffer := make([]byte, 65535)
	for {
		n, client, err := listener.ReadFrom(buffer)
		if err != nil {
			return
		}
		key := client.String()
		sessionsMu.Lock()
		session := sessions[key]
		if session == nil {
			host, _, _ := net.SplitHostPort(key)
			target, selectErr := s.NextTarget(ctx, groupID, host)
			if selectErr == nil {
				remote, resolveErr := net.ResolveUDPAddr("udp", net.JoinHostPort(target.IP, fmt.Sprintf("%d", target.Port)))
				if resolveErr == nil {
					backend, dialErr := net.DialUDP("udp", nil, remote)
					if dialErr == nil {
						session = &udpSession{backend: backend, client: client, last: time.Now()}
						sessions[key] = session
						go relayUDPResponses(listener, session)
					}
				}
			}
		}
		sessionsMu.Unlock()
		if session != nil {
			session.mu.Lock()
			session.last = time.Now()
			_, _ = session.backend.Write(buffer[:n])
			session.mu.Unlock()
		}
	}
}

func relayUDPResponses(listener net.PacketConn, session *udpSession) {
	buffer := make([]byte, 65535)
	for {
		n, err := session.backend.Read(buffer)
		if err != nil {
			return
		}
		_, _ = listener.WriteTo(buffer[:n], session.client)
	}
}
func (s *Service) releaseConnection(targetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connCount[targetID] > 0 {
		s.connCount[targetID]--
	}
}
func (s *Service) setTargetStatus(groupID, targetID string, status TargetStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.groups[groupID]
	if group == nil {
		return
	}
	for i := range group.Targets {
		if group.Targets[i].ID == targetID {
			group.Targets[i].Status = status
			if s.store != nil {
				_ = s.store.UpdateTargetStatus(context.Background(), targetID, string(status))
			}
			return
		}
	}
}

func (s *Service) runHealthChecks(ctx context.Context) {
	s.mu.RLock()
	groups := make([]TargetGroup, 0, len(s.groups))
	for _, g := range s.groups {
		copyGroup := *g
		copyGroup.Targets = append([]Target(nil), g.Targets...)
		groups = append(groups, copyGroup)
	}
	s.mu.RUnlock()
	for _, group := range groups {
		if group.Protocol == "udp" {
			continue
		}
		for _, target := range group.Targets {
			if target.Status == TargetStatusDraining {
				continue
			}
			network := group.Protocol
			if network != "udp" {
				network = "tcp"
			}
			conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, network, net.JoinHostPort(target.IP, fmt.Sprintf("%d", target.Port)))
			if err == nil {
				_ = conn.Close()
				s.setTargetStatus(group.ID, target.ID, TargetStatusHealthy)
			} else if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "operation was canceled") {
				s.setTargetStatus(group.ID, target.ID, TargetStatusUnhealthy)
			}
		}
	}
}
