package unittestutils

import (
	contextpkg "context"

	"k8s.io/client-go/dynamic"

	"github.com/op/go-logging"
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
	ServiceAccount string
	Context        contextpkg.Context
	Log            *logging.Logger
}

func TestNewClient(loggerName string, kubernetes kubernetes.Interface, apiExtensions apiextensionspkg.Interface, rest restpkg.Interface, config *restpkg.Config) *TestClient {
	return &TestClient{
		Kubernetes:    kubernetes,
		APIExtensions: apiExtensions,
		REST:          rest,
		Config:        config,

		Context: contextpkg.TODO(),
		Log:     logging.MustGetLogger(loggerName),
	}
}
