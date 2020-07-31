package unittestutils

import (
	"context"
	"fmt"
	"time"

	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/tenant/pkg/apis/tenancy/v1alpha1"
	kyverno "github.com/nirmata/kyverno/pkg/api/kyverno/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	podutil "k8s.io/kubernetes/test/e2e/framework/pod"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	cfg     *rest.Config
	c       client.Client
	err     error
	timeout = time.Second * 40
)

// In future if we want to add more tenants and tenantnamespaces
var Tenants []*tenancyv1alpha1.Tenant
var Tenantnamespaces []*tenancyv1alpha1.TenantNamespace
var ServiceAccounts []*corev1.ServiceAccount

func NamespaceObj(name string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind: "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// ServiceAccountObj returns the pointer to a service account object
func ServiceAccountObj(name string, namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func WaitForKyvernoToReady(k8sClient *kubernetes.Clientset) error {
	var podsList *corev1.PodList
	for {
		podsList, err = k8sClient.CoreV1().Pods("kyverno").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}

		time.Sleep(1 * time.Second)
		if len(podsList.Items) > 0 {
			break
		}
	}
	podNames := []string{podsList.Items[0].ObjectMeta.Name}

	for {
		if podutil.CheckPodsRunningReady(k8sClient, "kyverno", podNames, 200*time.Second) {
			break
		}
	}
	return nil
}

func WaitForPolicy() error {
	kubecfgFlags := genericclioptions.NewConfigFlags(false)

	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	kyverno.AddToScheme(scheme.Scheme)

	crdConfig := *config
	crdConfig.ContentConfig.GroupVersion = &kyverno.SchemeGroupVersion
	crdConfig.APIPath = "/apis"
	crdConfig.NegotiatedSerializer = serializer.NewCodecFactory(scheme.Scheme)
	crdConfig.UserAgent = rest.DefaultKubernetesUserAgent()

	c, err := rest.RESTClientFor(&crdConfig)
	if err != nil {
		fmt.Println(err.Error())
	}

	result := kyverno.ClusterPolicyList{}
	for {
		err = c.
			Get().
			Namespace("").
			Resource("clusterpolicies").
			Do(context.TODO()).
			Into(&result)
		if err != nil {
			fmt.Println(err.Error())
			return err
			// continue
		} else {
			if len(result.Items) < 0 {
				continue
			}
			break
		}
	}
	return nil
}

// CheckNamespaceExist namespace exists or not
func CheckNamespaceExist(namespace string, k8sClient *kubernetes.Clientset) bool {
	_, err = k8sClient.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err == nil {
		return true
	}
	return false
}
