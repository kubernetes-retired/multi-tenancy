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
	"time"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	kubeutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	netutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/net"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

const (
	APIServerSvcName = "apiserver-svc"

	pollStsPeriodSec  = 2
	pollStsTimeoutSec = 120
)

var createCommand = cli.Command{
	Name:  "create",
	Usage: "Create a new VirtualCluster",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "filename, f",
			Usage: "the configuration to apply",
		},
		cli.StringFlag{
			Name:  "output, o",
			Usage: "path to the kubeconfig that is used to access virtual cluster",
		},
	},
	Action: func(cctx *cli.Context) error {
		filename := cctx.String("filename")
		if filename == "" {
			return errors.New("must specific --filename,-f flag")
		}

		outputPath := cctx.String("output")
		if outputPath == "" {
			return errors.New("must specific --output,-o flag")
		}

		vccli, err := newVCClient()
		if err != nil {
			return err
		}

		fileBytes, err := getYamlContent(filename)
		if err != nil {
			return errors.Wrapf(err, "read \"%s\"", filename)
		}

		vc := &tenancyv1alpha1.VirtualCluster{}
		if err = yaml.Unmarshal(fileBytes, vc); err != nil {
			return err
		}

		kubecfgBytes, err := createVirtualCluster(vccli, vc)
		if err != nil {
			return err
		}

		// write tenant kubeconfig to outputPath.
		return ioutil.WriteFile(outputPath, kubecfgBytes, 0644)
	},
}

func createVirtualCluster(vccli vcclient.Interface, vc *tenancyv1alpha1.VirtualCluster) ([]byte, error) {
	cv, err := vccli.TenancyV1alpha1().ClusterVersions().Get(vc.Spec.ClusterVersionName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "required cluster version not found")
	}

	apiSvcPort, err := getAPISvcPort(cv.Spec.APIServer.Service)
	if err != nil {
		return nil, err
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

	cli, err := newGenericK8sClient()
	if err != nil {
		return nil, err
	}

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "etcd", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return nil, fmt.Errorf("cannot find sts/etcd in ns %s: %s", ns, err)
	}
	klog.Info("etcd is ready")

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "apiserver", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return nil, fmt.Errorf("cannot find sts/apiserver in ns %s: %s", ns, err)
	}
	klog.Info("apiserver is ready")

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "controller-manager", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return nil, fmt.Errorf("cannot find sts/controller-manager in ns %s: %s", ns, err)
	}
	klog.Info("controller-manager is ready")

	return genKubeConfig(ns, cli, svcType, apiSvcPort)
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
func genKubeConfig(clusterNamespace string, cli client.Client, svcType v1.ServiceType, apiSvcPort int) ([]byte, error) {
	kbCfgBytes, err := getVcKubeConfig(cli, clusterNamespace, "admin-kubeconfig")
	if err != nil {
		return nil, err
	}

	kubecfg, err := clientcmd.NewClientConfigFromBytes(kbCfgBytes)
	if err != nil {
		return nil, err
	}

	// replace the server address in kubeconfig based on service type
	kubecfg, err = replaceServerAddr(kubecfg, cli, clusterNamespace, svcType, apiSvcPort)
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
