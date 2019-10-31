package test

import (
	"encoding/json"
	"io/ioutil"
	"os"

	yaml "k8s.io/apimachinery/pkg/util/yaml"
	kubernetes "k8s.io/client-go/kubernetes"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/test/e2e/framework"
)

func GetContextFromKubeconfig(kubeconfigpath string) string {
	apiConfig := clientcmd.GetConfigFromFileOrDie(kubeconfigpath)

	if apiConfig.CurrentContext == "" {
		framework.Failf("current-context is not set in %s", kubeconfigpath)
	}

	return apiConfig.CurrentContext
}

func NewKubeClientWithKubeconfig(kubeconfigpath string) *kubernetes.Clientset {
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigpath)
	framework.ExpectNoError(err)

	kclient, err := kubernetes.NewForConfig(clientConfig)
	framework.ExpectNoError(err)

	return kclient
}

func ReadConfig(path string) (*BenchmarkConfig, error) {
	var config *BenchmarkConfig

	file, err := LoadFile(path)
	if err != nil {
		return nil, err
	}

	data, err := yaml.ToJSON(file)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return config, nil
}

func LoadFile(path string) ([]byte, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}

	return ioutil.ReadFile(path)
}
