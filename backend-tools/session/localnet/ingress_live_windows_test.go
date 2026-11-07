//go:build windows

package localnet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	core "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Opt-in experiment against the recorded owned local cluster. Only newly created
// UUID-named namespace resources are changed; no ingress/controller installation,
// workload image, server-side TLS secret, host DNS or trust mutation is involved.
func qualifyExistingIngress(t *testing.T, ctx context.Context, relayIP, plainPort, securePort, session, name, directory string, httpClient *http.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	boundedClient := *httpClient
	boundedClient.Timeout = 2 * time.Second
	httpClient = &boundedClient
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "AppData/Local/OrbWinData/home/.rdorbwin/kube-client.config")
	raw, err := clientcmd.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, auth := range raw.AuthInfos {
		if auth.Exec != nil || auth.AuthProvider != nil {
			t.Fatal("credential plugin refused")
		}
	}
	for _, cluster := range raw.Clusters {
		server, e := url.Parse(cluster.Server)
		if e != nil || server.Scheme != "https" || server.User != nil || net.ParseIP(server.Hostname()) == nil || !net.ParseIP(server.Hostname()).IsLoopback() || cluster.InsecureSkipTLSVerify || cluster.ProxyURL != "" {
			t.Fatal("owned verified loopback kubeconfig required")
		}
	}
	config, err := clientcmd.NewDefaultClientConfig(*raw, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	config.Proxy = nil
	config.Timeout = 5 * time.Second
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := dynamic.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	system, err := client.CoreV1().Namespaces().Get(ctx, "kube-system", meta.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	controller, err := client.AppsV1().Deployments("kube-system").Get(ctx, "traefik", meta.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(controller.Spec.Template.Spec.Containers) != 1 || controller.Spec.Template.Spec.Containers[0].Image != "rancher/mirrored-library-traefik:3.6.10" {
		t.Fatal("unqualified ingress controller image")
	}
	before, _ := json.Marshal(controller.Spec)
	beforeSHA := sha256.Sum256(before)
	nodes, err := client.CoreV1().Nodes().List(ctx, meta.ListOptions{Limit: 2})
	if err != nil || len(nodes.Items) != 1 {
		t.Fatal("one owned node required", err)
	}
	nodeIP := ""
	for _, address := range nodes.Items[0].Status.Addresses {
		if address.Type == core.NodeInternalIP && net.ParseIP(address.Address).To4() != nil {
			nodeIP = address.Address
		}
	}
	if nodeIP == "" {
		t.Fatal("owned node lacks IPv4 address")
	}
	portHTTP, err := strconv.ParseInt(plainPort, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	portHTTPS, err := strconv.ParseInt(securePort, 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	namespace := "orbwin-p2-ingress-" + session
	labels := map[string]string{"dev.orbwin.qualification": session}
	created, err := client.CoreV1().Namespaces().Create(ctx, &core.Namespace{ObjectMeta: meta.ObjectMeta{Name: namespace, Labels: labels}}, meta.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Persist the exact namespace UID before dependent resources are created.
	intent, err := json.Marshal(map[string]any{"namespace": namespace, "uid": created.UID, "clusterNamespaceUID": system.UID, "relayIP": relayIP, "name": name, "traefikSpecSHA256": hex.EncodeToString(beforeSHA[:])})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		current, e := client.CoreV1().Namespaces().Get(cleanup, namespace, meta.GetOptions{})
		if e != nil {
			t.Error("inspect exact namespace before cleanup", e)
			return
		}
		if current.UID != created.UID || current.Labels["dev.orbwin.qualification"] != session {
			t.Error("qualification namespace ownership changed; not removed")
			return
		}
		if e = client.CoreV1().Namespaces().Delete(cleanup, namespace, meta.DeleteOptions{Preconditions: &meta.Preconditions{UID: &created.UID}}); e != nil {
			t.Error(e)
			return
		}
		for {
			_, e = client.CoreV1().Namespaces().Get(cleanup, namespace, meta.GetOptions{})
			if apierrors.IsNotFound(e) {
				break
			}
			if e != nil {
				t.Error(e)
				return
			}
			select {
			case <-cleanup.Done():
				t.Error("namespace cleanup still pending", cleanup.Err())
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	file, err := os.OpenFile(filepath.Join(directory, "ingress-intent.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write(intent)
	if err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil || closed != nil {
		t.Fatal(err, closed)
	}
	service, err := client.CoreV1().Services(namespace).Create(ctx, &core.Service{ObjectMeta: meta.ObjectMeta{Name: "relay", Namespace: namespace, Labels: labels}, Spec: core.ServiceSpec{Ports: []core.ServicePort{{Name: "http", Protocol: core.ProtocolTCP, Port: 80}, {Name: "https", Protocol: core.ProtocolTCP, Port: 443}}}}, meta.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	httpName, httpsName, protocol, port80, port443, ready := "http", "https", core.ProtocolTCP, int32(portHTTP), int32(portHTTPS), true
	endpoint, err := client.DiscoveryV1().EndpointSlices(namespace).Create(ctx, &discovery.EndpointSlice{ObjectMeta: meta.ObjectMeta{Name: "relay", Namespace: namespace, Labels: map[string]string{"kubernetes.io/service-name": "relay", "endpointslice.kubernetes.io/managed-by": "orbwin-qualification", "dev.orbwin.qualification": session}}, AddressType: discovery.AddressTypeIPv4, Endpoints: []discovery.Endpoint{{Addresses: []string{nodeIP}, Conditions: discovery.EndpointConditions{Ready: &ready}}}, Ports: []discovery.EndpointPort{{Name: &httpName, Protocol: &protocol, Port: &port80}, {Name: &httpsName, Protocol: &protocol, Port: &port443}}}, meta.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{"name": "relay", "namespace": namespace, "labels": map[string]any{"dev.orbwin.qualification": session}}
	httpObject := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "traefik.io/v1alpha1", "kind": "IngressRoute", "metadata": metadata, "spec": map[string]any{"entryPoints": []any{"web"}, "routes": []any{map[string]any{"kind": "Rule", "match": fmt.Sprintf("Host(`%s`)", name), "services": []any{map[string]any{"name": "relay", "port": int64(80)}}}}}}}
	httpRoute, err := resources.Resource(schema.GroupVersionResource{Group: "traefik.io", Version: "v1alpha1", Resource: "ingressroutes"}).Namespace(namespace).Create(ctx, httpObject, meta.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tcpObject := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "traefik.io/v1alpha1", "kind": "IngressRouteTCP", "metadata": metadata, "spec": map[string]any{"entryPoints": []any{"websecure"}, "routes": []any{map[string]any{"match": fmt.Sprintf("HostSNI(`%s`)", name), "services": []any{map[string]any{"name": "relay", "port": int64(443)}}}}, "tls": map[string]any{"passthrough": true}}}}
	tcpRoute, err := resources.Resource(schema.GroupVersionResource{Group: "traefik.io", Version: "v1alpha1", Resource: "ingressroutetcps"}).Namespace(namespace).Create(ctx, tcpObject, meta.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var lastErr error
	for attempt := 0; attempt < 50; attempt++ {
		request, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+name+"/", nil)
		if e != nil {
			t.Fatal(e)
		}
		response, e := httpClient.Do(request)
		if e == nil {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, 1024))
			response.Body.Close()
			if readErr == nil && response.StatusCode == 200 && string(body) == "orbwin-p2-loopback-1792892200" {
				lastErr = nil
				break
			}
			e = fmt.Errorf("workload response %d %q: %v", response.StatusCode, body, readErr)
		}
		lastErr = e
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	if lastErr != nil {
		t.Fatal("shared ingress verified HTTPS failed", lastErr)
	}
	rootPath := filepath.Join(directory, "public-root.pem")
	linuxRoot := "/mnt/" + strings.ToLower(rootPath[:1]) + strings.ReplaceAll(rootPath[2:], "\\", "/")
	command := exec.CommandContext(ctx, "wsl.exe", "-d", "Ubuntu", "--exec", "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "3", "--cacert", linuxRoot, "--resolve", name+":443:127.0.0.1", "https://"+name+"/")
	output, err := command.CombinedOutput()
	if err != nil || string(output) != "orbwin-p2-loopback-1792892200" {
		t.Fatalf("shared ingress Ubuntu verified HTTPS %q %v", output, err)
	}
	response, err := httpClient.Get("http://" + name + "/a%2Fb?q=1")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 308 || response.Header.Get("Location") != "https://"+name+"/a%2Fb?q=1" {
		t.Fatal("shared ingress redirect failed", response.Status, response.Header)
	}
	after, err := client.AppsV1().Deployments("kube-system").Get(ctx, "traefik", meta.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	afterSpec, err := json.Marshal(after.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(afterSpec) != beforeSHA {
		t.Fatal("existing ingress deployment spec changed")
	}
	proof, err := json.MarshalIndent(map[string]any{"namespace": namespace, "namespaceUID": created.UID, "clusterNamespaceUID": system.UID, "serviceUID": service.UID, "endpointSliceUID": endpoint.UID, "httpRouteUID": httpRoute.GetUID(), "tcpRouteUID": tcpRoute.GetUID(), "relayIP": relayIP, "endpointNodeIP": nodeIP, "endpointHTTPPort": portHTTP, "endpointHTTPSPort": portHTTPS, "name": name, "windowsVerifiedHTTPS": true, "ubuntuVerifiedHTTPS": true, "windowsHTTPRedirect": true, "traefikSpecUnchanged": true, "privateKeysInKubernetes": false}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "ingress-proof.json"), proof, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("existing owned Traefik passes only the unique qualification Host/SNI through the relay; Windows and Ubuntu verify application TLS; ingress deployment unchanged")
}
