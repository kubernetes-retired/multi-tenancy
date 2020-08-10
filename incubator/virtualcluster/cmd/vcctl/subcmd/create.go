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
package subcmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	vcctlutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/cmd/vcctl/util"
	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	kubeutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	netutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/net"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"

	// Import all Auth Providers
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	DefaultPKIExpireDays = 365
	APIServerSvcName     = "apiserver-svc"

	pollStsPeriodSec  = 2
	pollStsTimeoutSec = 120
)

// Create creates an object based on the file yamlPath
func Create(yamlPath, vcKbCfg string) error {
	if _, err := os.Stat(vcKbCfg); err == nil {
		return fmt.Errorf("--vckbcfg %s file exists", vcKbCfg)
	}
	if yamlPath == "" {
		return errors.New("please specify the path of the virtualcluster yaml file")
	}
	kbCfg, err := config.GetConfig()
	if err != nil {
		return err
	}
	// create a new scheme that has virtualcluster and clusterversion registered
	cliScheme := scheme.Scheme
	err = tenancyv1alpha1.AddToScheme(cliScheme)
	if err != nil {
		return err
	}

	obj, err := vcctlutil.YamlToObj(cliScheme, yamlPath)
	if err != nil {
		return err
	}

	// create a new client to talk to apiserver directly
	// NOTE the client returned by manager.GetClient() will talk to local cache
	cli, err := client.New(kbCfg, client.Options{Scheme: cliScheme})
	if err != nil {
		return err
	}

	switch o := obj.(type) {
	case *tenancyv1alpha1.VirtualCluster:
		log.Printf("creating VirtualCluster %s", o.Name)
		err = createVirtualCluster(cli, o, vcKbCfg)
		if err != nil {
			return err
		}
	case *tenancyv1alpha1.ClusterVersion:
		log.Printf("creating ClusterVersion %s", o.Name)
		err = cli.Create(context.TODO(), o)
		if err != nil {
			return err
		}
	default:
		return errors.New("unknown object kind. please use a ClusterVersion or VirtualCluster yaml")
	}

	return nil
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

// createVirtualCluster creates a virtual cluster based on the file yamlPath and
// generates the kubeconfig file for accessing the virtual cluster
func createVirtualCluster(cli client.Client, vc *tenancyv1alpha1.VirtualCluster, vcKbCfg string) error {
	cv := &tenancyv1alpha1.ClusterVersion{}
	if err := cli.Get(context.TODO(), types.NamespacedName{
		Namespace: "default",
		Name:      vc.Spec.ClusterVersionName,
	}, cv); err != nil {
		return err
	}

	apiSvcPort, err := getAPISvcPort(cv.Spec.APIServer.Service)
	if err != nil {
		return err
	}

	// fail early, if service type is not supported
	svcType := cv.Spec.APIServer.Service.Spec.Type
	if svcType != v1.ServiceTypeNodePort &&
		svcType != v1.ServiceTypeLoadBalancer &&
		svcType != v1.ServiceTypeClusterIP {
		return fmt.Errorf("unsupported apiserver service type: %s", svcType)
	}

	if err := cli.Create(context.TODO(), vc); err != nil {
		return err
	}
	ns := conversion.ToClusterKey(vc)

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "etcd", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return fmt.Errorf("cannot find sts/etcd in ns %s: %s", ns, err)
	}
	log.Println("etcd is ready")

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "apiserver", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return fmt.Errorf("cannot find sts/apiserver in ns %s: %s", ns, err)
	}
	log.Println("apiserver is ready")

	if err := retryIfNotFound(5, 2, func() error {
		return kubeutil.WaitStatefulSetReady(cli, ns, "controller-manager", pollStsTimeoutSec, pollStsPeriodSec)
	}); err != nil {
		return fmt.Errorf("cannot find sts/controller-manager in ns %s: %s", ns, err)
	}
	log.Println("controller-manager is ready")

	return genKubeConfig(ns, vcKbCfg, cli, svcType, apiSvcPort)
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
func genKubeConfig(clusterNamespace, vcKbCfg string, cli client.Client, svcType v1.ServiceType, apiSvcPort int) error {
	// get the content of admin.kubeconfig and write to vcKbCfg
	fn, err := os.OpenFile(vcKbCfg, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	kbCfgBytes, err := getVcKubeConfig(cli, clusterNamespace, "admin-kubeconfig")
	if err != nil {
		return err
	}

	kubecfg, err := clientcmd.NewClientConfigFromBytes(kbCfgBytes)
	if err != nil {
		return err
	}
	// replace the server address in kubeconfig based on service type
	kubecfg, err = replaceServerAddr(kubecfg, cli, clusterNamespace, svcType, apiSvcPort)
	if err != nil {
		return err
	}
	rawConfig, err := kubecfg.RawConfig()
	if err != nil {
		return err
	}
	kubecfgBytes, err := clientcmd.Write(rawConfig)
	if err != nil {
		return err
	}
	n, err := fn.Write(kubecfgBytes)
	if err != nil {
		return err
	}
	if n != len(kbCfgBytes) {
		return fmt.Errorf("fail to write kubeconfig bytes to file: wrote %d of %d bytes", n, len(kbCfgBytes))
	}
	return nil
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
		svcNodePort, err := netutil.GetSvcNodePort(APIServerSvcName,
			clusterNamespace, cli)
		if err != nil {
			return nil, err
		}
		newStr = fmt.Sprintf("https://%s:%d", nodeIP, svcNodePort)
	case v1.ServiceTypeLoadBalancer:
		externalIP, err := netutil.GetLBIP(APIServerSvcName,
			clusterNamespace, cli)
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

// getMinikubeIP gets the ip that is used for accessing minikube
func getMinikubeIP() (string, error) {
	cmd := exec.Command("minikube", "ip")
	IP, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("fail to get minikube ip (ERRNO %s)", exitErr)
		}
		return "", fmt.Errorf("fail to get minikube ip: %s", err)
	}
	return strings.TrimSuffix(string(IP), "\n"), nil
}

