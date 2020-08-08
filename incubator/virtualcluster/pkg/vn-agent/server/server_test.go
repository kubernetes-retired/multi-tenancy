/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/client-go/tools/remotecommand"
	utiltesting "k8s.io/client-go/util/testing"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	api "k8s.io/kubernetes/pkg/apis/core"
	_ "k8s.io/kubernetes/pkg/apis/core/install"
	statsapi "k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1"
	"k8s.io/kubernetes/pkg/kubelet/cm"
	kubeletserver "k8s.io/kubernetes/pkg/kubelet/server"
	"k8s.io/kubernetes/pkg/kubelet/server/portforward"
	remotecommandserver "k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
	"k8s.io/kubernetes/pkg/kubelet/server/stats"
	"k8s.io/kubernetes/pkg/kubelet/server/streaming"
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/testcerts"
)

const (
	testUID          = "9b01b80f-8fb4-11e4-95ab-4200af06647"
	testContainerID  = "container789"
	testPodSandboxID = "pod0987"
)

func newTenantClient() (*http.Client, error) {
	tenantCert, err := tls.X509KeyPair(testcerts.TenantCert, testcerts.TenantKey)
	if err != nil {
		return nil, errors.Wrap(err, "load kubelet server cert")
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				Certificates:       []tls.Certificate{tenantCert},
			},
		},
	}, nil
}

type serverTestFramework struct {
	serverUnderTest *Server
	kubeletServer   *kubletServerTestFramework
	testHTTPServer  *httptest.Server
}

func (s *serverTestFramework) Close() {
	s.testHTTPServer.Close()
	s.kubeletServer.testHTTPServer.Close()
}

func newServerTest() *serverTestFramework {
	return newServerTestWithDebug(true, false, nil)
}

func newServerTestWithDebug(enableDebugging, redirectContainerStreaming bool, streamingServer streaming.Server) *serverTestFramework {
	fv := &serverTestFramework{}
	fv.kubeletServer = newKubeletServerTestWithDebug(enableDebugging, redirectContainerStreaming, streamingServer)

	// install the kubelet server certificate and start server
	kubeletServerCert, err := tls.X509KeyPair(testcerts.KubeletServerCert, testcerts.KubeletServerKey)
	if err != nil {
		panic(errors.Wrap(err, "load kubelet server cert"))
	}
	fv.kubeletServer.testHTTPServer.TLS = new(tls.Config)
	fv.kubeletServer.testHTTPServer.TLS.Certificates = []tls.Certificate{kubeletServerCert}
	fv.kubeletServer.testHTTPServer.StartTLS()

	kubeletClientCert, err := tls.X509KeyPair(testcerts.KubeletClientCert, testcerts.KubeletClientKey)
	if err != nil {
		panic(errors.Wrap(err, "load kubelet client cert"))
	}

	server, err := NewServer(&config.Config{
		KubeletClientCert: &kubeletClientCert,
		KubeletServerHost: fv.kubeletServer.testHTTPServer.URL,
	})
	if err != nil {
		panic(errors.Wrap(err, "new server"))
	}
	fv.serverUnderTest = server
	fv.testHTTPServer = httptest.NewUnstartedServer(fv.serverUnderTest)

	vnAgentCert, err := tls.X509KeyPair(testcerts.VnAgentCert, testcerts.VnAgentKey)
	if err != nil {
		panic(errors.Wrap(err, "load vn-agent cert"))
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(testcerts.CACert)
	fv.testHTTPServer.TLS = &tls.Config{
		ClientCAs:    certPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{vnAgentCert},
	}
	fv.testHTTPServer.StartTLS()

	return fv
}

type fakeKubelet struct {
	podByNameFunc       func(namespace, name string) (*v1.Pod, bool)
	containerInfoFunc   func(podFullName string, uid types.UID, containerName string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error)
	rawInfoFunc         func(query *cadvisorapi.ContainerInfoRequest) (map[string]*cadvisorapi.ContainerInfo, error)
	machineInfoFunc     func() (*cadvisorapi.MachineInfo, error)
	podsFunc            func() []*v1.Pod
	runningPodsFunc     func() ([]*v1.Pod, error)
	logFunc             func(w http.ResponseWriter, req *http.Request)
	runFunc             func(podFullName string, uid types.UID, containerName string, cmd []string) ([]byte, error)
	getExecCheck        func(string, types.UID, string, []string, remotecommandserver.Options)
	getAttachCheck      func(string, types.UID, string, remotecommandserver.Options)
	getPortForwardCheck func(string, string, types.UID, portforward.V4Options)

	containerLogsFunc func(ctx context.Context, podFullName, containerName string, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) error
	hostnameFunc      func() string
	resyncInterval    time.Duration
	loopEntryTime     time.Time
	plegHealth        bool
	streamingRuntime  streaming.Server
}

func (fk *fakeKubelet) ResyncInterval() time.Duration {
	return fk.resyncInterval
}

func (fk *fakeKubelet) LatestLoopEntryTime() time.Time {
	return fk.loopEntryTime
}

func (fk *fakeKubelet) GetPodByName(namespace, name string) (*v1.Pod, bool) {
	return fk.podByNameFunc(namespace, name)
}

func (fk *fakeKubelet) GetContainerInfo(podFullName string, uid types.UID, containerName string, req *cadvisorapi.ContainerInfoRequest) (*cadvisorapi.ContainerInfo, error) {
	return fk.containerInfoFunc(podFullName, uid, containerName, req)
}

func (fk *fakeKubelet) GetRawContainerInfo(containerName string, req *cadvisorapi.ContainerInfoRequest, subcontainers bool) (map[string]*cadvisorapi.ContainerInfo, error) {
	return fk.rawInfoFunc(req)
}

func (fk *fakeKubelet) GetCachedMachineInfo() (*cadvisorapi.MachineInfo, error) {
	return fk.machineInfoFunc()
}

func (*fakeKubelet) GetVersionInfo() (*cadvisorapi.VersionInfo, error) {
	return &cadvisorapi.VersionInfo{}, nil
}

func (fk *fakeKubelet) GetPods() []*v1.Pod {
	return fk.podsFunc()
}

func (fk *fakeKubelet) GetRunningPods() ([]*v1.Pod, error) {
	return fk.runningPodsFunc()
}

func (fk *fakeKubelet) ServeLogs(w http.ResponseWriter, req *http.Request) {
	fk.logFunc(w, req)
}

func (fk *fakeKubelet) GetKubeletContainerLogs(ctx context.Context, podFullName, containerName string, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) error {
	return fk.containerLogsFunc(ctx, podFullName, containerName, logOptions, stdout, stderr)
}

func (fk *fakeKubelet) GetHostname() string {
	return fk.hostnameFunc()
}

func (fk *fakeKubelet) RunInContainer(podFullName string, uid types.UID, containerName string, cmd []string) ([]byte, error) {
	return fk.runFunc(podFullName, uid, containerName, cmd)
}

type fakeRuntime struct {
	execFunc        func(string, []string, io.Reader, io.WriteCloser, io.WriteCloser, bool, <-chan remotecommand.TerminalSize) error
	attachFunc      func(string, io.Reader, io.WriteCloser, io.WriteCloser, bool, <-chan remotecommand.TerminalSize) error
	portForwardFunc func(string, int32, io.ReadWriteCloser) error
}

func (f *fakeRuntime) Exec(containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return f.execFunc(containerID, cmd, stdin, stdout, stderr, tty, resize)
}

func (f *fakeRuntime) Attach(containerID string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return f.attachFunc(containerID, stdin, stdout, stderr, tty, resize)
}

func (f *fakeRuntime) PortForward(podSandboxID string, port int32, stream io.ReadWriteCloser) error {
	return f.portForwardFunc(podSandboxID, port, stream)
}

type kubeletTestStreamingServer struct {
	streaming.Server
	fakeRuntime    *fakeRuntime
	testHTTPServer *httptest.Server
}

func newTestStreamingServer(streamIdleTimeout time.Duration) (s *kubeletTestStreamingServer, err error) {
	s = &kubeletTestStreamingServer{}
	s.testHTTPServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.ServeHTTP(w, r)
	}))
	defer func() {
		if err != nil {
			s.testHTTPServer.Close()
		}
	}()

	testURL, err := url.Parse(s.testHTTPServer.URL)
	if err != nil {
		return nil, err
	}
	s.fakeRuntime = &fakeRuntime{}
	config := streaming.DefaultConfig
	config.BaseURL = testURL
	if streamIdleTimeout != 0 {
		config.StreamIdleTimeout = streamIdleTimeout
	}
	s.Server, err = streaming.NewServer(config, s.fakeRuntime)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (fk *fakeKubelet) GetExec(podFullName string, podUID types.UID, containerName string, cmd []string, streamOpts remotecommandserver.Options) (*url.URL, error) {
	if fk.getExecCheck != nil {
		fk.getExecCheck(podFullName, podUID, containerName, cmd, streamOpts)
	}
	// Always use testContainerID
	resp, err := fk.streamingRuntime.GetExec(&runtimeapi.ExecRequest{
		ContainerId: testContainerID,
		Cmd:         cmd,
		Tty:         streamOpts.TTY,
		Stdin:       streamOpts.Stdin,
		Stdout:      streamOpts.Stdout,
		Stderr:      streamOpts.Stderr,
	})
	if err != nil {
		return nil, err
	}
	return url.Parse(resp.GetUrl())
}

func (fk *fakeKubelet) GetAttach(podFullName string, podUID types.UID, containerName string, streamOpts remotecommandserver.Options) (*url.URL, error) {
	if fk.getAttachCheck != nil {
		fk.getAttachCheck(podFullName, podUID, containerName, streamOpts)
	}
	// Always use testContainerID
	resp, err := fk.streamingRuntime.GetAttach(&runtimeapi.AttachRequest{
		ContainerId: testContainerID,
		Tty:         streamOpts.TTY,
		Stdin:       streamOpts.Stdin,
		Stdout:      streamOpts.Stdout,
		Stderr:      streamOpts.Stderr,
	})
	if err != nil {
		return nil, err
	}
	return url.Parse(resp.GetUrl())
}

func (fk *fakeKubelet) GetPortForward(podName, podNamespace string, podUID types.UID, portForwardOpts portforward.V4Options) (*url.URL, error) {
	if fk.getPortForwardCheck != nil {
		fk.getPortForwardCheck(podName, podNamespace, podUID, portForwardOpts)
	}
	// Always use testPodSandboxID
	resp, err := fk.streamingRuntime.GetPortForward(&runtimeapi.PortForwardRequest{
		PodSandboxId: testPodSandboxID,
		Port:         portForwardOpts.Ports,
	})
	if err != nil {
		return nil, err
	}
	return url.Parse(resp.GetUrl())
}

// Unused functions
func (*fakeKubelet) GetNode() (*v1.Node, error)                       { return nil, nil }
func (*fakeKubelet) GetNodeConfig() cm.NodeConfig                     { return cm.NodeConfig{} }
func (*fakeKubelet) GetPodCgroupRoot() string                         { return "" }
func (*fakeKubelet) GetPodByCgroupfs(cgroupfs string) (*v1.Pod, bool) { return nil, false }
func (fk *fakeKubelet) ListVolumesForPod(podUID types.UID) (map[string]volume.Volume, bool) {
	return map[string]volume.Volume{}, true
}

func (*fakeKubelet) RootFsStats() (*statsapi.FsStats, error)    { return nil, nil }
func (*fakeKubelet) ListPodStats() ([]statsapi.PodStats, error) { return nil, nil }
func (*fakeKubelet) ListPodStatsAndUpdateCPUNanoCoreUsage() ([]statsapi.PodStats, error) {
	return nil, nil
}
func (*fakeKubelet) ListPodCPUAndMemoryStats() ([]statsapi.PodStats, error) { return nil, nil }
func (*fakeKubelet) ImageFsStats() (*statsapi.FsStats, error)               { return nil, nil }
func (*fakeKubelet) RlimitStats() (*statsapi.RlimitStats, error)            { return nil, nil }
func (*fakeKubelet) GetCgroupStats(cgroupName string, updateStats bool) (*statsapi.ContainerStats, *statsapi.NetworkStats, error) {
	return nil, nil, nil
}
func (*fakeKubelet) GetCgroupCPUAndMemoryStats(cgroupName string, updateStats bool) (*statsapi.ContainerStats, error) {
	return nil, nil
}

type fakeAuth struct {
	authenticateFunc func(*http.Request) (*authenticator.Response, bool, error)
	attributesFunc   func(user.Info, *http.Request) authorizer.Attributes
	authorizeFunc    func(context.Context, authorizer.Attributes) (authorized authorizer.Decision, reason string, err error)
}

func (f *fakeAuth) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	return f.authenticateFunc(req)
}
func (f *fakeAuth) GetRequestAttributes(u user.Info, req *http.Request) authorizer.Attributes {
	return f.attributesFunc(u, req)
}
func (f *fakeAuth) Authorize(ctx context.Context, a authorizer.Attributes) (authorized authorizer.Decision, reason string, err error) {
	return f.authorizeFunc(ctx, a)
}

type kubletServerTestFramework struct {
	serverUnderTest         *kubeletserver.Server
	fakeKubelet             *fakeKubelet
	fakeAuth                *fakeAuth
	testHTTPServer          *httptest.Server
	fakeRuntime             *fakeRuntime
	testStreamingHTTPServer *httptest.Server
	criHandler              *utiltesting.FakeHandler
}

func newKubeletServerTestWithDebug(enableDebugging, redirectContainerStreaming bool, streamingServer streaming.Server) *kubletServerTestFramework {
	fw := &kubletServerTestFramework{}
	fw.fakeKubelet = &fakeKubelet{
		hostnameFunc: func() string {
			return "127.0.0.1"
		},
		podByNameFunc: func(namespace, name string) (*v1.Pod, bool) {
			return &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      name,
					UID:       testUID,
				},
			}, true
		},
		plegHealth:       true,
		streamingRuntime: streamingServer,
	}
	fw.fakeAuth = &fakeAuth{
		authenticateFunc: func(req *http.Request) (*authenticator.Response, bool, error) {
			return &authenticator.Response{User: &user.DefaultInfo{Name: "test"}}, true, nil
		},
		attributesFunc: func(u user.Info, req *http.Request) authorizer.Attributes {
			return &authorizer.AttributesRecord{User: u}
		},
		authorizeFunc: func(_ context.Context, a authorizer.Attributes) (decision authorizer.Decision, reason string, err error) {
			return authorizer.DecisionAllow, "", nil
		},
	}
	fw.criHandler = &utiltesting.FakeHandler{
		StatusCode: http.StatusOK,
	}
	server := kubeletserver.NewServer(
		fw.fakeKubelet,
		stats.NewResourceAnalyzer(fw.fakeKubelet, time.Minute),
		fw.fakeAuth,
		true,
		enableDebugging,
		false,
		redirectContainerStreaming,
		fw.criHandler)
	fw.serverUnderTest = &server
	fw.testHTTPServer = httptest.NewUnstartedServer(fw.serverUnderTest)
	return fw
}

// A helper function to return the correct pod name.
func getPodName(name, namespace string) string {
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	return name + "_" + namespace
}

// getEffectiveNamespace translate the tenant namespace name to super master namespace name.
func getEffectiveNamespace(tenantName, namespace string) string {
	return tenantName + "-" + namespace
}

func TestServeLogs(t *testing.T) {
	fv := newServerTest()
	defer fv.Close()

	content := string(`<pre><a href="kubelet.log">kubelet.log</a><a href="google.log">google.log</a></pre>`)

	fv.kubeletServer.fakeKubelet.logFunc = func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Add("Content-Type", "text/html")
		w.Write([]byte(content))
	}

	tenantClient, err := newTenantClient()
	if err != nil {
		t.Fatalf("Got tenant client: %v", err)
	}
	resp, err := tenantClient.Get(fv.testHTTPServer.URL + "/logs/")
	if err != nil {
		t.Fatalf("Got Error GETing: %v", err)
	}
	defer resp.Body.Close()

	body, err := httputil.DumpResponse(resp, true)
	if err != nil {
		t.Errorf("Cannot copy resp: %#v", err)
	}
	result := string(body)
	if !strings.Contains(result, "kubelet.log") || !strings.Contains(result, "google.log") {
		t.Errorf("Received wrong data: %s", result)
	}
}

func TestServeRunInContainer(t *testing.T) {
	fv := newServerTest()
	defer fv.Close()
	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodNamespace := getEffectiveNamespace(testcerts.TenantName, podNamespace)
	expectedPodName := getPodName(podName, expectedPodNamespace)
	expectedContainerName := "baz"
	expectedCommand := "ls -a"
	fv.kubeletServer.fakeKubelet.runFunc = func(podFullName string, uid types.UID, containerName string, cmd []string) ([]byte, error) {
		if podFullName != expectedPodName {
			t.Errorf("expected %s, got %s", expectedPodName, podFullName)
		}
		if containerName != expectedContainerName {
			t.Errorf("expected %s, got %s", expectedContainerName, containerName)
		}
		if strings.Join(cmd, " ") != expectedCommand {
			t.Errorf("expected: %s, got %v", expectedCommand, cmd)
		}

		return []byte(output), nil
	}

	tenantClient, err := newTenantClient()
	if err != nil {
		t.Fatalf("Got tenant client: %v", err)
	}

	resp, err := tenantClient.Post(fv.testHTTPServer.URL+"/run/"+podNamespace+"/"+podName+"/"+expectedContainerName+"?cmd=ls%20-a", "", nil)
	if err != nil {
		t.Fatalf("Got error POSTing: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// copying the response body did not work
		t.Errorf("Cannot copy resp: %#v", err)
	}
	result := string(body)
	if result != output {
		t.Errorf("expected %s, got %s", output, result)
	}
}

func TestServeRunInContainerWithUID(t *testing.T) {
	// TODO: support UID naming translate
}

func setPodByNameFunc(fv *serverTestFramework, namespace, pod, container string) {
	fv.kubeletServer.fakeKubelet.podByNameFunc = func(namespace, name string) (*v1.Pod, bool) {
		return &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      pod,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: container,
					},
				},
			},
		}, true
	}
}

func setGetContainerLogsFunc(fv *serverTestFramework, t *testing.T, expectedPodName, expectedContainerName string, expectedLogOptions *v1.PodLogOptions, output string) {
	fv.kubeletServer.fakeKubelet.containerLogsFunc = func(_ context.Context, podFullName, containerName string, logOptions *v1.PodLogOptions, stdout, stderr io.Writer) error {
		if podFullName != expectedPodName {
			t.Errorf("expected %s, got %s", expectedPodName, podFullName)
		}
		if containerName != expectedContainerName {
			t.Errorf("expected %s, got %s", expectedContainerName, containerName)
		}
		if !reflect.DeepEqual(expectedLogOptions, logOptions) {
			t.Errorf("expected %#v, got %#v", expectedLogOptions, logOptions)
		}

		io.WriteString(stdout, output)
		return nil
	}
}

func TestContainerLogs(t *testing.T) {
	fw := newServerTest()
	defer fw.Close()

	tenantClient, err := newTenantClient()
	if err != nil {
		t.Fatalf("Got tenant client: %v", err)
	}

	tests := map[string]struct {
		query         string
		containerName string
		podLogOption  *v1.PodLogOptions
	}{
		"without tail":        {"", "baz", &v1.PodLogOptions{}},
		"with tail":           {"?tailLines=5", "baz", &v1.PodLogOptions{TailLines: pointer.Int64Ptr(5)}},
		"with legacy tail":    {"?tail=5", "baz", &v1.PodLogOptions{TailLines: pointer.Int64Ptr(5)}},
		"with tail all":       {"?tail=all", "baz", &v1.PodLogOptions{}},
		"with follow":         {"?follow=1", "baz", &v1.PodLogOptions{Follow: true}},
		"with container name": {"", "blah", &v1.PodLogOptions{}},
	}

	for desc, test := range tests {
		t.Run(desc, func(t *testing.T) {
			output := "foo bar"
			podNamespace := "other"
			podName := "foo"
			expectedPodNamespace := getEffectiveNamespace(testcerts.TenantName, podNamespace)
			expectedPodName := getPodName(podName, expectedPodNamespace)
			expectedContainerName := test.containerName
			setPodByNameFunc(fw, expectedPodNamespace, podName, expectedContainerName)
			setGetContainerLogsFunc(fw, t, expectedPodName, expectedContainerName, test.podLogOption, output)

			resp, err := tenantClient.Get(fw.testHTTPServer.URL + "/containerLogs/" + podNamespace + "/" + podName + "/" + expectedContainerName + test.query)
			if err != nil {
				t.Fatalf("Got error GETing: %v", err)
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Error reading container logs: %v", err)
			}
			result := string(body)
			if result != output {
				t.Fatalf("Expected: '%v', got: '%v'", output, result)
			}
		})
	}
}

func TestContainerLogsWithInvalidTail(t *testing.T) {
	fw := newServerTest()
	defer fw.Close()

	output := "foo bar"
	podNamespace := "other"
	podName := "foo"
	expectedPodNamespace := getEffectiveNamespace(testcerts.TenantName, podNamespace)
	expectedPodName := getPodName(podName, expectedPodNamespace)
	expectedContainerName := "baz"
	setPodByNameFunc(fw, expectedPodNamespace, podName, expectedContainerName)
	setGetContainerLogsFunc(fw, t, expectedPodName, expectedContainerName, &v1.PodLogOptions{}, output)

	tenantClient, err := newTenantClient()
	if err != nil {
		t.Fatalf("Got tenant client: %v", err)
	}

	resp, err := tenantClient.Get(fw.testHTTPServer.URL + "/containerLogs/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?tail=-1")
	if err != nil {
		t.Errorf("Got error GETing: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("Unexpected non-error reading container logs: %#v", resp)
	}
}

func makeReq(t *testing.T, method, url, clientProtocol string) *http.Request {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("error creating request: %v", err)
	}
	req.Header.Set("Content-Type", "")
	req.Header.Add("X-Stream-Protocol-Version", clientProtocol)
	return req
}

func testExecAttach(t *testing.T, verb string) {
	tests := map[string]struct {
		stdin              bool
		stdout             bool
		stderr             bool
		tty                bool
		responseStatusCode int
		uid                bool
		redirect           bool
	}{
		"no input or output":           {responseStatusCode: http.StatusBadRequest},
		"stdin":                        {stdin: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdout":                       {stdout: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stderr":                       {stderr: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdout and stderr":            {stdout: true, stderr: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdin stdout and stderr":      {stdin: true, stdout: true, stderr: true, responseStatusCode: http.StatusSwitchingProtocols},
		"stdin stdout stderr with uid": {stdin: true, stdout: true, stderr: true, responseStatusCode: http.StatusSwitchingProtocols, uid: true},
		"stdout with redirect":         {stdout: true, responseStatusCode: http.StatusFound, redirect: true},
	}

	podNamespace := "other"
	podName := "foo"
	expectedNamespace := getEffectiveNamespace(testcerts.TenantName, podNamespace)
	expectedPodName := getPodName(podName, expectedNamespace)
	expectedContainerName := "baz"
	expectedCommand := "ls -a"
	expectedStdin := "stdin"
	expectedStdout := "stdout"
	expectedStderr := "stderr"

	for desc, test := range tests {
		test := test
		t.Run(desc, func(t *testing.T) {
			ss, err := newTestStreamingServer(0)
			require.NoError(t, err)
			defer ss.testHTTPServer.Close()
			fv := newServerTestWithDebug(true, test.redirect, ss)
			defer fv.Close()

			done := make(chan struct{})
			clientStdoutReadDone := make(chan struct{})
			clientStderrReadDone := make(chan struct{})
			execInvoked := false
			attachInvoked := false

			checkStream := func(podFullName string, uid types.UID, containerName string, streamOpts remotecommandserver.Options) {
				assert.Equal(t, expectedPodName, podFullName, "podFullName")
				if test.uid {
					assert.Equal(t, testUID, string(uid), "uid")
				}
				assert.Equal(t, expectedContainerName, containerName, "containerName")
				assert.Equal(t, test.stdin, streamOpts.Stdin, "stdin")
				assert.Equal(t, test.stdout, streamOpts.Stdout, "stdout")
				assert.Equal(t, test.tty, streamOpts.TTY, "tty")
				assert.Equal(t, !test.tty && test.stderr, streamOpts.Stderr, "stderr")
			}

			fv.kubeletServer.fakeKubelet.getExecCheck = func(podFullName string, uid types.UID, containerName string, cmd []string, streamOpts remotecommandserver.Options) {
				execInvoked = true
				assert.Equal(t, expectedCommand, strings.Join(cmd, " "), "cmd")
				checkStream(podFullName, uid, containerName, streamOpts)
			}

			fv.kubeletServer.fakeKubelet.getAttachCheck = func(podFullName string, uid types.UID, containerName string, streamOpts remotecommandserver.Options) {
				attachInvoked = true
				checkStream(podFullName, uid, containerName, streamOpts)
			}

			testStream := func(containerID string, in io.Reader, out, stderr io.WriteCloser, tty bool, done chan struct{}) error {
				close(done)
				assert.Equal(t, testContainerID, containerID, "containerID")
				assert.Equal(t, test.tty, tty, "tty")
				require.Equal(t, test.stdin, in != nil, "in")
				require.Equal(t, test.stdout, out != nil, "out")
				require.Equal(t, !test.tty && test.stderr, stderr != nil, "err")

				if test.stdin {
					b := make([]byte, 10)
					n, err := in.Read(b)
					assert.NoError(t, err, "reading from stdin")
					assert.Equal(t, expectedStdin, string(b[0:n]), "content from stdin")
				}

				if test.stdout {
					_, err := out.Write([]byte(expectedStdout))
					assert.NoError(t, err, "writing to stdout")
					out.Close()
					<-clientStdoutReadDone
				}

				if !test.tty && test.stderr {
					_, err := stderr.Write([]byte(expectedStderr))
					assert.NoError(t, err, "writing to stderr")
					stderr.Close()
					<-clientStderrReadDone
				}
				return nil
			}

			ss.fakeRuntime.execFunc = func(containerID string, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
				assert.Equal(t, expectedCommand, strings.Join(cmd, " "), "cmd")
				return testStream(containerID, stdin, stdout, stderr, tty, done)
			}

			ss.fakeRuntime.attachFunc = func(containerID string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
				return testStream(containerID, stdin, stdout, stderr, tty, done)
			}

			var url string
			if test.uid {
				url = fv.testHTTPServer.URL + "/" + verb + "/" + podNamespace + "/" + podName + "/" + testUID + "/" + expectedContainerName + "?ignore=1"
			} else {
				url = fv.testHTTPServer.URL + "/" + verb + "/" + podNamespace + "/" + podName + "/" + expectedContainerName + "?ignore=1"
			}
			if verb == "exec" {
				url += "&command=ls&command=-a"
			}
			if test.stdin {
				url += "&" + api.ExecStdinParam + "=1"
			}
			if test.stdout {
				url += "&" + api.ExecStdoutParam + "=1"
			}
			if test.stderr && !test.tty {
				url += "&" + api.ExecStderrParam + "=1"
			}
			if test.tty {
				url += "&" + api.ExecTTYParam + "=1"
			}

			fmt.Println(url)
			var (
				resp                *http.Response
				upgradeRoundTripper httpstream.UpgradeRoundTripper
				c                   *http.Client
			)
			tenantCert, err := tls.X509KeyPair(testcerts.TenantCert, testcerts.TenantKey)
			require.NoError(t, err)
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
				Certificates:       []tls.Certificate{tenantCert},
			}

			if test.redirect {
				c = &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: tlsConfig,
					},
					// Don't follow redirects, since we want to inspect the redirect response.
					CheckRedirect: func(*http.Request, []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}
			} else {
				upgradeRoundTripper = spdy.NewRoundTripper(tlsConfig, true, true)
				c = &http.Client{Transport: upgradeRoundTripper}
			}

			resp, err = c.Do(makeReq(t, "POST", url, "v4.channel.k8s.io"))
			require.NoError(t, err, "POSTing")
			defer resp.Body.Close()

			_, err = ioutil.ReadAll(resp.Body)
			assert.NoError(t, err, "reading response body")

			require.Equal(t, test.responseStatusCode, resp.StatusCode, "response status")
			if test.responseStatusCode != http.StatusSwitchingProtocols {
				return
			}

			conn, err := upgradeRoundTripper.NewConnection(resp)
			require.NoError(t, err, "creating streaming connection")
			defer conn.Close()

			h := http.Header{}
			h.Set(api.StreamType, api.StreamTypeError)
			_, err = conn.CreateStream(h)
			require.NoError(t, err, "creating error stream")

			if test.stdin {
				h.Set(api.StreamType, api.StreamTypeStdin)
				stream, err := conn.CreateStream(h)
				require.NoError(t, err, "creating stdin stream")
				_, err = stream.Write([]byte(expectedStdin))
				require.NoError(t, err, "writing to stdin stream")
			}

			var stdoutStream httpstream.Stream
			if test.stdout {
				h.Set(api.StreamType, api.StreamTypeStdout)
				stdoutStream, err = conn.CreateStream(h)
				require.NoError(t, err, "creating stdout stream")
			}

			var stderrStream httpstream.Stream
			if test.stderr && !test.tty {
				h.Set(api.StreamType, api.StreamTypeStderr)
				stderrStream, err = conn.CreateStream(h)
				require.NoError(t, err, "creating stderr stream")
			}

			if test.stdout {
				output := make([]byte, 10)
				n, err := stdoutStream.Read(output)
				close(clientStdoutReadDone)
				assert.NoError(t, err, "reading from stdout stream")
				assert.Equal(t, expectedStdout, string(output[0:n]), "stdout")
			}

			if test.stderr && !test.tty {
				output := make([]byte, 10)
				n, err := stderrStream.Read(output)
				close(clientStderrReadDone)
				assert.NoError(t, err, "reading from stderr stream")
				assert.Equal(t, expectedStderr, string(output[0:n]), "stderr")
			}

			// wait for the server to finish before checking if the attach/exec funcs were invoked
			<-done

			if verb == "exec" {
				assert.True(t, execInvoked, "exec should be invoked")
				assert.False(t, attachInvoked, "attach should not be invoked")
			} else {
				assert.True(t, attachInvoked, "attach should be invoked")
				assert.False(t, execInvoked, "exec should not be invoked")
			}
		})
	}
}

func TestServeExecInContainer(t *testing.T) {
	testExecAttach(t, "exec")
}

func TestServeAttachContainer(t *testing.T) {
	testExecAttach(t, "attach")
}

func TestServePortForwardIdleTimeout(t *testing.T) {
	ss, err := newTestStreamingServer(100 * time.Millisecond)
	require.NoError(t, err)
	defer ss.testHTTPServer.Close()
	fv := newServerTestWithDebug(true, false, ss)
	defer fv.Close()

	podNamespace := "other"
	podName := "foo"

	url := fv.testHTTPServer.URL + "/portForward/" + podNamespace + "/" + podName

	tenantCert, err := tls.X509KeyPair(testcerts.TenantCert, testcerts.TenantKey)
	require.NoError(t, err)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{tenantCert},
	}

	upgradeRoundTripper := spdy.NewRoundTripper(tlsConfig, true, true)
	c := &http.Client{Transport: upgradeRoundTripper}

	resp, err := c.Do(makeReq(t, "POST", url, "portforward.k8s.io"))
	if err != nil {
		t.Fatalf("Got error POSTing: %v", err)
	}
	defer resp.Body.Close()

	conn, err := upgradeRoundTripper.NewConnection(resp)
	if err != nil {
		t.Fatalf("Unexpected error creating streaming connection: %s", err)
	}
	if conn == nil {
		t.Fatal("Unexpected nil connection")
	}
	defer conn.Close()

	<-conn.CloseChan()
}

func TestServePortForward(t *testing.T) {
	tests := map[string]struct {
		port          string
		uid           bool
		clientData    string
		containerData string
		redirect      bool
		shouldError   bool
	}{
		"no port":                       {port: "", shouldError: true},
		"none number port":              {port: "abc", shouldError: true},
		"negative port":                 {port: "-1", shouldError: true},
		"too large port":                {port: "65536", shouldError: true},
		"0 port":                        {port: "0", shouldError: true},
		"min port":                      {port: "1", shouldError: false},
		"normal port":                   {port: "8000", shouldError: false},
		"normal port with data forward": {port: "8000", clientData: "client data", containerData: "container data", shouldError: false},
		"max port":                      {port: "65535", shouldError: false},
		"normal port with uid":          {port: "8000", uid: true, shouldError: false},
		"normal port with redirect":     {port: "8000", redirect: true, shouldError: false},
	}

	podNamespace := "other"
	podName := "foo"
	expectedNamespace := getEffectiveNamespace(testcerts.TenantName, podNamespace)

	for desc, test := range tests {
		test := test
		t.Run(desc, func(t *testing.T) {
			ss, err := newTestStreamingServer(0)
			require.NoError(t, err)
			defer ss.testHTTPServer.Close()
			fv := newServerTestWithDebug(true, test.redirect, ss)
			defer fv.Close()

			portForwardFuncDone := make(chan struct{})

			fv.kubeletServer.fakeKubelet.getPortForwardCheck = func(name, namespace string, uid types.UID, opts portforward.V4Options) {
				assert.Equal(t, podName, name, "pod name")
				assert.Equal(t, expectedNamespace, namespace, "pod namespace")
				if test.uid {
					assert.Equal(t, testUID, string(uid), "uid")
				}
			}

			ss.fakeRuntime.portForwardFunc = func(podSandboxID string, port int32, stream io.ReadWriteCloser) error {
				defer close(portForwardFuncDone)
				assert.Equal(t, testPodSandboxID, podSandboxID, "pod sandbox id")
				// The port should be valid if it reaches here.
				testPort, err := strconv.ParseInt(test.port, 10, 32)
				require.NoError(t, err, "parse port")
				assert.Equal(t, int32(testPort), port, "port")

				if test.clientData != "" {
					fromClient := make([]byte, 32)
					n, err := stream.Read(fromClient)
					assert.NoError(t, err, "reading client data")
					assert.Equal(t, test.clientData, string(fromClient[0:n]), "client data")
				}

				if test.containerData != "" {
					_, err := stream.Write([]byte(test.containerData))
					assert.NoError(t, err, "writing container data")
				}

				return nil
			}

			var url string
			if test.uid {
				url = fmt.Sprintf("%s/portForward/%s/%s/%s", fv.testHTTPServer.URL, podNamespace, podName, testUID)
			} else {
				url = fmt.Sprintf("%s/portForward/%s/%s", fv.testHTTPServer.URL, podNamespace, podName)
			}

			var (
				upgradeRoundTripper httpstream.UpgradeRoundTripper
				c                   *http.Client
			)
			tenantCert, err := tls.X509KeyPair(testcerts.TenantCert, testcerts.TenantKey)
			require.NoError(t, err)
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
				Certificates:       []tls.Certificate{tenantCert},
			}

			if test.redirect {
				c = &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: tlsConfig,
					},
				}
				// Don't follow redirects, since we want to inspect the redirect response.
				c.CheckRedirect = func(*http.Request, []*http.Request) error {
					return http.ErrUseLastResponse
				}
			} else {
				upgradeRoundTripper = spdy.NewRoundTripper(tlsConfig, true, true)
				c = &http.Client{Transport: upgradeRoundTripper}
			}

			resp, err := c.Do(makeReq(t, "POST", url, "portforward.k8s.io"))
			require.NoError(t, err, "POSTing")
			defer resp.Body.Close()

			if test.redirect {
				assert.Equal(t, http.StatusFound, resp.StatusCode, "status code")
				return
			}
			assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode, "status code")

			conn, err := upgradeRoundTripper.NewConnection(resp)
			require.NoError(t, err, "creating streaming connection")
			defer conn.Close()

			headers := http.Header{}
			headers.Set("streamType", "error")
			headers.Set("port", test.port)
			_, err = conn.CreateStream(headers)
			assert.Equal(t, test.shouldError, err != nil, "expect error")

			if test.shouldError {
				return
			}

			headers.Set("streamType", "data")
			headers.Set("port", test.port)
			dataStream, err := conn.CreateStream(headers)
			require.NoError(t, err, "create stream")

			if test.clientData != "" {
				_, err := dataStream.Write([]byte(test.clientData))
				assert.NoError(t, err, "writing client data")
			}

			if test.containerData != "" {
				fromContainer := make([]byte, 32)
				n, err := dataStream.Read(fromContainer)
				assert.NoError(t, err, "reading container data")
				assert.Equal(t, test.containerData, string(fromContainer[0:n]), "container data")
			}

			<-portForwardFuncDone
		})
	}
}
