package http

import (
	"context"
	"errors"
	"testing"
)

func TestVerifyMTLSRevocation(t *testing.T) {
	lookupErr := errors.New("database unavailable")
	tests := []struct {
		name   string
		lookup func(context.Context, string) (bool, error)
		want   error
	}{
		{
			name: "active certificate",
			lookup: func(context.Context, string) (bool, error) {
				return false, nil
			},
		},
		{
			name: "revoked certificate",
			lookup: func(context.Context, string) (bool, error) {
				return true, nil
			},
			want: errMTLSCertificateRevoked,
		},
		{
			name: "lookup failure",
			lookup: func(context.Context, string) (bool, error) {
				return false, lookupErr
			},
			want: lookupErr,
		},
		{
			name: "missing lookup",
			want: errors.New("mTLS revocation lookup is unavailable"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := verifyMTLSRevocation(context.Background(), "1234", test.lookup)
			if test.want == nil && err != nil {
				t.Fatalf("verifyMTLSRevocation() error = %v", err)
			}
			if test.want != nil && !errors.Is(err, test.want) && err.Error() != test.want.Error() {
				t.Fatalf("verifyMTLSRevocation() error = %v, want %v", err, test.want)
			}
		})
	}
}
