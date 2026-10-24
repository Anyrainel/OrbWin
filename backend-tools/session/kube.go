// OrbWin's per-session Kubernetes exec client. client-go versions match the
// effective versions in the pinned rdd module, including its replacements.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

var podName = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9.]{0,251}[a-z0-9])?$`)

func validatePodOpen(msg message) error {
	if msg.Kind != "open" || !podName.MatchString(msg.Namespace) || len(msg.Namespace) > 63 || !podName.MatchString(msg.Pod) || msg.PodUID == "" || len(msg.PodUID) > 128 || strings.ContainsAny(msg.PodUID, "\x00\r\n") {
		return errors.New("pod terminal requires namespace, pod name and returned UID")
	}
	if msg.ContainerName != "" && (!podName.MatchString(msg.ContainerName) || len(msg.ContainerName) > 63) {
		return errors.New("invalid pod container name")
	}
	if msg.Kubeconfig == "" || len(msg.Kubeconfig) > 4096 || strings.ContainsAny(msg.Kubeconfig, "\x00\r\n") {
		return errors.New("owned kubeconfig path is required")
	}
	if msg.Shell != "sh" && msg.Shell != "bash" {
		return errors.New("unsupported container shell")
	}
	return dimensions(msg.Cols, msg.Rows)
}

// Never execute a kubeconfig credential plugin or consult inherited proxies.
// The native adapter supplies only the owned mirrored config; its server must
// remain a TLS-authenticated loopback endpoint, not a renderer-provided URL.
func podConfig(path string) (*rest.Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	bytes, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(bytes) > 1<<20 {
		return nil, errors.New("owned kubeconfig exceeds 1 MiB")
	}
	config, err := clientcmd.Load(bytes)
	if err != nil {
		return nil, err
	}
	for _, auth := range config.AuthInfos {
		if auth.Exec != nil || auth.AuthProvider != nil {
			return nil, errors.New("kubeconfig credential plugins are not allowed for local pod terminals")
		}
	}
	for _, cluster := range config.Clusters {
		server, err := url.Parse(cluster.Server)
		if err != nil || server.Scheme != "https" || (server.Hostname() != "127.0.0.1" && server.Hostname() != "::1") || server.User != nil || cluster.InsecureSkipTLSVerify || cluster.ProxyURL != "" {
			return nil, errors.New("pod terminal requires the owned TLS loopback cluster endpoint")
		}
	}
	result, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, err
	}
	result.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	return result, nil
}

type podSizes struct {
	ctx   context.Context
	sizes chan remotecommand.TerminalSize
	ready func()
}

func (q *podSizes) Next() *remotecommand.TerminalSize {
	q.ready()
	select {
	case size := <-q.sizes:
		return &size
	case <-q.ctx.Done():
		return nil
	}
}

type readyOutput struct {
	io.Writer
	ready func()
}

func (w readyOutput) Write(data []byte) (int, error) { w.ready(); return w.Writer.Write(data) }

func runKubernetes(msg message, scanner *bufio.Scanner, output io.Writer) (int, error) {
	config, err := podConfig(msg.Kubeconfig)
	if err != nil {
		return 125, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return 125, err
	}
	ctx, cancel := bounded()
	pod, err := client.CoreV1().Pods(msg.Namespace).Get(ctx, msg.Pod, metav1.GetOptions{})
	cancel()
	if err != nil {
		return 125, err
	}
	if string(pod.UID) != msg.PodUID {
		return 125, errors.New("pod was replaced; refresh before opening a terminal")
	}
	if pod.Status.Phase != v1.PodRunning {
		return 125, errors.New("pod is not running")
	}
	container := msg.ContainerName
	if container == "" && len(pod.Spec.Containers) == 1 {
		container = pod.Spec.Containers[0].Name
	}
	found := false
	for _, entry := range pod.Spec.Containers {
		found = found || entry.Name == container
	}
	if !found {
		return 125, errors.New("select a current pod container")
	}
	request := client.CoreV1().RESTClient().Post().Resource("pods").Namespace(msg.Namespace).Name(msg.Pod).SubResource("exec").VersionedParams(&v1.PodExecOptions{Container: container, Command: []string{"/bin/" + msg.Shell}, Stdin: true, Stdout: true, Stderr: true, TTY: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, request.URL())
	if err != nil {
		return 125, err
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	readyChannel := make(chan struct{})
	var once sync.Once
	ready := func() { once.Do(func() { fmt.Fprintln(os.Stderr, "ORBWIN_SESSION_READY"); close(readyChannel) }) }
	sizes := &podSizes{ctx: ctx, sizes: make(chan remotecommand.TerminalSize, 1), ready: ready}
	sizes.sizes <- remotecommand.TerminalSize{Width: uint16(msg.Cols), Height: uint16(msg.Rows)}
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	finished := make(chan error, 1)
	go func() {
		finished <- executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: input, Stdout: readyOutput{Writer: output, ready: ready}, Tty: true, TerminalSizeQueue: sizes})
	}()
	go func() {
		select {
		case <-readyChannel:
		case <-time.After(15 * time.Second):
			cancel()
		case <-ctx.Done():
		}
	}()
	commands := make(chan error, 1)
	go func() {
		for scanner.Scan() {
			next, err := decode(scanner.Bytes())
			if err != nil {
				commands <- err
				return
			}
			switch next.Kind {
			case "input":
				if len(next.Data) > 8192 {
					commands <- errors.New("input exceeds 8 KiB")
					return
				}
				if err := writeFull(writer, next.Data); err != nil {
					commands <- err
					return
				}
			case "resize":
				if err := dimensions(next.Cols, next.Rows); err != nil {
					commands <- err
					return
				}
				size := remotecommand.TerminalSize{Width: uint16(next.Cols), Height: uint16(next.Rows)}
				select {
				case sizes.sizes <- size:
				default:
					select {
					case <-sizes.sizes:
					default:
					}
					select {
					case sizes.sizes <- size:
					case <-ctx.Done():
						return
					}
				}
			case "close":
				commands <- nil
				return
			default:
				commands <- errors.New("unsupported session message")
				return
			}
		}
		commands <- scanner.Err()
	}()
	select {
	case err := <-finished:
		if err == nil {
			return 0, nil
		}
		var exit utilexec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitStatus(), nil
		}
		return 125, err
	case err := <-commands:
		return 125, err
	case <-ctx.Done():
		return 125, errors.New("pod terminal handshake timed out or session reached its time limit")
	}
}
