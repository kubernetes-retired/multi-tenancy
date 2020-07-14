/*
Copyright 2020 The Kubernetes Authors.
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

package framework

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"time"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/kubernetes/test/e2e/framework/ginkgowrapper"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
)

const (
	// Poll is how often to Poll pods, nodes and claims.
	Poll = 2 * time.Second
)

// RandomSuffix provides a random string to append to pods,services,rcs.
func RandomSuffix() string {
	return strconv.Itoa(rand.Intn(10000))
}

// ExpectNoError checks if "err" is set
func ExpectNoError(err error, explain ...interface{}) {
	ExpectNoErrorWithOffset(1, err, explain...)
}

// ExpectNoErrorWithOffset checks if "err" is set, and if so, fails assertion while logging the error at "offset" levels above its caller
// (for example, for call chain f -> g -> ExpectNoErrorWithOffset(1, ...) error would be logged for "f").
func ExpectNoErrorWithOffset(offset int, err error, explain ...interface{}) {
	if err != nil {
		Logf("Unexpected error occurred: %v", err)
	}
	gomega.ExpectWithOffset(1+offset, err).NotTo(gomega.HaveOccurred(), explain...)
}

// ExpectNoErrorWithRetries checks if "err" is set with retries
func ExpectNoErrorWithRetries(fn func() error, maxRetries int, explain ...interface{}) {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return
		}
		Logf("(Attempt %d of %d) Unexpected error occurred: %v", i+1, maxRetries, err)
	}
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred(), explain...)
}

// byFirstTimestamp sorts a slice of events by first timestamp, using their involvedObject's name as a tie breaker.
type byFirstTimestamp []v1.Event

func (o byFirstTimestamp) Len() int      { return len(o) }
func (o byFirstTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o byFirstTimestamp) Less(i, j int) bool {
	if o[i].FirstTimestamp.Equal(&o[j].FirstTimestamp) {
		return o[i].InvolvedObject.Name < o[j].InvolvedObject.Name
	}
	return o[i].FirstTimestamp.Before(&o[j].FirstTimestamp)
}

// EventsLister is a func that lists events.
type EventsLister func(opts metav1.ListOptions, ns string) (*v1.EventList, error)

// DumpEventsInNamespace dumps events in the given namespace.
func DumpEventsInNamespace(eventsLister EventsLister, namespace string) {
	ginkgo.By(fmt.Sprintf("Collecting events from namespace %q.", namespace))
	events, err := eventsLister(metav1.ListOptions{}, namespace)
	ExpectNoError(err, "failed to list events in namespace %q", namespace)

	ginkgo.By(fmt.Sprintf("Found %d events.", len(events.Items)))
	// Sort events by their first timestamp
	sortedEvents := events.Items
	if len(sortedEvents) > 1 {
		sort.Sort(byFirstTimestamp(sortedEvents))
	}
	for _, e := range sortedEvents {
		Logf("At %v - event for %v: %v %v: %v", e.FirstTimestamp, e.InvolvedObject.Name, e.Source, e.Reason, e.Message)
	}
	// Note that we don't wait for any Cleanup to propagate, which means
	// that if you delete a bunch of pods right before ending your test,
	// you may or may not see the killing/deletion/Cleanup events.
}

// DumpAllNamespaceInfo dumps events, pods and nodes information in the given namespace.
func DumpAllNamespaceInfo(c clientset.Interface, namespace string) {
	DumpEventsInNamespace(func(opts metav1.ListOptions, ns string) (*v1.EventList, error) {
		return c.CoreV1().Events(ns).List(opts)
	}, namespace)

	e2epod.DumpAllPodInfoForNamespace(c, namespace)
}

// LoadConfig returns a config for a rest client.
func LoadConfig() (*restclient.Config, error) {
	c, err := RestclientConfig(TestContext.KubeContext)
	if err != nil {
		if TestContext.KubeConfig == "" {
			return restclient.InClusterConfig()
		}
		return nil, err
	}

	return clientcmd.NewDefaultClientConfig(*c, &clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: TestContext.Host}}).ClientConfig()
}

// RestclientConfig returns a config holds the information needed to build connection to kubernetes clusters.
func RestclientConfig(kubeContext string) (*clientcmdapi.Config, error) {
	Logf(">>> kubeConfig: %s", TestContext.KubeConfig)
	if TestContext.KubeConfig == "" {
		return nil, fmt.Errorf("KubeConfig must be specified to load client config")
	}
	c, err := clientcmd.LoadFromFile(TestContext.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("error loading KubeConfig: %v", err.Error())
	}
	if kubeContext != "" {
		Logf(">>> kubeContext: %s", kubeContext)
		c.CurrentContext = kubeContext
	}
	return c, nil
}

// RunID is a unique identifier of the e2e run.
// Beware that this ID is not the same for all tests in the e2e run, because each Ginkgo node creates it separately.
var RunID = uuid.NewUUID()

func nowStamp() string {
	return time.Now().Format(time.StampMilli)
}

func log(level string, format string, args ...interface{}) {
	fmt.Fprintf(ginkgo.GinkgoWriter, nowStamp()+": "+level+": "+format+"\n", args...)
}

// Logf logs the info.
func Logf(format string, args ...interface{}) {
	log("INFO", format, args...)
}

// Failf logs the fail info.
func Failf(format string, args ...interface{}) {
	FailfWithOffset(1, format, args...)
}

// FailfWithOffset calls "Fail" and logs the error at "offset" levels above its caller
// (for example, for call chain f -> g -> FailfWithOffset(1, ...) error would be logged for "f").
func FailfWithOffset(offset int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log("FAIL", msg)
	ginkgowrapper.Fail(nowStamp()+": "+msg, 1+offset)
}

// Fail is a replacement for ginkgo.Fail which logs the problem as it occurs
// and then calls ginkgowrapper.Fail.
func Fail(msg string, callerSkip ...int) {
	skip := 1
	if len(callerSkip) > 0 {
		skip += callerSkip[0]
	}
	log("FAIL", msg)
	ginkgowrapper.Fail(nowStamp()+": "+msg, skip)
}

// findAvailableNamespaceName random namespace name starting with baseName.
func findAvailableNamespaceName(baseName string, c clientset.Interface) (string, error) {
	var name string
	err := wait.PollImmediate(Poll, 30*time.Second, func() (bool, error) {
		name = fmt.Sprintf("%v-%v", baseName, RandomSuffix())
		_, err := c.CoreV1().Namespaces().Get(name, metav1.GetOptions{})
		if err == nil {
			// Already taken
			return false, nil
		}
		if apierrs.IsNotFound(err) {
			return true, nil
		}
		Logf("Unexpected error while getting namespace: %v", err)
		return false, nil
	})
	return name, err
}

// CreateTestingNS should be used by every test, note that we append a common prefix to the provided test name.
// Please see NewFramework instead of using this directly.
func CreateTestingNS(baseName string, c clientset.Interface, labels map[string]string) (*v1.Namespace, error) {
	if labels == nil {
		labels = map[string]string{}
	}
	labels["e2e-run"] = string(RunID)

	// We don't use ObjectMeta.GenerateName feature, as in case of API call
	// failure we don't know whether the namespace was created and what is its
	// name.
	name, err := findAvailableNamespaceName(baseName, c)
	if err != nil {
		return nil, err
	}

	namespaceObj := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "",
			Labels:    labels,
		},
		Status: v1.NamespaceStatus{},
	}
	// Be robust about making the namespace creation call.
	var got *v1.Namespace
	if err := wait.PollImmediate(Poll, 30*time.Second, func() (bool, error) {
		var err error
		got, err = c.CoreV1().Namespaces().Create(namespaceObj)
		if err != nil {
			Logf("Unexpected error while creating namespace: %v", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, err
	}

	return got, nil
}
