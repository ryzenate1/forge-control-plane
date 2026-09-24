package runtime

import (
	"errors"
	"os"
	"strings"
)

// ErrUnsupportedProvider means Forge has no runtime for the named engine at
// all. It is distinct from "not available on this node": an unsupported name
// would otherwise be answered by silently building a container on whichever
// engine the daemon happens to run, and reporting that as the requested one.
var ErrUnsupportedProvider = errors.New("unsupported provider")

// experimentalProviders are engines whose adapter exists but is not mature
// enough to serve by default. They are accepted only while
// ENABLE_EXPERIMENTAL_RUNTIMES is set, so an operator can exercise them
// explicitly instead of the panel falling back to Docker behind their back.
var experimentalProviders = map[string]bool{
	"lxc": true,
	"kvm": true,
}

// supportedProviders are engines this Beacon build can actually run.
var supportedProviders = map[string]bool{
	ProviderDocker:      true,
	ProviderContainerd:  true,
	ProviderPodman:      true,
	ProviderFirecracker: true,
	ProviderKubernetes:  true,
}

// ExperimentalRuntimesEnabled reports whether the experimental engines are
// opted in. Read per call so a process-level toggle does not need a restart in
// development, and so tests can set it around a single request.
func ExperimentalRuntimesEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ENABLE_EXPERIMENTAL_RUNTIMES")))
	return value == "true" || value == "1" || value == "yes"
}

// IsSupportedProvider reports whether the named engine can be served by this
// build, honouring the experimental opt-in. An empty name means "whatever this
// node runs" and is always acceptable.
func IsSupportedProvider(provider string) bool {
	name := strings.ToLower(strings.TrimSpace(provider))
	if name == "" {
		return true
	}
	if supportedProviders[name] {
		return true
	}
	return experimentalProviders[name] && ExperimentalRuntimesEnabled()
}

// ValidateProvider rejects a provider Forge cannot run rather than quietly
// serving Docker for it. The error names the provider so the caller can see
// which part of the request was refused.
func ValidateProvider(provider string) error {
	name := strings.ToLower(strings.TrimSpace(provider))
	if IsSupportedProvider(name) {
		return nil
	}
	if experimentalProviders[name] {
		// The wording still names the refusal the caller must handle
		// (unsupported provider) and adds the way out, rather than replacing one
		// message with another that clients cannot match on.
		return &ProviderError{Provider: name, Err: ErrUnsupportedProvider, Message: "unsupported provider " + name + " (experimental; set ENABLE_EXPERIMENTAL_RUNTIMES to serve it)"}
	}
	return &ProviderError{Provider: name, Err: ErrUnsupportedProvider}
}

// ProviderError carries the rejected name alongside the sentinel so callers can
// match on errors.Is(err, ErrUnsupportedProvider) and still report which
// provider was refused.
type ProviderError struct {
	Provider string
	Message  string
	Err      error
}

func (e *ProviderError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "unsupported provider " + strings.TrimSpace(e.Provider)
}

func (e *ProviderError) Unwrap() error { return e.Err }
