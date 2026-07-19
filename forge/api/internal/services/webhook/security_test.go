package webhook

import (
	"context"
	"net"
	"testing"
)

type staticResolver map[string][]net.IP

func (r staticResolver) LookupIP(_ context.Context, _, host string) ([]net.IP, error) {
	return r[host], nil
}

func TestValidateURLRejectsSSRFAddresses(t *testing.T) {
	r := staticResolver{"private.test": {net.ParseIP("10.0.0.1")}, "metadata.test": {net.ParseIP("169.254.169.254")}, "public.test": {net.ParseIP("93.184.216.34")}}
	for _, target := range []string{"file:///tmp/a", "http://127.0.0.1/hook", "http://private.test/hook", "http://metadata.test/latest"} {
		if err := validateURL(context.Background(), target, r); err == nil {
			t.Errorf("expected %s to be rejected", target)
		}
	}
	if err := validateURL(context.Background(), "https://public.test/hook", r); err != nil {
		t.Fatal(err)
	}
}
