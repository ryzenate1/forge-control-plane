package config

type SchedulerBackend struct {
	Type      string
	Addr      string
	Kubeconfig string
	Namespace string
	Region    string
	Datacenter string
}

type Runtime struct {
	Schedulers map[string]SchedulerBackend
}

func RuntimeConfig() Runtime {
	return Runtime{
		Schedulers: map[string]SchedulerBackend{
			"docker": {
				Type: "docker",
			},
			"k3s": {
				Type:       "k3s",
				Addr:       env("K3S_API_URL", ""),
				Kubeconfig: env("K3S_KUBECONFIG", ""),
				Namespace:  env("K3S_NAMESPACE", "default"),
			},
			"nomad": {
				Type:       "nomad",
				Addr:       env("NOMAD_ADDR", "http://127.0.0.1:4646"),
				Region:     env("NOMAD_REGION", "global"),
				Datacenter: env("NOMAD_DATACENTER", "dc1"),
				Namespace:  env("NOMAD_NAMESPACE", "default"),
			},
		},
	}
}
