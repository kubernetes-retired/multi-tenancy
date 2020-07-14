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

package multi_tenancy

import (
	. "github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/test/e2e/framework"
	e2ecv "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/test/e2e/framework/clusterversion"
)

var _ = SIGDescribe("VirtualCluster", func() {
	f := framework.NewDefaultFramework("virtualcluster")
	var (
		ns       string
		vcClient *framework.VCClient
		cv       *v1alpha1.ClusterVersion
		err      error
	)

	BeforeEach(func() {
		vcClient = f.VCClient()
		ns = f.Namespace.Name

		By("Creating a ClusterVersion " + ns)
		cv, err = e2ecv.CreateDefaultClusterVersion(f.VCClientSet, ns)
		framework.ExpectNoError(err, "Error Creating ClusterVersion")
	})

	AfterEach(func() {
		By("Deleting ClusterVersion " + ns)
		e2ecv.DeleteCV(f.VCClientSet, cv)
	})

	framework.VCDescribe("VirtualCluster LifeCycle", func() {
		It("should be created and removed", func() {
			name := "create-remove-" + framework.RandomSuffix()
			vc := &v1alpha1.VirtualCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: v1alpha1.VirtualClusterSpec{
					ClusterDomain:      "cluster.local",
					ClusterVersionName: cv.GetName(),
					PKIExpireDays:      365,
				},
			}

			By("creating the virtualcluster " + vc.Name)
			vc = vcClient.CreateSync(vc)

			By("check if tenant master is healthy")
			kubecfgBytes, err := conversion.GetKubeConfigOfVC(vcClient.Interface.CoreV1(), vc)
			framework.ExpectNoError(err, "failed to get kubeconfig of vc")
			clusterRestConfig, err := clientcmd.RESTConfigFromKubeConfig(kubecfgBytes)
			framework.ExpectNoError(err, "failed to parse kubeconfig")
			_, err = clientset.NewForConfig(clusterRestConfig)
			framework.ExpectNoError(err, "failed to create clientset from rest config")
			// TODO: we need convert tenant apiserver host for nodeport typed tenant.

			By("deleting the virtualcluster " + vc.Name)
			vcClient.DeleteSync(vc.Name, nil)
		})
	})
})
