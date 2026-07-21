package scheduler

import "fmt"

func NewScheduler(cfg SchedulerConfig) (Scheduler, error) {
	switch cfg.Type {
	case SchedulerTypeDocker:
		return nil, fmt.Errorf("docker scheduler uses the existing runtime adapter; use a dedicated K3s or Nomad config")
	case SchedulerTypeK3s:
		if cfg.K3s == nil {
			return nil, fmt.Errorf("k3s config is required")
		}
		return NewK3sScheduler(*cfg.K3s), nil
	case SchedulerTypeNomad:
		if cfg.Nomad == nil {
			return nil, fmt.Errorf("nomad config is required")
		}
		return NewNomadScheduler(*cfg.Nomad), nil
	default:
		return nil, fmt.Errorf("unknown scheduler type: %s", cfg.Type)
	}
}

func NodeScheduler(schedulerType string, nodeConfig map[string]any) (Scheduler, error) {
	switch SchedulerType(schedulerType) {
	case SchedulerTypeK3s:
		cfg := K3sConfig{}
		if v, ok := nodeConfig["kubeconfigPath"].(string); ok {
			cfg.KubeconfigPath = v
		}
		if v, ok := nodeConfig["namespace"].(string); ok {
			cfg.Namespace = v
		}
		if v, ok := nodeConfig["kubeApi"].(string); ok {
			cfg.KubeAPI = v
		}
		return NewK3sScheduler(cfg), nil
	case SchedulerTypeNomad:
		cfg := NomadConfig{}
		if v, ok := nodeConfig["addr"].(string); ok {
			cfg.Addr = v
		}
		if v, ok := nodeConfig["region"].(string); ok {
			cfg.Region = v
		}
		if v, ok := nodeConfig["datacenter"].(string); ok {
			cfg.Datacenter = v
		}
		if v, ok := nodeConfig["namespace"].(string); ok {
			cfg.Namespace = v
		}
		return NewNomadScheduler(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported node scheduler type: %s", schedulerType)
	}
}
