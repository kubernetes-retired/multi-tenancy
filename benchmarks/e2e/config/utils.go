package test

import (
	"io/ioutil"
	"os"
	"errors"
	"gopkg.in/yaml.v2"
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

	if err := yaml.Unmarshal(file, &config); err != nil {
		return nil, err
	}

	if config == nil {
		return config, errors.New("Please fill in a valid/non-empty yaml file")
	}
	return config, nil
}

func LoadFile(path string) ([]byte, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}

	return ioutil.ReadFile(path)
}
