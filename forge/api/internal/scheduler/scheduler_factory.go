package scheduler

import (
	"errors"
	"fmt"
)

// ErrDockerHandledByBeacon reports that a "docker" scheduler target is not
// served by this package. Docker workloads run on the node through Beacon's
// runtime adapters (beacon/internal/runtime) reached over the Panel→Beacon
// HTTP client (internal/daemon.Client); building a second docker path here
// would duplicate it. The sentinel exists so callers — nodes default to
// scheduler_type 'docker' — can branch on "handled elsewhere" instead of
// treating it as an opaque failure.
var ErrDockerHandledByBeacon = errors.New("docker workloads are executed by the Beacon runtime adapters over /api/remote, not by an internal/scheduler backend")

func NewScheduler(cfg SchedulerConfig) (Scheduler, error) {
	switch cfg.Type {
	case SchedulerTypeDocker:
		return nil, fmt.Errorf("%w", ErrDockerHandledByBeacon)
	case SchedulerTypeK3s:
		if cfg.K3s == nil {
			return nil, fmt.Errorf("k3s config is required")
		}
		return NewK3sScheduler(*cfg.K3s)
	case SchedulerTypeNomad:
		if cfg.Nomad == nil {
			return nil, fmt.Errorf("nomad config is required")
		}
		return NewNomadScheduler(*cfg.Nomad)
	default:
		return nil, fmt.Errorf("unknown scheduler type: %s", cfg.Type)
	}
}

// configString extracts one string field from a node's JSON
// scheduler_config. A value that is present but not a string (a number, a
// nested object) is a configuration error and is reported naming the field
// and the type found, instead of being skipped so that a scheduler with an
// empty kubeconfig/addr is built and misbehaves far from the cause. JSON
// null and absent keys are treated as unset; the constructors below then
// reject configs that are unusably incomplete.
func configString(nodeConfig map[string]any, field string) (string, error) {
	raw, ok := nodeConfig[field]
	if !ok || raw == nil {
		return "", nil
	}
	v, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("scheduler config field %q must be a string, got %T", field, raw)
	}
	return v, nil
}

func NodeScheduler(schedulerType string, nodeConfig map[string]any) (Scheduler, error) {
	switch SchedulerType(schedulerType) {
	case SchedulerTypeDocker:
		return nil, fmt.Errorf("%w", ErrDockerHandledByBeacon)
	case SchedulerTypeK3s:
		kubeconfigPath, err := configString(nodeConfig, "kubeconfigPath")
		if err != nil {
			return nil, err
		}
		namespace, err := configString(nodeConfig, "namespace")
		if err != nil {
			return nil, err
		}
		kubeAPI, err := configString(nodeConfig, "kubeApi")
		if err != nil {
			return nil, err
		}
		return NewK3sScheduler(K3sConfig{
			KubeconfigPath: kubeconfigPath,
			Namespace:      namespace,
			KubeAPI:        kubeAPI,
		})
	case SchedulerTypeNomad:
		addr, err := configString(nodeConfig, "addr")
		if err != nil {
			return nil, err
		}
		region, err := configString(nodeConfig, "region")
		if err != nil {
			return nil, err
		}
		datacenter, err := configString(nodeConfig, "datacenter")
		if err != nil {
			return nil, err
		}
		namespace, err := configString(nodeConfig, "namespace")
		if err != nil {
			return nil, err
		}
		return NewNomadScheduler(NomadConfig{
			Addr:       addr,
			Region:     region,
			Datacenter: datacenter,
			Namespace:  namespace,
		})
	default:
		return nil, fmt.Errorf("unsupported node scheduler type: %s", schedulerType)
	}
}
