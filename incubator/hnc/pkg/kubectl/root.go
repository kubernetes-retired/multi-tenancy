/*

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

// Package kubectl implements the HNC kubectl plugin
package kubectl

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var k8sClient *kubernetes.Clientset
var hncClient *rest.RESTClient
var rootCmd *cobra.Command
var client Client

type realClient struct{}

type Client interface {
	getHierarchy(nnm string) *api.HierarchyConfiguration
	updateHierarchy(hier *api.HierarchyConfiguration, reason string)
	createHierarchicalNamespace(nnm string, hnnm string)
	getHierarchicalNamespacesNames(nnm string) []string
	getHNCConfig() *api.HNCConfiguration
	updateHNCConfig(*api.HNCConfiguration)
}

func init() {
	api.AddToScheme(scheme.Scheme)

	client = &realClient{}

	kubecfgFlags := genericclioptions.NewConfigFlags(false)

	rootCmd = &cobra.Command{
		Use:   "kubectl hierarchical-namespaces",
		Short: "Manipulates hierarchical namespaces provided by HNC",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			config, err := kubecfgFlags.ToRESTConfig()
			if err != nil {
				return err
			}

			// create the K8s clientset
			k8sClient, err = kubernetes.NewForConfig(config)
			if err != nil {
				return err
			}

			// create the HNC clientset
			hncConfig := *config
			hncConfig.ContentConfig.GroupVersion = &api.GroupVersion
			hncConfig.APIPath = "/apis"
			hncConfig.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs}
			hncConfig.UserAgent = rest.DefaultKubernetesUserAgent()
			hncClient, err = rest.UnversionedRESTClientFor(&hncConfig)
			if err != nil {
				return err
			}

			return nil
		},
	}
	kubecfgFlags.AddFlags(rootCmd.PersistentFlags())

	rootCmd.AddCommand(newSetCmd())
	rootCmd.AddCommand(newDescribeCmd())
	rootCmd.AddCommand(newTreeCmd())
	rootCmd.AddCommand(newCreateCmd())
	rootCmd.AddCommand(newConfigCmd())
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func (cl *realClient) getHierarchy(nnm string) *api.HierarchyConfiguration {
	if _, err := k8sClient.CoreV1().Namespaces().Get(nnm, metav1.GetOptions{}); err != nil {
		fmt.Printf("Error reading namespace %s: %s\n", nnm, err)
		os.Exit(1)
	}
	hier := &api.HierarchyConfiguration{}
	hier.Name = api.Singleton
	hier.Namespace = nnm
	err := hncClient.Get().Resource(api.HierarchyConfigurations).Namespace(nnm).Name(api.Singleton).Do().Into(hier)
	if err != nil && !errors.IsNotFound(err) {
		fmt.Printf("Error reading hierarchy for %s: %s\n", nnm, err)
		os.Exit(1)
	}
	return hier
}

func (cl *realClient) getHierarchicalNamespacesNames(nnm string) []string {
	var hnsnms []string

	// List all the hns instance in the namespace.
	ul := &unstructured.UnstructuredList{}
	ul.SetKind(api.HierarchicalNamespacesKind)
	ul.SetAPIVersion(api.HierarchicalNamespacesAPIVersion)
	err := hncClient.Get().Resource(api.HierarchicalNamespaces).Namespace(nnm).Do().Into(ul)
	if err != nil && !errors.IsNotFound(err) {
		fmt.Printf("Error listing hierarchicalNamespaces for %s: %s\n", nnm, err)
		os.Exit(1)
	}

	// Create a list of strings of the hns names.
	for _, inst := range ul.Items {
		hnsnms = append(hnsnms, inst.GetName())
	}

	return hnsnms
}

func (cl *realClient) updateHierarchy(hier *api.HierarchyConfiguration, reason string) {
	nnm := hier.Namespace
	var err error
	if hier.CreationTimestamp.IsZero() {
		err = hncClient.Post().Resource(api.HierarchyConfigurations).Namespace(nnm).Name(api.Singleton).Body(hier).Do().Error()
	} else {
		err = hncClient.Put().Resource(api.HierarchyConfigurations).Namespace(nnm).Name(api.Singleton).Body(hier).Do().Error()
	}
	if err != nil {
		fmt.Printf("\nCould not %s.\nReason: %s\n", reason, err)
		os.Exit(1)
	}
}

func (cl *realClient) createHierarchicalNamespace(nnm string, hnnm string) {
	hns := &api.HierarchicalNamespace{}
	hns.Name = hnnm
	hns.Namespace = nnm
	err := hncClient.Post().Resource(api.HierarchicalNamespaces).Namespace(nnm).Name(hnnm).Body(hns).Do().Error()
	if err != nil {
		fmt.Printf("\nCould not create hierarchicalnamespace instance.\nReason: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully created \"%s\" hierarchicalnamespace instance in \"%s\" namespace\n", hnnm, nnm)
}

func (cl *realClient) getHNCConfig() *api.HNCConfiguration {
	config := &api.HNCConfiguration{}
	config.Name = api.HNCConfigSingleton
	err := hncClient.Get().Resource(api.HNCConfigSingletons).Name(api.HNCConfigSingleton).Do().Into(config)
	if err != nil && !errors.IsNotFound(err) {
		fmt.Printf("Error reading the HNC Configuration: %s\n", err)
		os.Exit(1)
	}
	return config
}

func (cl *realClient) updateHNCConfig(config *api.HNCConfiguration) {
	var err error
	if config.CreationTimestamp.IsZero() {
		err = hncClient.Post().Resource(api.HNCConfigSingletons).Name(api.HNCConfigSingleton).Body(config).Do().Error()
	} else {
		err = hncClient.Put().Resource(api.HNCConfigSingletons).Name(api.HNCConfigSingleton).Body(config).Do().Error()
	}
	if err != nil {
		fmt.Printf("\nCould not update the HNC Configuration: %s\n", err)
		os.Exit(1)
	}
}
