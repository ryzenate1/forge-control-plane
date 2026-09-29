// Required dependencies:
//   go get k8s.io/api@v0.32.0
//   go get k8s.io/apimachinery@v0.32.0
//   go get k8s.io/client-go@v0.32.0

package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/intstr"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
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

func (r *KubernetesRuntime) Close() error { return nil }

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
	var err error
	req, err = canonicalizeKubernetesMounts(req)
	if err != nil {
		return err
	}
	hash, err := createRequestHash(req)
	if err != nil {
		return err
	}
	name := kubePodName(req.ServerID)
	existing, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil && existing != nil {
		// Idempotent by hash: an identical Running/Pending pod is a no-op.
		// A hash mismatch means the desired config drifted, so the stale
		// pod is replaced below instead of being silently kept.
		if existing.Status.Phase == v1.PodRunning || existing.Status.Phase == v1.PodPending {
			if existing.Labels != nil && existing.Labels[configHashLabel] == hash {
				return nil
			}
		} else if existing.Labels != nil && existing.Labels[configHashLabel] == hash {
			// Terminal pod with identical config: leave replacement to
			// Start, which recreates Succeeded/Failed pods.
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
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[configHashLabel] = hash
	_, err = r.client.CoreV1().Pods(r.namespace).Create(ctx, pod, metav1.CreateOptions{})
	return err
}

func (r *KubernetesRuntime) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	if r.namespace == "" {
		return InstallResult{}, errors.New("namespace is required")
	}
	// Installer images must be digest-pinned like every other runtime, and
	// the host path must pass the same allowlist as workload mounts: an
	// installer with an unpinned image or an arbitrary HostPath would run
	// untrusted content with node-filesystem access.
	if req.Image == "" {
		req.Image = "docker.io/library/alpine:3.21@sha256:21a3deaa0d32a8057914f36584b5288d2e5da9845c690f493846b7b90a70dbcd"
	}
	if !pinnedImagePattern.MatchString(req.Image) {
		return InstallResult{}, fmt.Errorf("remote image %q is not digest-pinned; use name@sha256:<64 hex characters>", req.Image)
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "sh"
	}
	rootDir, err := validateRootDir(req.RootDir)
	if err != nil {
		return InstallResult{}, err
	}
	if err := k8sHostPathAllowed(rootDir); err != nil {
		return InstallResult{}, fmt.Errorf("root dir: %w", err)
	}
	req.RootDir = rootDir
	jobName := kubePodName(req.ServerID) + "-installer"
	_ = r.client.CoreV1().Pods(r.namespace).Delete(ctx, jobName, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)})
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: r.namespace,
			Labels: map[string]string{
				"modern-game-panel.server_id": req.ServerID,
				"modern-game-panel.managed":   "true",
				"modern-game-panel.job":       "install",
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
						HostPath: &v1.HostPathVolumeSource{Path: req.RootDir, Type: func() *v1.HostPathType {
							value := v1.HostPathDirectory
							return &value
						}()},
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
	if pod.Status.StartTime != nil {
		state.StartedAt = pod.Status.StartTime.Time
	}
	return state, nil
}

func (r *KubernetesRuntime) List(ctx context.Context) ([]ContainerState, error) {
	pods, err := r.client.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "modern-game-panel.managed=true",
	})
	if err != nil {
		return nil, err
	}
	states := make([]ContainerState, 0, len(pods.Items))
	for _, pod := range pods.Items {
		// Installer pods share the managed label but are not workloads:
		// listing them as servers would surface phantom entries (and
		// collide by server_id with the real workload pod).
		if pod.Labels["modern-game-panel.job"] == "install" {
			continue
		}
		serverID := pod.Labels["modern-game-panel.server_id"]
		if serverID == "" {
			continue
		}
		state := ContainerState{
			ServerID: serverID,
			ID:       string(pod.UID),
			Exists:   true,
			Running:  pod.Status.Phase == v1.PodRunning,
			Status:   string(pod.Status.Phase),
		}
		if pod.Status.StartTime != nil {
			state.StartedAt = pod.Status.StartTime.Time
		}
		states = append(states, state)
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
	// A Pending pod has been accepted but is not serving yet. Returning nil
	// here would report a workload that cannot serve traffic as started, so
	// wait for Running (or a terminal phase) instead.
	if pod.Status.Phase == v1.PodPending {
		return r.waitForPodRunning(ctx, name)
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
	pod, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := r.client.CoreV1().Pods(r.namespace).Delete(ctx, name, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(30)}); err != nil {
		return err
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()
	for {
		_, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil && isNotFound(err) {
			break
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout.C:
			return fmt.Errorf("timed out waiting for pod %s deletion", name)
		case <-ticker.C:
		}
	}
	created, err := r.recreatePod(ctx, pod)
	if err != nil {
		return fmt.Errorf("recreate pod %s: %w", name, err)
	}
	return r.waitForPodRunning(ctx, created.Name)
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
	if err := r.client.CoreV1().Pods(r.namespace).Delete(ctx, name, metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)}); err != nil && !isNotFound(err) {
		return err
	}
	confirmCtx, confirmCancel := context.WithTimeout(ctx, 30*time.Second)
	defer confirmCancel()
	confirm, err := r.client.CoreV1().Pods(r.namespace).Watch(confirmCtx, metav1.ListOptions{
		FieldSelector: fields.Set{"metadata.name": name}.String(),
	})
	if err != nil {
		return err
	}
	defer confirm.Stop()
	for {
		select {
		case event, ok := <-confirm.ResultChan():
			if !ok {
				return errors.New("pod deletion watch closed before confirmation")
			}
			if event.Type == watch.Deleted {
				return nil
			}
		case <-confirmCtx.Done():
			return fmt.Errorf("forced pod deletion was not confirmed: %w", confirmCtx.Err())
		}
	}
}

func (r *KubernetesRuntime) Stats(ctx context.Context, serverID string) (Stats, error) {
	name := kubePodName(serverID)
	pod, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Stats{}, err
	}
	if pod.Status.Phase != v1.PodRunning {
		return Stats{}, fmt.Errorf("pod %q is %q: no metrics to report", name, pod.Status.Phase)
	}
	var memLimit uint64
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
	// Usage requires metrics.k8s.io. Do not exec tools inside an untrusted game
	// container or misreport node-wide /proc values as pod utilization.
	// Unknown is not zero: usage is unavailable without a metrics backend,
	// so report the failure instead of a healthy-looking zero reading. The
	// limit is still returned alongside the error for callers that can use
	// placement data without live usage.
	if memLimit == 0 {
		return Stats{}, errors.New("kubernetes metrics are unavailable: no memory limit observed and no usage backend configured")
	}
	return Stats{MemoryLimit: memLimit}, fmt.Errorf("kubernetes pod usage metrics are unavailable (memory limit %d bytes, no metrics.k8s.io backend); not reporting zero usage as healthy", memLimit)
}

func (r *KubernetesRuntime) Logs(ctx context.Context, serverID string) (io.ReadCloser, error) {
	name := kubePodName(serverID)
	// A pod that has never started has no output. The kubelet answers
	// GetLogs on a Pending pod with an empty stream and nil error, which
	// reads as "healthy but quiet"; refuse it instead.
	if pod, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, name, metav1.GetOptions{}); err == nil && pod != nil {
		if pod.Status.Phase == v1.PodPending && pod.Status.StartTime == nil {
			everRan := false
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Terminated != nil || cs.State.Running != nil {
					everRan = true
					break
				}
			}
			if !everRan {
				return nil, fmt.Errorf("pod %q is pending and has never started: no logs to report", name)
			}
		}
	}
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
	// Align with Docker LogsStream: empty or "all" means the last 10000
	// lines, not 50. A 50-line default would silently truncate install
	// output that Docker callers can still see.
	tailLines := int64(10000)
	if strings.TrimSpace(tail) != "" && tail != "all" {
		if n, err := strconv.ParseInt(strings.TrimSpace(tail), 10, 64); err == nil && n >= 0 {
			tailLines = n
		} else {
			return nil, errors.New("tail must be a non-negative integer or all")
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
					// Propagate instead of a silent EOF: the reader must see
					// the failure, not an idle stream that looks healthy.
					_ = pw.CloseWithError(err)
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
	const maxKubernetesCommandBytes = 4 * 1024
	if len(command) > maxKubernetesCommandBytes {
		return fmt.Errorf("command exceeds %d byte limit", maxKubernetesCommandBytes)
	}
	if strings.ContainsRune(command, '\x00') {
		return errors.New("command contains invalid characters")
	}
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
	if err := r.client.CoreV1().Pods(r.namespace).Delete(ctx, kubePodName(serverID), metav1.DeleteOptions{GracePeriodSeconds: ptrInt64(0)}); err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

func (r *KubernetesRuntime) WatchEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	out := make(chan ContainerEvent, 128)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		backoff := time.Second
		for ctx.Err() == nil {
			watcher, err := r.client.CoreV1().Pods(r.namespace).Watch(ctx, metav1.ListOptions{
				LabelSelector: "modern-game-panel.managed=true",
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
			backoff = time.Second
			for event := range watcher.ResultChan() {
				pod, ok := event.Object.(*v1.Pod)
				if !ok {
					continue
				}
				serverID := pod.Labels["modern-game-panel.server_id"]
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
				default:
					select {
					case errs <- errors.New("Kubernetes event dropped because the consumer is not keeping up"):
					default:
					}
				}
			}
		}
	}()
	return out, errs
}

func (r *KubernetesRuntime) buildPod(req CreateRequest) *v1.Pod {
	name := kubePodName(req.ServerID)
	labels := map[string]string{
		"modern-game-panel.server_id": req.ServerID,
		"modern-game-panel.managed":   "true",
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
	// UID/GID are part of the workload contract, not advisory metadata: a
	// pod that ignores them runs as root instead of the requested user.
	if req.UID != 0 || req.GID != 0 {
		uid := int64(req.UID)
		gid := int64(req.GID)
		if req.UID != 0 {
			container.SecurityContext.RunAsUser = &uid
			container.SecurityContext.RunAsNonRoot = ptrBool(true)
		}
		if req.GID != 0 {
			container.SecurityContext.RunAsGroup = &gid
		}
	}
	volumes := []v1.Volume{}
	mounts := []v1.VolumeMount{}
	if req.RootDir != "" {
		hostPathType := v1.HostPathDirectory
		volumes = append(volumes, v1.Volume{
			Name: "server-data",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: req.RootDir, Type: &hostPathType},
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
		hostPathType := v1.HostPathDirectory
		volumes = append(volumes, v1.Volume{
			Name: volName,
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{Path: m.Source, Type: &hostPathType},
			},
		})
		mounts = append(mounts, v1.VolumeMount{
			Name:      volName,
			MountPath: m.Target,
			ReadOnly:  m.ReadOnly,
		})
	}
	container.VolumeMounts = mounts
	if len(container.Ports) > 0 {
		probe := &v1.Probe{
			ProbeHandler: v1.ProbeHandler{TCPSocket: &v1.TCPSocketAction{
				Port: intstr.FromInt32(container.Ports[0].ContainerPort),
			}},
			InitialDelaySeconds: 15,
			PeriodSeconds:       10,
			TimeoutSeconds:      3,
			FailureThreshold:    6,
		}
		container.ReadinessProbe = probe.DeepCopy()
		container.LivenessProbe = probe
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.namespace,
			Labels:    labels,
		},
		Spec: v1.PodSpec{
			Containers:                   []v1.Container{container},
			Volumes:                      volumes,
			RestartPolicy:                v1.RestartPolicyNever,
			AutomountServiceAccountToken: ptrBool(false),
			DNSPolicy:                    v1.DNSDefault,
		},
	}
	if req.RegistryAuth != nil {
		pod.Spec.ImagePullSecrets = []v1.LocalObjectReference{
			{Name: imagePullSecretName(req.ServerID)},
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
	secretName := imagePullSecretName(req.ServerID)
	existing, err := r.client.CoreV1().Secrets(r.namespace).Get(ctx, secretName, metav1.GetOptions{})
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
	if err == nil && existing != nil {
		secret.ResourceVersion = existing.ResourceVersion
		_, err = r.client.CoreV1().Secrets(r.namespace).Update(ctx, secret, metav1.UpdateOptions{})
		return err
	}
	if err != nil && !isNotFound(err) {
		return err
	}
	_, err = r.client.CoreV1().Secrets(r.namespace).Create(ctx, secret, metav1.CreateOptions{})
	return err
}

func imagePullSecretName(serverID string) string {
	sum := sha256.Sum256([]byte(serverID))
	return "gamepanel-pull-" + hex.EncodeToString(sum[:8])
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
	if !validContainerID(req.ServerID) {
		return errors.New("server ID contains invalid characters or is too long")
	}
	// Pod names are derived as "forge-<serverID>" and must be DNS1123
	// subdomains. Docker-style IDs (uppercase, underscores) would be
	// accepted above but rejected by the API server at create time, so
	// refuse them here with a clear error instead of a late API failure.
	if errs := kvalidation.IsDNS1123Subdomain(kubePodName(req.ServerID)); len(errs) != 0 {
		return fmt.Errorf("server ID %q is not a valid DNS1123 subdomain for a pod name: %s", req.ServerID, strings.Join(errs, "; "))
	}
	// The pod spec only enforces CPU and memory. Every other Docker limit
	// below is silently dropped by buildResourceLimits, so accepting it
	// would run an unconstrained workload behind a constrained request.
	// Reject what cannot be honoured instead of misreporting it.
	if req.SwapMB != 0 {
		return errors.New("kubernetes runtime does not enforce swap limits; omit swapMb")
	}
	if req.IOWeight != 0 {
		return errors.New("kubernetes runtime does not enforce IO weight; omit ioWeight")
	}
	if req.PIDLimit > 0 {
		return errors.New("kubernetes runtime does not enforce PID limits; omit pidLimit")
	}
	if req.OOMKillDisabled {
		return errors.New("kubernetes runtime cannot disable OOM kill; omit oomKillDisabled")
	}
	if strings.TrimSpace(req.CPUSet) != "" {
		return errors.New("kubernetes runtime does not support CPU pinning; omit cpuSet")
	}
	if strings.TrimSpace(req.NetworkName) != "" || strings.TrimSpace(req.NetworkSubnet) != "" || strings.TrimSpace(req.NetworkGateway) != "" || strings.TrimSpace(req.NetworkIP) != "" {
		return errors.New("kubernetes runtime does not manage Docker networks; omit networkName/subnet/gateway/ip")
	}
	if req.MemoryMB < 0 || req.SwapMB < 0 {
		return errors.New("memory and swap must not be negative")
	}
	if req.MemoryOverhead < 0 || req.MemoryOverhead > 100 {
		return errors.New("memory overhead must be between 0 and 100 percent")
	}
	if req.SwapMB > 0 && req.MemoryMB == 0 {
		return errors.New("swap requires a positive memory limit")
	}
	if req.CPUShares < 0 || req.CPUPercent < 0 || req.CPUPercent > 100000 {
		return errors.New("CPU limits are outside the supported range")
	}
	if req.IOWeight != 0 && (req.IOWeight < 10 || req.IOWeight > 1000) {
		return errors.New("IO weight must be 0 or between 10 and 1000")
	}
	if req.PIDLimit < -1 {
		return errors.New("PID limit must be -1, 0, or positive")
	}
	for _, entry := range req.Env {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || len(kvalidation.IsEnvVarName(parts[0])) != 0 {
			return fmt.Errorf("invalid environment entry %q", entry)
		}
	}
	for _, dns := range req.DNS {
		if net.ParseIP(dns) == nil {
			return fmt.Errorf("invalid DNS server %q", dns)
		}
	}
	_, _, err := dockerPorts(req.Ports)
	return err
}

// k8sProtectedHostPrefixes are host locations a workload pod must never
// mount, even when an allowlist is configured. HostPath volumes expose the
// node filesystem directly, so custom mounts are confined to explicitly
// allowed roots and can never reach system or Beacon data directories.
var k8sProtectedHostPrefixes = []string{
	"/", "/etc", "/proc", "/sys", "/dev", "/boot",
	"/usr", "/bin", "/sbin", "/lib", "/lib64",
	"/root", "/var/run", "/run", "/var/lib/kubelet",
}

func k8sHostPathAllowed(source string) error {
	for _, prefix := range k8sProtectedHostPrefixes {
		if source == prefix || strings.HasPrefix(source, strings.TrimSuffix(prefix, "/")+"/") {
			return fmt.Errorf("host path %q is in a protected system location", source)
		}
	}
	if extra := strings.TrimSpace(os.Getenv("DAEMON_K8S_ALLOWED_HOSTPATHS")); extra != "" {
		for _, root := range strings.Split(extra, ",") {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			if source == root || strings.HasPrefix(source, strings.TrimSuffix(root, "/")+"/") {
				return nil
			}
		}
		return fmt.Errorf("host path %q is outside DAEMON_K8S_ALLOWED_HOSTPATHS", source)
	}
	// Without an explicit allowlist, only the Beacon data directory subtree is
	// eligible: workloads mount server data, not arbitrary host paths.
	dataDir := strings.TrimSpace(os.Getenv("DAEMON_DATA_DIR"))
	if dataDir == "" {
		return fmt.Errorf("host path %q requires DAEMON_K8S_ALLOWED_HOSTPATHS to be configured", source)
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Clean(dataDir)); err == nil {
		dataDir = resolved
	}
	if source == dataDir || strings.HasPrefix(source, strings.TrimSuffix(dataDir, "/")+"/") {
		return nil
	}
	return fmt.Errorf("host path %q is outside the Beacon data directory; set DAEMON_K8S_ALLOWED_HOSTPATHS to grant explicit roots", source)
}

func canonicalizeKubernetesMounts(req CreateRequest) (CreateRequest, error) {
	if req.RootDir != "" {
		root, err := validateRootDir(req.RootDir)
		if err != nil {
			return req, err
		}
		if err := k8sHostPathAllowed(root); err != nil {
			return req, fmt.Errorf("root dir: %w", err)
		}
		req.RootDir = root
	}
	for index := range req.Mounts {
		source, err := validateRootDir(req.Mounts[index].Source)
		if err != nil {
			return req, fmt.Errorf("validate mount %d: %w", index, err)
		}
		if err := k8sHostPathAllowed(source); err != nil {
			return req, fmt.Errorf("mount %d: %w", index, err)
		}
		target := pathpkg.Clean(req.Mounts[index].Target)
		if !pathpkg.IsAbs(target) || target == "/" || target == serverContainerRoot {
			return req, fmt.Errorf("mount %d target must be an absolute path below / and may not replace %s", index, serverContainerRoot)
		}
		req.Mounts[index].Source = source
		req.Mounts[index].Target = target
	}
	return req, nil
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
		// CPUShares are a relative weight (1024 == 1 CPU), not millicores.
		// Reporting shares directly as millicores under-limits small values
		// and over-limits large ones; convert through the 1024==1000m ratio.
		millicores := req.CPUShares * 1000 / 1024
		if millicores < 1 {
			millicores = 1
		}
		limits[v1.ResourceCPU] = *resource.NewMilliQuantity(millicores, resource.DecimalSI)
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
		millicores := req.CPUShares * 1000 / 1024
		if millicores < 1 {
			millicores = 1
		}
		requests[v1.ResourceCPU] = *resource.NewMilliQuantity(millicores, resource.DecimalSI)
	}
	if req.MemoryMB > 0 {
		requests[v1.ResourceMemory] = resource.MustParse(fmt.Sprintf("%dMi", req.MemoryMB))
	}
	return requests
}

func isNotFound(err error) bool {
	return apierrors.IsNotFound(err)
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
	pw        *io.PipeWriter
	pr        *io.PipeReader
	executor  remotecommand.Executor
	closed    chan struct{}
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
