package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"

	"gamepanel/forge/internal/store"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/azure"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/dnspod"
	"github.com/go-acme/lego/v4/providers/dns/duckdns"
	"github.com/go-acme/lego/v4/providers/dns/dynu"
	"github.com/go-acme/lego/v4/providers/dns/easydns"
	"github.com/go-acme/lego/v4/providers/dns/exoscale"
	"github.com/go-acme/lego/v4/providers/dns/gandi"
	"github.com/go-acme/lego/v4/providers/dns/gcloud"
	"github.com/go-acme/lego/v4/providers/dns/glesys"
	"github.com/go-acme/lego/v4/providers/dns/godaddy"
	"github.com/go-acme/lego/v4/providers/dns/hetzner"
	"github.com/go-acme/lego/v4/providers/dns/infomaniak"
	"github.com/go-acme/lego/v4/providers/dns/ionos"
	"github.com/go-acme/lego/v4/providers/dns/lightsail"
	"github.com/go-acme/lego/v4/providers/dns/linode"
	"github.com/go-acme/lego/v4/providers/dns/namecheap"
	"github.com/go-acme/lego/v4/providers/dns/netcup"
	"github.com/go-acme/lego/v4/providers/dns/netlify"
	"github.com/go-acme/lego/v4/providers/dns/ns1"
	"github.com/go-acme/lego/v4/providers/dns/oraclecloud"
	"github.com/go-acme/lego/v4/providers/dns/ovh"
	"github.com/go-acme/lego/v4/providers/dns/pdns"
	"github.com/go-acme/lego/v4/providers/dns/porkbun"
	"github.com/go-acme/lego/v4/providers/dns/rfc2136"
	"github.com/go-acme/lego/v4/providers/dns/route53"
	"github.com/go-acme/lego/v4/providers/dns/scaleway"
	"github.com/go-acme/lego/v4/providers/dns/selectel"
	"github.com/go-acme/lego/v4/providers/dns/transip"
	"github.com/go-acme/lego/v4/providers/dns/vercel"
	"github.com/go-acme/lego/v4/providers/dns/vultr"
	"github.com/go-acme/lego/v4/providers/dns/wedos"
	"github.com/go-acme/lego/v4/providers/dns/zoneee"
)

type ProviderDefinition struct {
	Type             string            `json:"type"`
	Name             string            `json:"name"`
	Description      string            `json:"description"`
	CredentialFields []CredentialField `json:"credentialFields"`
}

type CredentialField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description,omitempty"`
}

type Service struct {
	store  *store.Store
	logger *log.Logger
}

func New(st *store.Store) (*Service, error) {
	if st == nil {
		return nil, errors.New("store required")
	}
	return &Service{store: st}, nil
}

func (s *Service) SetLogger(logger *log.Logger) {
	s.logger = logger
}

func (s *Service) log(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf("[dns] "+format, args...)
	}
}

func supportedProviders() []ProviderDefinition {
	providers := []ProviderDefinition{
		{Type: "cloudflare", Name: "Cloudflare", Description: "Cloudflare DNS",
			CredentialFields: []CredentialField{
				{Key: "CF_DNS_API_TOKEN", Label: "API Token", Type: "password", Required: true, Description: "Cloudflare API Token with DNS:Edit permission"},
			},
		},
		{Type: "route53", Name: "Amazon Route 53", Description: "AWS Route 53 DNS",
			CredentialFields: []CredentialField{
				{Key: "AWS_ACCESS_KEY_ID", Label: "Access Key ID", Type: "text", Required: true},
				{Key: "AWS_SECRET_ACCESS_KEY", Label: "Secret Access Key", Type: "password", Required: true},
				{Key: "AWS_REGION", Label: "Region", Type: "text", Required: false},
				{Key: "AWS_HOSTED_ZONE_ID", Label: "Hosted Zone ID", Type: "text", Required: false},
			},
		},
		{Type: "gcloud", Name: "Google Cloud DNS", Description: "Google Cloud DNS",
			CredentialFields: []CredentialField{
				{Key: "GCE_PROJECT", Label: "Project ID", Type: "text", Required: true},
				{Key: "GCE_SERVICE_ACCOUNT_FILE", Label: "Service Account JSON Key Path", Type: "text", Required: false},
				{Key: "GCE_SERVICE_ACCOUNT", Label: "Service Account Email", Type: "text", Required: false},
			},
		},
		{Type: "digitalocean", Name: "DigitalOcean", Description: "DigitalOcean DNS",
			CredentialFields: []CredentialField{
				{Key: "DO_AUTH_TOKEN", Label: "API Token", Type: "password", Required: true},
			},
		},
		{Type: "linode", Name: "Linode", Description: "Linode DNS",
			CredentialFields: []CredentialField{
				{Key: "LINODE_TOKEN", Label: "API Token", Type: "password", Required: true},
			},
		},
		{Type: "vultr", Name: "Vultr", Description: "Vultr DNS",
			CredentialFields: []CredentialField{
				{Key: "VULTR_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
		{Type: "ovh", Name: "OVH", Description: "OVH DNS",
			CredentialFields: []CredentialField{
				{Key: "OVH_ENDPOINT", Label: "Endpoint", Type: "text", Required: true, Description: "e.g. ovh-eu"},
				{Key: "OVH_APPLICATION_KEY", Label: "Application Key", Type: "text", Required: true},
				{Key: "OVH_APPLICATION_SECRET", Label: "Application Secret", Type: "password", Required: true},
				{Key: "OVH_CONSUMER_KEY", Label: "Consumer Key", Type: "text", Required: true},
			},
		},
		{Type: "namecheap", Name: "Namecheap", Description: "Namecheap DNS",
			CredentialFields: []CredentialField{
				{Key: "NAMECHEAP_API_USER", Label: "API User", Type: "text", Required: true},
				{Key: "NAMECHEAP_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
		{Type: "porkbun", Name: "Porkbun", Description: "Porkbun DNS",
			CredentialFields: []CredentialField{
				{Key: "PORKBUN_API_KEY", Label: "API Key", Type: "text", Required: true},
				{Key: "PORKBUN_SECRET_API_KEY", Label: "Secret API Key", Type: "password", Required: true},
			},
		},
		{Type: "godaddy", Name: "GoDaddy", Description: "GoDaddy DNS",
			CredentialFields: []CredentialField{
				{Key: "GODADDY_API_KEY", Label: "API Key", Type: "text", Required: true},
				{Key: "GODADDY_API_SECRET", Label: "API Secret", Type: "password", Required: true},
			},
		},
		{Type: "azure", Name: "Azure DNS", Description: "Microsoft Azure DNS",
			CredentialFields: []CredentialField{
				{Key: "AZURE_CLIENT_ID", Label: "Client ID", Type: "text", Required: true},
				{Key: "AZURE_CLIENT_SECRET", Label: "Client Secret", Type: "password", Required: true},
				{Key: "AZURE_TENANT_ID", Label: "Tenant ID", Type: "text", Required: true},
				{Key: "AZURE_SUBSCRIPTION_ID", Label: "Subscription ID", Type: "text", Required: true},
				{Key: "AZURE_RESOURCE_GROUP", Label: "Resource Group", Type: "text", Required: true},
			},
		},
		{Type: "alidns", Name: "Alibaba Cloud DNS", Description: "Alibaba Cloud (Aliyun) DNS",
			CredentialFields: []CredentialField{
				{Key: "ALICLOUD_ACCESS_KEY", Label: "Access Key ID", Type: "text", Required: true},
				{Key: "ALICLOUD_SECRET_KEY", Label: "Secret Access Key", Type: "password", Required: true},
			},
		},
		{Type: "dnspod", Name: "DNSPod (Tencent)", Description: "Tencent DNSPod",
			CredentialFields: []CredentialField{
				{Key: "DNSPOD_API_KEY", Label: "API Key", Type: "text", Required: true},
			},
		},
		{Type: "duckdns", Name: "Duck DNS", Description: "Duck DNS",
			CredentialFields: []CredentialField{
				{Key: "DUCKDNS_TOKEN", Label: "Token", Type: "password", Required: true},
			},
		},
		{Type: "dynu", Name: "Dynu", Description: "Dynu DNS",
			CredentialFields: []CredentialField{
				{Key: "DYNU_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
		{Type: "easydns", Name: "EasyDNS", Description: "EasyDNS",
			CredentialFields: []CredentialField{
				{Key: "EASYDNS_TOKEN", Label: "API Token", Type: "password", Required: true},
				{Key: "EASYDNS_API_KEY", Label: "API Key", Type: "text", Required: true},
			},
		},
		{Type: "exoscale", Name: "Exoscale", Description: "Exoscale DNS",
			CredentialFields: []CredentialField{
				{Key: "EXOSCALE_API_KEY", Label: "API Key", Type: "text", Required: true},
				{Key: "EXOSCALE_API_SECRET", Label: "API Secret", Type: "password", Required: true},
			},
		},
		{Type: "gandi", Name: "Gandi", Description: "Gandi LiveDNS",
			CredentialFields: []CredentialField{
				{Key: "GANDIV5_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
		{Type: "glesys", Name: "GleSYS", Description: "GleSYS DNS",
			CredentialFields: []CredentialField{
				{Key: "GLESYS_API_USER", Label: "API User", Type: "text", Required: true},
				{Key: "GLESYS_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
		{Type: "hetzner", Name: "Hetzner", Description: "Hetzner DNS",
			CredentialFields: []CredentialField{
				{Key: "HETZNER_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
		{Type: "infomaniak", Name: "Infomaniak", Description: "Infomaniak DNS",
			CredentialFields: []CredentialField{
				{Key: "INFOMANIAK_ACCESS_TOKEN", Label: "Access Token", Type: "password", Required: true},
			},
		},
		{Type: "ionos", Name: "Ionos", Description: "Ionos DNS",
			CredentialFields: []CredentialField{
				{Key: "IONOS_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
		{Type: "lightsail", Name: "Amazon Lightsail", Description: "AWS Lightsail DNS",
			CredentialFields: []CredentialField{
				{Key: "AWS_ACCESS_KEY_ID", Label: "Access Key ID", Type: "text", Required: true},
				{Key: "AWS_SECRET_ACCESS_KEY", Label: "Secret Access Key", Type: "password", Required: true},
				{Key: "AWS_REGION", Label: "Region", Type: "text", Required: false},
			},
		},
		{Type: "netcup", Name: "Netcup", Description: "Netcup DNS",
			CredentialFields: []CredentialField{
				{Key: "NETCUP_CUSTOMER_NUMBER", Label: "Customer Number", Type: "text", Required: true},
				{Key: "NETCUP_API_KEY", Label: "API Key", Type: "password", Required: true},
				{Key: "NETCUP_API_PASSWORD", Label: "API Password", Type: "password", Required: true},
			},
		},
		{Type: "netlify", Name: "Netlify", Description: "Netlify DNS",
			CredentialFields: []CredentialField{
				{Key: "NETLIFY_TOKEN", Label: "Access Token", Type: "password", Required: true},
			},
		},
		{Type: "ns1", Name: "NS1", Description: "NS1 DNS",
			CredentialFields: []CredentialField{
				{Key: "NS1_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
		{Type: "oraclecloud", Name: "Oracle Cloud DNS", Description: "Oracle Cloud DNS",
			CredentialFields: []CredentialField{
				{Key: "OCI_PRIVKEY_FILE", Label: "Private Key Path", Type: "text", Required: true},
				{Key: "OCI_TENANCY_OCID", Label: "Tenancy OCID", Type: "text", Required: true},
				{Key: "OCI_USER_OCID", Label: "User OCID", Type: "text", Required: true},
				{Key: "OCI_REGION", Label: "Region", Type: "text", Required: true},
				{Key: "OCI_COMPARTMENT_OCID", Label: "Compartment OCID", Type: "text", Required: true},
			},
		},
		{Type: "pdns", Name: "PowerDNS", Description: "PowerDNS",
			CredentialFields: []CredentialField{
				{Key: "PDNS_API_URL", Label: "API URL", Type: "text", Required: true},
				{Key: "PDNS_API_KEY", Label: "API Key", Type: "password", Required: true},
				{Key: "PDNS_TTL", Label: "TTL", Type: "number", Required: false},
			},
		},
		{Type: "rfc2136", Name: "RFC2136 (Dynamic DNS)", Description: "Generic RFC2136 dynamic DNS update",
			CredentialFields: []CredentialField{
				{Key: "RFC2136_TSIG_KEY", Label: "TSIG Key Name", Type: "text", Required: false},
				{Key: "RFC2136_TSIG_SECRET", Label: "TSIG Secret", Type: "password", Required: false},
				{Key: "RFC2136_TSIG_ALGORITHM", Label: "TSIG Algorithm", Type: "text", Required: false, Description: "e.g. hmac-sha256"},
				{Key: "RFC2136_NAMESERVER", Label: "Nameserver", Type: "text", Required: true},
			},
		},
		{Type: "scaleway", Name: "Scaleway", Description: "Scaleway DNS",
			CredentialFields: []CredentialField{
				{Key: "SCALEWAY_API_TOKEN", Label: "API Token", Type: "password", Required: true},
			},
		},
		{Type: "selectel", Name: "Selectel", Description: "Selectel DNS",
			CredentialFields: []CredentialField{
				{Key: "SELECTEL_API_TOKEN", Label: "API Token", Type: "password", Required: true},
			},
		},
		{Type: "transip", Name: "TransIP", Description: "TransIP DNS",
			CredentialFields: []CredentialField{
				{Key: "TRANSIP_ACCOUNT_NAME", Label: "Account Name", Type: "text", Required: true},
				{Key: "TRANSIP_PRIVATE_KEY_PATH", Label: "Private Key Path", Type: "text", Required: true},
			},
		},
		{Type: "vercel", Name: "Vercel", Description: "Vercel DNS",
			CredentialFields: []CredentialField{
				{Key: "VERCEL_API_TOKEN", Label: "API Token", Type: "password", Required: true},
			},
		},
		{Type: "wedos", Name: "WEDOS", Description: "WEDOS DNS",
			CredentialFields: []CredentialField{
				{Key: "WEDOS_USERNAME", Label: "Username", Type: "text", Required: true},
				{Key: "WEDOS_WAPI_PASSWORD", Label: "WAPI Password", Type: "password", Required: true},
			},
		},
		{Type: "zoneee", Name: "Zone.ee", Description: "Zone.ee DNS",
			CredentialFields: []CredentialField{
				{Key: "ZONEEE_API_USER", Label: "API User", Type: "text", Required: true},
				{Key: "ZONEEE_API_KEY", Label: "API Key", Type: "password", Required: true},
			},
		},
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})
	return providers
}

type providerFactory func(credentials map[string]string) (challenge.Provider, error)

var providerRegistry = map[string]providerFactory{
	"alidns": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return alidns.NewDNSProvider()
		})
	},
	"azure": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return azure.NewDNSProvider()
		})
	},
	"cloudflare": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return cloudflare.NewDNSProvider()
		})
	},
	"digitalocean": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return digitalocean.NewDNSProvider()
		})
	},
	"dnspod": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return dnspod.NewDNSProvider()
		})
	},
	"duckdns": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return duckdns.NewDNSProvider()
		})
	},
	"dynu": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return dynu.NewDNSProvider()
		})
	},
	"easydns": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return easydns.NewDNSProvider()
		})
	},
	"exoscale": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return exoscale.NewDNSProvider()
		})
	},
	"gandi": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return gandi.NewDNSProvider()
		})
	},
	"gcloud": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return gcloud.NewDNSProvider()
		})
	},
	"glesys": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return glesys.NewDNSProvider()
		})
	},
	"godaddy": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return godaddy.NewDNSProvider()
		})
	},
	"hetzner": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return hetzner.NewDNSProvider()
		})
	},
	"infomaniak": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return infomaniak.NewDNSProvider()
		})
	},
	"ionos": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return ionos.NewDNSProvider()
		})
	},
	"lightsail": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return lightsail.NewDNSProvider()
		})
	},
	"linode": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return linode.NewDNSProvider()
		})
	},
	"namecheap": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return namecheap.NewDNSProvider()
		})
	},
	"netcup": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return netcup.NewDNSProvider()
		})
	},
	"netlify": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return netlify.NewDNSProvider()
		})
	},
	"ns1": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return ns1.NewDNSProvider()
		})
	},
	"oraclecloud": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return oraclecloud.NewDNSProvider()
		})
	},
	"ovh": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return ovh.NewDNSProvider()
		})
	},
	"pdns": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return pdns.NewDNSProvider()
		})
	},
	"porkbun": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return porkbun.NewDNSProvider()
		})
	},
	"rfc2136": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return rfc2136.NewDNSProvider()
		})
	},
	"route53": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return route53.NewDNSProvider()
		})
	},
	"scaleway": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return scaleway.NewDNSProvider()
		})
	},
	"selectel": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return selectel.NewDNSProvider()
		})
	},
	"transip": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return transip.NewDNSProvider()
		})
	},
	"vercel": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return vercel.NewDNSProvider()
		})
	},
	"vultr": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return vultr.NewDNSProvider()
		})
	},
	"wedos": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return wedos.NewDNSProvider()
		})
	},
	"zoneee": func(credentials map[string]string) (challenge.Provider, error) {
		return createDNSProvider(credentials, func() (challenge.Provider, error) {
			return zoneee.NewDNSProvider()
		})
	},
}

func createDNSProvider(credentials map[string]string, newProvider func() (challenge.Provider, error)) (challenge.Provider, error) {
	restore := setEnvRestore(credentials)
	p, err := newProvider()
	if err != nil {
		restore()
		return nil, err
	}
	return &restoringProvider{provider: p, restore: restore}, nil
}

type restoringProvider struct {
	provider challenge.Provider
	restore  func()
}

func (r *restoringProvider) Present(domain, token, keyAuth string) error {
	return r.provider.Present(domain, token, keyAuth)
}

func (r *restoringProvider) CleanUp(domain, token, keyAuth string) error {
	defer r.restore()
	return r.provider.CleanUp(domain, token, keyAuth)
}

func (s *Service) ListSupportedProviders() []ProviderDefinition {
	return supportedProviders()
}

func (s *Service) GetProviderDefinition(providerType string) (ProviderDefinition, error) {
	for _, p := range supportedProviders() {
		if p.Type == providerType {
			return p, nil
		}
	}
	return ProviderDefinition{}, fmt.Errorf("unsupported provider type: %s", providerType)
}

func (s *Service) ConfigureProvider(ctx context.Context, name, providerType string, credentials json.RawMessage) (store.DNSProvider, error) {
	if _, err := s.GetProviderDefinition(providerType); err != nil {
		return store.DNSProvider{}, err
	}
	if _, ok := providerRegistry[providerType]; !ok {
		return store.DNSProvider{}, fmt.Errorf("provider type %s has no implementation", providerType)
	}
	return s.store.UpsertDNSProvider(ctx, store.UpsertDNSProviderRequest{
		Name:         name,
		ProviderType: providerType,
		Credentials:  credentials,
	})
}

func (s *Service) VerifyProvider(ctx context.Context, providerID string) error {
	provider, err := s.store.GetDNSProvider(ctx, providerID)
	if err != nil {
		return fmt.Errorf("get dns provider: %w", err)
	}

	factory, ok := providerRegistry[provider.ProviderType]
	if !ok {
		return fmt.Errorf("no implementation for provider type %s", provider.ProviderType)
	}

	if _, err := factory(credentialsFromProvider(provider)); err != nil {
		return fmt.Errorf("initialize provider: %w", err)
	}

	if err := s.store.MarkDNSProviderVerified(ctx, providerID, true); err != nil {
		return fmt.Errorf("mark verified: %w", err)
	}
	return nil
}

func (s *Service) SetDefaultProvider(ctx context.Context, providerID string) error {
	if _, err := s.store.GetDNSProvider(ctx, providerID); err != nil {
		return fmt.Errorf("provider not found: %w", err)
	}
	return s.store.SetDefaultDNSProvider(ctx, providerID)
}

func (s *Service) ExecuteDNSChallenge(ctx context.Context, domain, txtValue string) error {
	stored, err := s.store.GetDefaultDNSProvider(ctx)
	if err != nil {
		return fmt.Errorf("no default dns provider configured: %w", err)
	}

	factory, ok := providerRegistry[stored.ProviderType]
	if !ok {
		return fmt.Errorf("no implementation for provider type %s", stored.ProviderType)
	}

	cp, err := factory(credentialsFromProvider(stored))
	if err != nil {
		return fmt.Errorf("create challenge provider: %w", err)
	}

	return cp.Present(domain, "", txtValue)
}

func (s *Service) CleanupDNSChallenge(ctx context.Context, domain, txtValue string) error {
	stored, err := s.store.GetDefaultDNSProvider(ctx)
	if err != nil {
		return fmt.Errorf("no default dns provider configured: %w", err)
	}

	factory, ok := providerRegistry[stored.ProviderType]
	if !ok {
		return fmt.Errorf("no implementation for provider type %s", stored.ProviderType)
	}

	cp, err := factory(credentialsFromProvider(stored))
	if err != nil {
		return fmt.Errorf("create challenge provider: %w", err)
	}

	return cp.CleanUp(domain, "", txtValue)
}

func (s *Service) RegisterWithAcme(register func(name string, factory func(providerName string, credentials map[string]string) (challenge.Provider, error))) {
	// Register all providers from the registry
	for providerType, provFactory := range providerRegistry {
		fn := provFactory
		register(providerType, func(providerName string, credentials map[string]string) (challenge.Provider, error) {
			return s.acmeFactory(providerType, credentials, fn)
		})
	}
}

func (s *Service) acmeFactory(providerType string, inlineCredentials map[string]string, factory providerFactory) (challenge.Provider, error) {
	if len(inlineCredentials) > 0 {
		return factory(inlineCredentials)
	}

	ctx := context.Background()
	stored, err := s.store.GetDefaultDNSProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("no credentials provided and no default %s provider: %w", providerType, err)
	}
	if stored.ProviderType != providerType {
		return nil, fmt.Errorf("default dns provider is %s, requested %s", stored.ProviderType, providerType)
	}

	return factory(credentialsFromProvider(stored))
}

func credentialsFromProvider(provider store.DNSProvider) map[string]string {
	creds := make(map[string]string)
	if len(provider.Credentials) == 0 {
		return creds
	}
	var m map[string]string
	if err := json.Unmarshal(provider.Credentials, &m); err != nil {
		return creds
	}
	for k, v := range m {
		if v != "" {
			creds[k] = v
		}
	}
	return creds
}

var envMu sync.Mutex

func setEnvRestore(env map[string]string) func() {
	envMu.Lock()
	previous := make(map[string]*string, len(env))
	for k, v := range env {
		if existing, ok := os.LookupEnv(k); ok {
			val := existing
			previous[k] = &val
		} else {
			previous[k] = nil
		}
		os.Setenv(k, v)
	}
	return func() {
		defer envMu.Unlock()
		for k, prev := range previous {
			if prev != nil {
				os.Setenv(k, *prev)
			} else {
				os.Unsetenv(k)
			}
		}
	}
}

var _ = errors.New
