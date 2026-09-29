package trafficmanager

import (
	"context"
	"errors"
	"testing"
)

type fakeNodeResolver struct {
	host string
	err  error
}

func (f *fakeNodeResolver) ResolveTargetHost(_ context.Context, _, _ string) (string, error) {
	return f.host, f.err
}

func TestNodeResolverReturnsError(t *testing.T) {
	// The contract is (string,error): unresolved targets must surface an
	// error, never a bare string callers could mistake for a host.
	var r NodeResolver = &fakeNodeResolver{host: "", err: errors.New("no target")}
	if _, err := r.ResolveTargetHost(context.Background(), "", ""); err == nil {
		t.Fatal("expected error for unresolved target")
	}

	var ok NodeResolver = &fakeNodeResolver{host: "node.internal"}
	host, err := ok.ResolveTargetHost(context.Background(), "srv", "node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "node.internal" {
		t.Fatalf("unexpected host %q", host)
	}
}

func TestCaddyProxyAcceptsNodeResolver(t *testing.T) {
	p := NewCaddyReverseProxy("127.0.0.1:2019")
	// Must compile against the (string,error) shape.
	p.SetNodeResolver(&fakeNodeResolver{host: "h"})
}
