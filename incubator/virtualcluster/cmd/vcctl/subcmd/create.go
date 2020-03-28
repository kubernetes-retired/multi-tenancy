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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	vcctlutil "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/cmd/vcctl/util"
	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	kubeutil "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
	netutil "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/util/net"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"

	_ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
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
	mgr, err := manager.New(kbCfg,
		manager.Options{MetricsBindAddress: ":8081"})
	if err != nil {
		return err
	}

	err = tenancyv1alpha1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	obj, err := vcctlutil.YamlToObj(mgr.GetScheme(), yamlPath)
	if err != nil {
		return err
	}

	// create a new client to talk to apiserver directly
	// NOTE the client returned by manager.GetClient() will talk to local cache
	cli, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return err
	}

	switch o := obj.(type) {
	case *tenancyv1alpha1.Virtualcluster:
		log.Printf("creating Virtualcluster %s", o.Name)
		err = createVirtualcluster(cli, o, vcKbCfg)
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
		return errors.New("unknown object kind. please use a ClusterVersion or Virtualcluster yaml")
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

// createVirtualcluster creates a virtual cluster based on the file yamlPath and
// generates the kubeconfig file for accessing the virtual cluster
func createVirtualcluster(cli client.Client, vc *tenancyv1alpha1.Virtualcluster, vcKbCfg string) error {
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
		svcType != v1.ServiceTypeLoadBalancer {
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
	// replace the server address in kubeconfig based on service type
	kbCfgBytes, err = replaceServerAddr(kbCfgBytes, cli, clusterNamespace, svcType, apiSvcPort)
	if err != nil {
		return err
	}

	n, err := fn.Write(kbCfgBytes)
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
func replaceServerAddr(kubeCfgContent []byte, cli client.Client, clusterNamespace string, svcType v1.ServiceType, apiSvcPort int) ([]byte, error) {
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
		newStr = fmt.Sprintf("server: https://%s:%d", nodeIP, svcNodePort)
	case v1.ServiceTypeLoadBalancer:
		externalIP, err := netutil.GetLBIP(APIServerSvcName,
			clusterNamespace, cli)
		if err != nil {
			return nil, err
		}
		newStr = fmt.Sprintf("server: https://%s:%d", externalIP, apiSvcPort)
	}

	lines := strings.Split(string(kubeCfgContent), "\n")
	// remove server CA, disable TLS varification
	for i := 0; i < len(lines); {
		if strings.Contains(lines[i], "certificate-authority-data: ") {
			lines = append(lines[:i], lines[i+1:]...)
			continue
		}
		if strings.Contains(lines[i], "server: ") {
			newSvrAddr, disableTLS := genNewLines(lines[i], newStr)
			lines[i] = newSvrAddr
			lines = insertStrAt(disableTLS, i+1, lines)
		}
		i++
	}
	return []byte(strings.Join(lines, "\n")), nil
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

// genNewLines generates new lines contain "insecure-skip-tls-verify: true"
// and new server address
func genNewLines(oldLine, newLine string) (string, string) {
	numSpace := countHeadingSpace(oldLine)
	disableTLS := "insecure-skip-tls-verify: true"

	for i := 0; i < numSpace-1; i++ {
		newLine = " " + newLine
		disableTLS = " " + disableTLS
	}
	return newLine, disableTLS
}

// countHeadingSpace counts the number of indents
func countHeadingSpace(inp string) int {
	var count int
	for _, c := range inp {
		if c == ' ' {
			count++
		}
	}
	return count
}

// insertStrAt inserts str at i of strSlice
func insertStrAt(str string, i int, strSlice []string) []string {
	return append(strSlice[:i], append([]string{str}, strSlice[i:]...)...)
}
