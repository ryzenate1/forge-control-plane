// Required dependencies:
//   go get k8s.io/api@v0.32.0
//   go get k8s.io/apimachinery@v0.32.0
//   go get k8s.io/client-go@v0.32.0

package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type KubernetesRuntime struct {
	client    kubernetes.Interface
	config    KubernetesConfig
	namespace string
	mu        sync.Mutex
	pods      map[string]*v1.Pod
}

func NewKubernetesRuntime(cfg KubernetesConfig) (*KubernetesRuntime, error) {
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = "forge"
	}
	var kubeClient kubernetes.Interface
	if cfg.InCluster {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
		kubeClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			return nil, fmt.Errorf("create k8s client: %w", err)
		}
	} else {
		kubeconfig := cfg.KubeconfigPath
		if kubeconfig == "" {
			kubeconfig = os.Getenv("KUBECONFIG")
			if kubeconfig == "" {
				kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
			}
		}
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("build k8s config: %w", err)
		}
		kubeClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			return nil, fmt.Errorf("create k8s client: %w", err)
		}
	}
	return &KubernetesRuntime{
		client:    kubeClient,
		config:    cfg,
		namespace: namespace,
		pods:      make(map[string]*v1.Pod),
	}, nil
}

func (r *KubernetesRuntime) Provider() string {
	return ProviderKubernetes
}

func (r *KubernetesRuntime) Ping(ctx context.Context) error {
	_, err := r.client.CoreV1().Namespaces().Get(ctx, r.namespace, metav1.GetOptions{})
	return err
}

func (r *KubernetesRuntime) Create(ctx context.Context, req CreateRequest) error {
	if err := r.validateCreate(req); err != nil {
		return err
	}
	name := kubePodName(req.ServerID)
	existing, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil && existing != nil {
		if existing.Status.Phase == v1.PodRunning || existing.Status.Phase == v1.PodPending {
			return nil
		}
		if err := r.client.CoreV1().Pods(r.namespace).Delete(ctx, name, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)}); err != nil {
			return fmt.Errorf("delete stale pod: %w", err)
		}
	}
	if err := r.ensureImagePull(ctx, req); err != nil {
		return err
	}
	pod := r.buildPod(req)
	_, err = r.client.CoreV1().Pods(r.namespace).Create(ctx, pod, metav1.CreateOptions{})
	return err
}

func (r *KubernetesRuntime) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	if r.namespace == "" {
		return InstallResult{}, errors.New("namespace is required")
	}
	if req.Image == "" {
		req.Image = "alpine:3.21"
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "sh"
	}
	jobName := kubePodName(req.ServerID) + "-installer"
	_ = r.client.CoreV1().Pods(r.namespace).Delete(ctx, jobName, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)})
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: r.namespace,
			Labels: map[string]string{
				"forge.server_id": req.ServerID,
				"forge.managed":   "true",
				"forge.job":       "install",
			},
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				{
					Name:    "installer",
					Image:   req.Image,
					Command: []string{req.Entrypoint, "-lc", req.Script},
					Env:     envVarsFromSlice(req.Env),
					VolumeMounts: []v1.VolumeMount{
						{Name: "server-data", MountPath: "/mnt/server"},
					},
					SecurityContext: &v1.SecurityContext{
						Privileged:               ptrBool(false),
						AllowPrivilegeEscalation: ptrBool(false),
						ReadOnlyRootFilesystem:   ptrBool(true),
						Capabilities: &v1.Capabilities{
							Drop: []v1.Capability{"ALL"},
						},
					},
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "server-data",
					VolumeSource: v1.VolumeSource{
						HostPath: &v1.HostPathVolumeSource{Path: req.RootDir},
					},
				},
			},
			AutomountServiceAccountToken: ptrBool(false),
		},
	}
	created, err := r.client.CoreV1().Pods(r.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return InstallResult{}, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	watch, err := r.client.CoreV1().Pods(r.namespace).Watch(waitCtx, metav1.ListOptions{
		FieldSelector: fields.Set{"metadata.name": created.Name}.String(),
	})
	if err != nil {
		return InstallResult{}, err
	}
	defer watch.Stop()
	var exitCode int
	for event := range watch.ResultChan() {
		p, ok := event.Object.(*v1.Pod)
		if !ok {
			continue
		}
		if p.Status.Phase == v1.PodSucceeded || p.Status.Phase == v1.PodFailed {
			for _, cs := range p.Status.ContainerStatuses {
				if cs.State.Terminated != nil {
					exitCode = int(cs.State.Terminated.ExitCode)
				}
			}
			break
		}
	}
	logs, err := r.client.CoreV1().Pods(r.namespace).GetLogs(created.Name, &v1.PodLogOptions{}).DoRaw(ctx)
	if err != nil {
		return InstallResult{ExitCode: exitCode}, err
	}
	_ = r.client.CoreV1().Pods(r.namespace).Delete(ctx, created.Name, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)})
	return InstallResult{ExitCode: exitCode, Logs: string(logs)}, nil
}

func (r *KubernetesRuntime) Inspect(ctx context.Context, serverID string) (ContainerState, error) {
	pod, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, kubePodName(serverID), metav1.GetOptions{})
	if err != nil {
		if isNotFound(err) {
			return ContainerState{ServerID: serverID, Exists: false}, nil
		}
		return ContainerState{}, err
	}
	state := ContainerState{
		ServerID: serverID,
		ID:       string(pod.UID),
		Exists:   true,
		Running:  pod.Status.Phase == v1.PodRunning,
		Status:   string(pod.Status.Phase),
	}
	return state, nil
}

func (r *KubernetesRuntime) List(ctx context.Context) ([]ContainerState, error) {
	pods, err := r.client.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "forge.managed=true",
	})
	if err != nil {
		return nil, err
	}
	states := make([]ContainerState, 0, len(pods.Items))
	for _, pod := range pods.Items {
		serverID := pod.Labels["forge.server_id"]
		if serverID == "" {
			continue
		}
		states = append(states, ContainerState{
			ServerID: serverID,
			ID:       string(pod.UID),
			Exists:   true,
			Running:  pod.Status.Phase == v1.PodRunning,
			Status:   string(pod.Status.Phase),
		})
	}
	return states, nil
}

func (r *KubernetesRuntime) Start(ctx context.Context, serverID string) error {
	name := kubePodName(serverID)
	pod, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if pod.Status.Phase == v1.PodRunning {
		return nil
	}
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
		if err := r.client.CoreV1().Pods(r.namespace).Delete(ctx, name, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)}); err != nil {
			return err
		}
		created, err := r.recreatePod(ctx, pod)
		if err != nil {
			return err
		}
		return r.waitForPodRunning(ctx, created.Name)
	}
	return nil
}

func (r *KubernetesRuntime) Stop(ctx context.Context, serverID string) error {
	return r.client.CoreV1().Pods(r.namespace).Delete(ctx, kubePodName(serverID), metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(30)})
}

func (r *KubernetesRuntime) Kill(ctx context.Context, serverID string) error {
	return r.client.CoreV1().Pods(r.namespace).Delete(ctx, kubePodName(serverID), metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)})
}

func (r *KubernetesRuntime) Signal(ctx context.Context, serverID, signal string) error {
	signal = strings.ToUpper(strings.TrimSpace(signal))
	sigNum, ok := signalToNumber(signal)
	if !ok {
		return fmt.Errorf("unsupported signal %q", signal)
	}
	name := kubePodName(serverID)
	containerName := "server"
	cmd := []string{"kill", "-" + strconv.Itoa(sigNum), "1"}
	req := r.client.CoreV1().Pods(r.namespace).GetLogs(name, &v1.PodLogOptions{Container: containerName})
	_ = req
	exec := r.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(r.namespace).
		SubResource("exec").
		Param("container", containerName)
	exec.VersionedParams(&v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
	}, scheme.ParameterCodec)
	config, err := r.restConfig()
	if err != nil {
		return err
	}
	executor, err := remotecommand.NewSPDYExecutor(config, "POST", exec.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
}

func (r *KubernetesRuntime) Restart(ctx context.Context, serverID string) error {
	name := kubePodName(serverID)
	if err := r.client.CoreV1().Pods(r.namespace).Delete(ctx, name, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(30)}); err != nil {
		return err
	}
	for i := 0; i < 60; i++ {
		_, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && isNotFound(err) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	existing, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil && !isNotFound(err) {
		return err
	}
	if existing != nil {
		return r.waitForPodRunning(ctx, name)
	}
	return fmt.Errorf("pod %s was deleted but not recreated", name)
}

func (r *KubernetesRuntime) WaitForStop(ctx context.Context, serverID string, duration time.Duration, terminate bool) error {
	if duration <= 0 {
		duration = 30 * time.Second
	}
	name := kubePodName(serverID)
	waitCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	watcher, err := r.client.CoreV1().Pods(r.namespace).Watch(waitCtx, metav1.ListOptions{
		FieldSelector: fields.Set{"metadata.name": name}.String(),
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()
	for event := range watcher.ResultChan() {
		pod, ok := event.Object.(*v1.Pod)
		if !ok {
			continue
		}
		if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed {
			return nil
		}
		if pod.DeletionTimestamp != nil && (pod.Status.Phase == v1.PodRunning || pod.Status.Phase == v1.PodPending) {
			continue
		}
	}
	if !terminate {
		return context.DeadlineExceeded
	}
	_ = r.client.CoreV1().Pods(r.namespace).Delete(ctx, name, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(10)})
	return nil
}

func (r *KubernetesRuntime) Stats(ctx context.Context, serverID string) (Stats, error) {
	name := kubePodName(serverID)
	pod, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Stats{}, err
	}
	var cpuUsage, memUsage, memLimit uint64
	for _, container := range pod.Status.ContainerStatuses {
		if container.State.Running == nil {
			continue
		}
		resources := container.Resources
		if resources == nil {
			resources = &v1.ResourceRequirements{}
		}
		if q, ok := resources.Limits[v1.ResourceMemory]; ok {
			memLimit += uint64(q.Value())
		}
	}
	config, err := r.restConfig()
	if err != nil {
		return Stats{}, err
	}
	exec := r.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(r.namespace).
		SubResource("exec").
		Param("container", "server").
		Param("stdin", "false").
		Param("stdout", "true").
		Param("stderr", "false").
		Param("tty", "false")
	exec.VersionedParams(&v1.PodExecOptions{
		Command: []string{"cat", "/proc/stat"},
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
	}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(config, "POST", exec.URL())
	if err != nil {
		return Stats{}, err
	}
	var cpuBuf bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &cpuBuf,
		Stderr: io.Discard,
	})
	if err != nil {
		return Stats{}, err
	}
	scanner := bufio.NewScanner(&cpuBuf)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) >= 8 {
				var total uint64
				for _, f := range fields[1:] {
					v, _ := strconv.ParseUint(f, 10, 64)
					total += v
				}
				cpuUsage = total
			}
			break
		}
	}
	exec = r.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(r.namespace).
		SubResource("exec").
		Param("container", "server").
		Param("stdin", "false").
		Param("stdout", "true").
		Param("stderr", "false").
		Param("tty", "false")
	exec.VersionedParams(&v1.PodExecOptions{
		Command: []string{"cat", "/proc/meminfo"},
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
	}, scheme.ParameterCodec)
	var memBuf bytes.Buffer
	executor, err = remotecommand.NewSPDYExecutor(config, "POST", exec.URL())
	if err != nil {
		return Stats{}, err
	}
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &memBuf,
		Stderr: io.Discard,
	})
	if err != nil {
		return Stats{}, err
	}
	scanner = bufio.NewScanner(&memBuf)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, _ := strconv.ParseUint(fields[1], 10, 64)
				if memLimit == 0 {
					memLimit = v * 1024
				}
			}
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				v, _ := strconv.ParseUint(fields[1], 10, 64)
				memUsage = memLimit - v*1024
			}
		}
	}
	cpuPercent := float64(0)
	if cpuUsage > 0 {
		cpuPercent = float64(cpuUsage) / 100.0
	}
	return Stats{
		CPUPercent:  cpuPercent,
		MemoryBytes: memUsage,
		MemoryLimit: memLimit,
	}, nil
}

func (r *KubernetesRuntime) Logs(ctx context.Context, serverID string) (io.ReadCloser, error) {
	name := kubePodName(serverID)
	logOpts := &v1.PodLogOptions{
		Container: "server",
		Follow:    false,
	}
	req := r.client.CoreV1().Pods(r.namespace).GetLogs(name, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (r *KubernetesRuntime) LogsStream(ctx context.Context, serverID string, tail string) (io.ReadCloser, error) {
	name := kubePodName(serverID)
	tailLines := int64(50)
	if tail != "" {
		if n, err := strconv.ParseInt(tail, 10, 64); err == nil && n > 0 {
			tailLines = n
		}
	}
	logOpts := &v1.PodLogOptions{
		Container: "server",
		Follow:    true,
		TailLines: &tailLines,
	}
	req := r.client.CoreV1().Pods(r.namespace).GetLogs(name, logOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (r *KubernetesRuntime) StatsStream(ctx context.Context, serverID string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats, err := r.Stats(ctx, serverID)
				if err != nil {
					return
				}
				data, _ := json.Marshal(stats)
				data = append(data, '\n')
				if _, err := pw.Write(data); err != nil {
					return
				}
			}
		}
	}()
	return pr, nil
}

func (r *KubernetesRuntime) AttachConsole(ctx context.Context, serverID string) (ConsoleSession, error) {
	name := kubePodName(serverID)
	config, err := r.restConfig()
	if err != nil {
		return nil, err
	}
	exec := r.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(r.namespace).
		SubResource("exec").
		Param("container", "server").
		Param("stdin", "true").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "true")
	exec.VersionedParams(&v1.PodExecOptions{
		Command: []string{"/bin/sh"},
		Stdin:   true,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(config, "POST", exec.URL())
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	session := &kubeConsoleSession{
		pw:       pw,
		pr:       pr,
		executor: executor,
		closed:   make(chan struct{}),
	}
	go func() {
		err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  session.pr,
			Stdout: session.pw,
			Stderr: session.pw,
			Tty:    true,
		})
		if err != nil {
			_ = session.pw.CloseWithError(err)
		}
		close(session.closed)
	}()
	return session, nil
}

func (r *KubernetesRuntime) SendCommand(ctx context.Context, serverID, command string) error {
	name := kubePodName(serverID)
	config, err := r.restConfig()
	if err != nil {
		return err
	}
	shell := []string{"/bin/sh", "-c", command}
	exec := r.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(r.namespace).
		SubResource("exec").
		Param("container", "server").
		Param("stdin", "false").
		Param("stdout", "true").
		Param("stderr", "true")
	exec.VersionedParams(&v1.PodExecOptions{
		Command: shell,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
	}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(config, "POST", exec.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
}

func (r *KubernetesRuntime) Delete(ctx context.Context, serverID string) error {
	_ = r.client.CoreV1().Pods(r.namespace).Delete(ctx, kubePodName(serverID)+"-installer", metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)})
	return r.client.CoreV1().Pods(r.namespace).Delete(ctx, kubePodName(serverID), metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)})
}

func (r *KubernetesRuntime) WatchEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	out := make(chan ContainerEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		backoff := 100 * time.Millisecond
		for ctx.Err() == nil {
			watcher, err := r.client.CoreV1().Pods(r.namespace).Watch(ctx, metav1.ListOptions{
				LabelSelector: "forge.managed=true",
			})
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				timer := time.NewTimer(backoff)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return
				}
				if backoff < 5*time.Second {
					backoff *= 2
					if backoff > 5*time.Second {
						backoff = 5 * time.Second
					}
				}
				continue
			}
			backoff = 100 * time.Millisecond
			for event := range watcher.ResultChan() {
				pod, ok := event.Object.(*v1.Pod)
				if !ok {
					continue
				}
				serverID := pod.Labels["forge.server_id"]
				if serverID == "" {
					continue
				}
				exitCode := 0
				oomKilled := false
				if event.Type == watch.Deleted || event.Type == watch.Modified {
					for _, cs := range pod.Status.ContainerStatuses {
						if cs.State.Terminated != nil {
							exitCode = int(cs.State.Terminated.ExitCode)
							oomKilled = cs.State.Terminated.Reason == "OOMKilled"
						}
					}
				}
				select {
				case out <- ContainerEvent{
					ServerID:  serverID,
					Action:    string(event.Type),
					ExitCode:  exitCode,
					OOMKilled: oomKilled,
				}:
				case <-ctx.Done():
					watcher.Stop()
					return
				}
			}
		}
	}()
	return out, errs
}

func (r *KubernetesRuntime) buildPod(req CreateRequest) *v1.Pod {
	name := kubePodName(req.ServerID)
	labels := map[string]string{
		"forge.server_id": req.ServerID,
		"forge.managed":   "true",
	}
	container := v1.Container{
		Name:    "server",
		Image:   req.Image,
		Command: req.Command,
		Env:     envVarsFromSlice(req.Env),
		Ports:   containerPorts(req.Ports),
		Resources: v1.ResourceRequirements{
			Limits:   buildResourceLimits(req),
			Requests: buildResourceRequests(req),
		},
		SecurityContext: &v1.SecurityContext{
			Privileged:               ptrBool(false),
			AllowPrivilegeEscalation: ptrBool(false),
			ReadOnlyRootFilesystem:   ptrBool(true),
			Capabilities: &v1.Capabilities{
				Drop: []v1.Capability{"ALL"},
			},
		},
	}
	volumes := []v1.Volume{}
	mounts := []v1.VolumeMount{}
	if req.RootDir != "" {
		volumes = append(volumes, v1.Volume{
			Name: "server-data",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: req.RootDir},
			},
		})
		mounts = append(mounts, v1.VolumeMount{
			Name:      "server-data",
			MountPath: "/home/container",
		})
	}
	for i, m := range req.Mounts {
		if m.Source == "" || m.Target == "" {
			continue
		}
		volName := fmt.Sprintf("mount-%d", i)
		volumes = append(volumes, v1.Volume{
			Name: volName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: m.Source},
			},
		})
		mounts = append(mounts, v1.VolumeMount{
			Name:      volName,
			MountPath: m.Target,
			ReadOnly:  m.ReadOnly,
		})
	}
	container.VolumeMounts = mounts
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.namespace,
			Labels:    labels,
		},
		Spec: v1.PodSpec{
			Containers:                []v1.Container{container},
			Volumes:                   volumes,
			RestartPolicy:             v1.RestartPolicyAlways,
			AutomountServiceAccountToken: ptrBool(false),
			DNSPolicy:                 v1.DNSDefault,
		},
	}
	if req.RegistryAuth != nil {
		pod.Spec.ImagePullSecrets = []v1.LocalObjectReference{
			{Name: r.ensurePullSecret(context.Background(), req.RegistryAuth)},
		}
	}
	if req.DNS != nil {
		pod.Spec.DNSConfig = &v1.PodDNSConfig{Nameservers: req.DNS}
	}
	return pod
}

func (r *KubernetesRuntime) ensureImagePull(ctx context.Context, req CreateRequest) error {
	if req.RegistryAuth == nil {
		return nil
	}
	secretName := "forge-pull-" + req.ServerID
	existing, err := r.client.CoreV1().Secrets(r.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil && existing != nil {
		return nil
	}
	data := map[string][]byte{}
	auth := map[string]any{
		"auths": map[string]any{
			req.RegistryAuth.ServerAddress: map[string]string{
				"username":      req.RegistryAuth.Username,
				"password":      req.RegistryAuth.Password,
				"auth":          encodeBase64(req.RegistryAuth.Username + ":" + req.RegistryAuth.Password),
				"identitytoken": req.RegistryAuth.IdentityToken,
				"registrytoken": req.RegistryAuth.RegistryToken,
			},
		},
	}
	body, _ := json.Marshal(auth)
	data[v1.DockerConfigJsonKey] = body
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.namespace,
		},
		Type: v1.SecretTypeDockerConfigJson,
		Data: data,
	}
	_, err = r.client.CoreV1().Secrets(r.namespace).Create(ctx, secret, metav1.CreateOptions{})
	return err
}

func (r *KubernetesRuntime) ensurePullSecret(ctx context.Context, auth *RegistryAuth) string {
	secretName := "forge-pull-" + strings.ReplaceAll(auth.ServerAddress, ":", "-")
	existing, err := r.client.CoreV1().Secrets(r.namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil && existing != nil {
		return secretName
	}
	dockerConfig := map[string]any{
		"auths": map[string]any{
			auth.ServerAddress: map[string]string{
				"username": auth.Username,
				"password": auth.Password,
				"auth":     encodeBase64(auth.Username + ":" + auth.Password),
			},
		},
	}
	cfgBytes, _ := json.Marshal(dockerConfig)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.namespace,
		},
		Type: v1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{v1.DockerConfigJsonKey: cfgBytes},
	}
	created, err := r.client.CoreV1().Secrets(r.namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return secretName
	}
	return created.Name
}

func (r *KubernetesRuntime) recreatePod(ctx context.Context, orig *v1.Pod) (*v1.Pod, error) {
	newPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orig.Name,
			Namespace: orig.Namespace,
			Labels:    orig.Labels,
		},
		Spec: orig.Spec,
	}
	return r.client.CoreV1().Pods(r.namespace).Create(ctx, newPod, metav1.CreateOptions{})
}

func (r *KubernetesRuntime) waitForPodRunning(ctx context.Context, name string) error {
	watch, err := r.client.CoreV1().Pods(r.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.Set{"metadata.name": name}.String(),
	})
	if err != nil {
		return err
	}
	defer watch.Stop()
	timeout := time.After(60 * time.Second)
	for {
		select {
		case event, ok := <-watch.ResultChan():
			if !ok {
				return errors.New("watch closed")
			}
			pod, ok := event.Object.(*v1.Pod)
			if !ok {
				continue
			}
			switch pod.Status.Phase {
			case v1.PodRunning:
				return nil
			case v1.PodSucceeded, v1.PodFailed:
				return nil
			}
		case <-timeout:
			return errors.New("timeout waiting for pod to start")
		}
	}
}

func (r *KubernetesRuntime) restConfig() (*rest.Config, error) {
	if r.config.InCluster {
		return rest.InClusterConfig()
	}
	kubeconfig := r.config.KubeconfigPath
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func (r *KubernetesRuntime) validateCreate(req CreateRequest) error {
	if strings.TrimSpace(req.ServerID) == "" || strings.TrimSpace(req.Image) == "" {
		return errors.New("server ID and image are required")
	}
	if req.MemoryMB < 0 || req.SwapMB < 0 {
		return errors.New("memory and swap must not be negative")
	}
	if req.SwapMB > 0 && req.MemoryMB == 0 {
		return errors.New("swap requires a positive memory limit")
	}
	return nil
}

func kubePodName(serverID string) string {
	return "forge-" + serverID
}

func envVarsFromSlice(env []string) []v1.EnvVar {
	if len(env) == 0 {
		return nil
	}
	vars := make([]v1.EnvVar, 0, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		}
		vars = append(vars, v1.EnvVar{Name: parts[0], Value: parts[1]})
	}
	return vars
}

func containerPorts(ports []PortBinding) []v1.ContainerPort {
	if len(ports) == 0 {
		return nil
	}
	cp := make([]v1.ContainerPort, 0, len(ports))
	for _, p := range ports {
		cp = append(cp, v1.ContainerPort{
			ContainerPort: int32(p.ContainerPort),
			Protocol:      v1.Protocol(strings.ToUpper(p.Protocol)),
			HostPort:      int32(p.HostPort),
			HostIP:        p.HostIP,
		})
	}
	return cp
}

func buildResourceLimits(req CreateRequest) v1.ResourceList {
	limits := v1.ResourceList{}
	if req.CPUPercent > 0 {
		limits[v1.ResourceCPU] = resource.MustParse(fmt.Sprintf("%dm", req.CPUPercent*10))
	} else if req.CPUShares > 0 {
		limits[v1.ResourceCPU] = resource.MustParse(fmt.Sprintf("%dm", req.CPUShares))
	}
	if req.MemoryMB > 0 {
		limits[v1.ResourceMemory] = resource.MustParse(fmt.Sprintf("%dMi", req.MemoryMB))
	}
	return limits
}

func buildResourceRequests(req CreateRequest) v1.ResourceList {
	requests := v1.ResourceList{}
	if req.CPUPercent > 0 {
		requests[v1.ResourceCPU] = resource.MustParse(fmt.Sprintf("%dm", req.CPUPercent*10))
	} else if req.CPUShares > 0 {
		requests[v1.ResourceCPU] = resource.MustParse(fmt.Sprintf("%dm", req.CPUShares))
	}
	if req.MemoryMB > 0 {
		requests[v1.ResourceMemory] = resource.MustParse(fmt.Sprintf("%dMi", req.MemoryMB))
	}
	return requests
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

func encodeBase64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func signalToNumber(signal string) (int, bool) {
	switch strings.ToUpper(strings.TrimSpace(signal)) {
	case "SIGTERM":
		return 15, true
	case "SIGINT":
		return 2, true
	case "SIGQUIT":
		return 3, true
	case "SIGHUP":
		return 1, true
	case "SIGUSR1":
		return 10, true
	case "SIGUSR2":
		return 12, true
	case "SIGKILL":
		return 9, true
	}
	return 0, false
}

type kubeConsoleSession struct {
	pw       *io.PipeWriter
	pr       *io.PipeReader
	executor remotecommand.Executor
	closed   chan struct{}
	closeOnce sync.Once
}

func (s *kubeConsoleSession) Read(p []byte) (int, error) {
	return s.pr.Read(p)
}

func (s *kubeConsoleSession) Write(p []byte) (int, error) {
	select {
	case <-s.closed:
		return 0, io.ErrClosedPipe
	default:
	}
	n, err := s.pw.Write(p)
	return n, err
}

func (s *kubeConsoleSession) Close() error {
	s.closeOnce.Do(func() {
		_ = s.pw.Close()
		_ = s.pr.Close()
	})
	return nil
}
