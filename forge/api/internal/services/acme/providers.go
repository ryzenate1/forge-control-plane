package acme

import (
	"fmt"
	"sync"

	"github.com/go-acme/lego/v4/challenge"
)

var (
	providersMu sync.RWMutex
	providers   = make(map[string]DNSProviderFactory)
)

func RegisterDNSProvider(name string, factory DNSProviderFactory) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[name] = factory
}

func GetDNSProvider(name string, credentials map[string]string) (challenge.Provider, error) {
	providersMu.RLock()
	factory, ok := providers[name]
	providersMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("dns provider %q not registered", name)
	}
	return factory(name, credentials)
}
