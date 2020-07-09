package unittestutils

import (
	"fmt"
	"log"
	"os"

	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
)

// KindCluster configures the Kind Cluster
type KindCluster struct {
	Name           string
	Nodes          int
	KubeConfig     string
	KubeConfigFile string
	Provider       *cluster.Provider
}

// Creates Cluster
func (k *KindCluster) CreateCluster() error {
	k.Name = "kubectl-mtb"
	nodeList := []v1alpha4.Node{}

	k.Nodes = 1
	for i := 0; i < k.Nodes; i++ {
		node := v1alpha4.Node{
			Role: v1alpha4.NodeRole("control-plane"),
		}
		nodeList = append(nodeList, node)
	}
	config := &v1alpha4.Cluster{
		Nodes: nodeList,
	}
	fmt.Println(k.Name, "cluster is being created")
	options := cluster.CreateWithV1Alpha4Config(config)
	newProvider := cluster.NewProvider()
	if err := newProvider.Create(
		k.Name,
		options,
	); err != nil {
		fmt.Println(err.Error())
		return err
	}
	k.KubeConfigFile, _ = newProvider.KubeConfig(k.Name, false)
	newProvider.ExportKubeConfig(k.Name, k.KubeConfigFile)

	if err != nil {
		log.Println(err.Error())
		return err
	}
	k.Provider = newProvider
	fmt.Println(k.Name, "cluster is created")
	return nil
}

func getClientSet(configPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

// Delete Kind Cluster
func (k *KindCluster) DeleteCluster() error {
	fmt.Println(k.Name, "cluster is being deleted")
	err := k.Provider.Delete(k.Name, k.KubeConfigFile)
	if err != nil {
		log.Fatal(err)
	}
	os.Remove(k.KubeConfigFile)
	return err
}
