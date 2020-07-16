package unittestutils

import (
	"fmt"
	"log"
	"os"

	"sigs.k8s.io/kind/pkg/apis/config/v1alpha4"

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

	k.Provider = newProvider
	fmt.Println(k.Name, "cluster is created")
	return nil
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
