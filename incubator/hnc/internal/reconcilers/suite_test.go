/*

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

package reconcilers_test

import (
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// +kubebuilder:scaffold:imports

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/reconcilers"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg        *rest.Config
	k8sClient  client.Client
	testEnv    *envtest.Environment
	k8sManager ctrl.Manager
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	// Prow machines appear to be very overloaded. While 2s seems to work just fine on a workstation
	// (1s is usually not enough), we've seen some errors on Prow that can only really be attributed
	// to needing more time. So let's set this to 4s for now.
	// - aludwin, Oct 2020
	SetDefaultEventuallyTimeout(time.Second * 4)
	RunSpecsWithDefaultAndCustomReporters(t,
		"Reconciler Suite",
		[]Reporter{printer.NewlineReporter{}})
}

// All tests in the reconcilers_test package are in one suite. As a result, they
// share the same test environment (e.g., same api server).
var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = api.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = corev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = apiextensions.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	// CF: https://github.com/microsoft/azure-databricks-operator/blob/0f722a710fea06b86ecdccd9455336ca712bf775/controllers/suite_test.go

	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())
	err = reconcilers.Create(k8sManager, forest.NewForest(), 100, true)
	Expect(err).ToNot(HaveOccurred())

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
