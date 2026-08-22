package appstore

import (
	"context"
	"testing"
)

func TestInstallRequiresExplicitTargetAndName(t *testing.T) {
	// Validation must run before any database write or runtime operation.
	svc := &Service{}
	for _, req := range []InstallRequest{
		{AppKey: "redis", Name: "cache"},
		{AppKey: "redis", Name: "cache", NodeID: "   "},
		{AppKey: "redis", NodeID: "node-1"},
	} {
		if _, err := svc.InstallApp(context.Background(), &req); err == nil {
			t.Fatal("expected validation error")
		}
	}
}
