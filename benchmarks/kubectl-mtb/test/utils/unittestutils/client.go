package unittestutils

import (
	contextpkg "context"

	"k8s.io/client-go/dynamic"

	"github.com/op/go-logging"
	corev1 "k8s.io/api/core/v1"
	apiextensionspkg "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/kubernetes"
	restpkg "k8s.io/client-go/rest"
)

// TestClient contains fields required to run unittests
type TestClient struct {
	Kubernetes      kubernetes.Interface
	APIExtensions   apiextensionspkg.Interface
	REST            restpkg.Interface
	Config          *restpkg.Config
	RoleName        string
	RoleBindingName string
	ResourcePath    string
	DynamicResource dynamic.ResourceInterface
	PolicyName      string
	TenantClient    *kubernetes.Clientset
	K8sClient       *kubernetes.Clientset

	Namespace      string
	ServiceAccount *corev1.ServiceAccount
	Context        contextpkg.Context
	Log            *logging.Logger
}

func TestNewClient(loggerName string, k8sClient *kubernetes.Clientset, apiExtensions apiextensionspkg.Interface, rest restpkg.Interface, config *restpkg.Config) *TestClient {
	return &TestClient{
		K8sClient:     k8sClient,
		APIExtensions: apiExtensions,
		REST:          rest,
		Config:        config,

		Context: contextpkg.TODO(),
		Log:     logging.MustGetLogger(loggerName),
	}
}
