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

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned/scheme"
	kubeutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	netutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/net"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

const (
	APIServerSvcName = "apiserver-svc"

	pollStsPeriodSec  = 2
	pollStsTimeoutSec = 120
)

type CreateOptions struct {
	client     client.Client
	vcclient   vcclient.Interface
	fileName   string
	outputPath string
}

func NewCmdCreate(f Factory) *cobra.Command {
	o := &CreateOptions{}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new VirtualCluster",
		Run: func(cmd *cobra.Command, args []string) {
			CheckErr(o.Complete(f, cmd))
			CheckErr(o.Validate(cmd))
			CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVarP(&o.fileName, "filename", "f", "", "the configuration to apply. in json, yaml or url")
	cmd.Flags().StringVarP(&o.outputPath, "output", "o", "", "path to the kubeconfig that is used to access virtual cluster")

	return cmd
}

func (o *CreateOptions) Complete(f Factory, cmd *cobra.Command) error {
	var err error
	o.vcclient, err = f.VirtualClusterClientSet()
	if err != nil {
		return err
	}

	o.client, err = f.GenericClient()
	if err != nil {
		return err
	}

	return nil
}

func (o *CreateOptions) Validate(cmd *cobra.Command) error {
	if len(o.fileName) == 0 {
		return UsageErrorf(cmd, "--filename,-f should not be empty")
	}
	if len(o.outputPath) == 0 {
		return UsageErrorf(cmd, "--output,-o should not be empty")
	}
	return nil
}

func (o *CreateOptions) Run() error {
	fileBytes, err := readFromFileOrURL(o.fileName)
	if err != nil {
		return errors.Wrapf(err, "read \"%s\"", o.fileName)
	}

	vc := &tenancyv1alpha1.VirtualCluster{}
	codecs := serializer.NewCodecFactory(scheme.Scheme)
	if err = runtime.DecodeInto(codecs.UniversalDecoder(), fileBytes, vc); err != nil {
		return err
	}

	kubecfgBytes, err := createVirtualCluster(o.client, o.vcclient, vc)
	if err != nil {
		return err
	}

	// write tenant kubeconfig to outputPath.
	if err := ioutil.WriteFile(o.outputPath, kubecfgBytes, 0644); err != nil {
		return err
	}

	log.Printf("VirtualCluster %s/%s setup successfully\n", vc.Namespace, vc.Name)

	return nil
}

func createVirtualCluster(cli client.Client, vccli vcclient.Interface, vc *tenancyv1alpha1.VirtualCluster) ([]byte, error) {
	cv, err := vccli.TenancyV1alpha1().ClusterVersions().Get(vc.Spec.ClusterVersionName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "required cluster version not found")
	}

	// fail early, if service type is not supported
	svcType := cv.Spec.APIServer.Service.Spec.Type
	if svcType != v1.ServiceTypeNodePort &&
		svcType != v1.ServiceTypeLoadBalancer &&
		svcType != v1.ServiceTypeClusterIP {
		return nil, fmt.Errorf("unsupported apiserver service type: %s", svcType)
	}

	if vc, err = vccli.TenancyV1alpha1().VirtualClusters(vc.Namespace).Create(vc); err != nil {
		return nil, errors.Wrapf(err, "create virtual cluster")
	}

	ns := conversion.ToClusterKey(vc)

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "etcd", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return nil, fmt.Errorf("cannot find sts/etcd in ns %s: %s", ns, err)
	}
	log.Println("etcd is ready")

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "apiserver", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return nil, fmt.Errorf("cannot find sts/apiserver in ns %s: %s", ns, err)
	}
	log.Println("apiserver is ready")

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "controller-manager", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return nil, fmt.Errorf("cannot find sts/controller-manager in ns %s: %s", ns, err)
	}
	log.Println("controller-manager is ready")

	return genKubeConfig(cli, vc, cv)
}

// getAPISvcPort gets the apiserver service port if not specifed
func getAPISvcPort(svc *v1.Service) (int, error) {
	if len(svc.Spec.Ports) == 0 {
		return 0, errors.New("no port is specified for apiserver service ")
	}
	if svc.Spec.Ports[0].TargetPort.IntValue() != 0 {
		return svc.Spec.Ports[0].TargetPort.IntValue(), nil
	}
	return int(svc.Spec.Ports[0].Port), nil
}

// retryIfNotFound retries to call `f` `retry` times if the returned error
// of `f` is `metav1.StatusReasonNotFound`
func retryIfNotFound(retry, retryPeriod int, f func() error) error {
	for retry >= 0 {
		if err := f(); err != nil {
			if apierrors.IsNotFound(err) && retry > 0 {
				retry--
				<-time.After(time.Duration(retryPeriod) * time.Second)
				continue
			}
			// if other err or having retried too many times
			return err
		}
		// success
		break
	}
	return nil
}

// getVcKubeConfig gets the kubeconfig of the virtual cluster
func getVcKubeConfig(cli client.Client, clusterNamespace, srtName string) ([]byte, error) {
	// kubeconfig of the tenant cluster is stored in meta cluster as a secret
	srt := &corev1.Secret{}
	err := cli.Get(context.TODO(),
		types.NamespacedName{
			Namespace: clusterNamespace,
			Name:      srtName,
		}, srt)
	if err != nil {
		return nil, fmt.Errorf("fail to get %s: %s", srtName, err)
	}
	// get the secret that stores the kubeconfig of the tenant cluster
	kcBytes, exist := srt.Data[srtName]
	if !exist {
		return nil, fmt.Errorf("fail to get secret data %s: %s", srtName, err)
	}
	return kcBytes, nil
}

// genKubeConfig generates the kubeconfig file for accessing the virtual cluster
func genKubeConfig(cli client.Client, vc *tenancyv1alpha1.VirtualCluster, cv *tenancyv1alpha1.ClusterVersion) ([]byte, error) {
	clusterNamespace := conversion.ToClusterKey(vc)
	kbCfgBytes, err := getVcKubeConfig(cli, clusterNamespace, "admin-kubeconfig")
	if err != nil {
		return nil, err
	}

	kubecfg, err := clientcmd.NewClientConfigFromBytes(kbCfgBytes)
	if err != nil {
		return nil, err
	}

	apiSvcPort, err := getAPISvcPort(cv.Spec.APIServer.Service)
	if err != nil {
		return nil, err
	}

	// replace the server address in kubeconfig based on service type
	kubecfg, err = replaceServerAddr(kubecfg, cli, clusterNamespace, cv.Spec.APIServer.Service.Spec.Type, apiSvcPort)
	if err != nil {
		return nil, err
	}

	rawConfig, err := kubecfg.RawConfig()
	if err != nil {
		return nil, err
	}

	return clientcmd.Write(rawConfig)
}

// replaceServerAddr replace api server IP with the minikube gateway IP, and
// disable TLS varification by removing the server CA
func replaceServerAddr(kubecfg clientcmd.ClientConfig, cli client.Client, clusterNamespace string, svcType v1.ServiceType, apiSvcPort int) (clientcmd.ClientConfig, error) {
	var newStr string
	switch svcType {
	case v1.ServiceTypeNodePort:
		nodeIP, err := netutil.GetNodeIP(cli)
		if err != nil {
			return nil, err
		}
		svcNodePort, err := netutil.GetSvcNodePort(APIServerSvcName, clusterNamespace, cli)
		if err != nil {
			return nil, err
		}
		newStr = fmt.Sprintf("https://%s:%d", nodeIP, svcNodePort)
	case v1.ServiceTypeLoadBalancer:
		externalIP, err := netutil.GetLBIP(APIServerSvcName, clusterNamespace, cli)
		if err != nil {
			return nil, err
		}
		newStr = fmt.Sprintf("https://%s:%d", externalIP, apiSvcPort)
	}

	rawConfig, err := kubecfg.RawConfig()
	if err != nil {
		return nil, err
	}
	for _, cluster := range rawConfig.Clusters {
		cluster.InsecureSkipTLSVerify = true
		cluster.CertificateAuthorityData = nil
		cluster.Server = newStr
	}

	return clientcmd.NewDefaultClientConfig(rawConfig, &clientcmd.ConfigOverrides{}), nil
}
