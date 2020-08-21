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
	"context"
	"fmt"
	"os"
	"time"

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

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/version"
)

var k8sClient *kubernetes.Clientset
var hncClient *rest.RESTClient
var rootCmd *cobra.Command
var client Client

type realClient struct{}

type Client interface {
	getHierarchy(nnm string) *api.HierarchyConfiguration
	updateHierarchy(hier *api.HierarchyConfiguration, reason string)
	createAnchor(nnm string, hnnm string)
	getAnchorNames(nnm string) []string
	getHNCConfig() *api.HNCConfiguration
	updateHNCConfig(*api.HNCConfiguration)
}

func init() {
	api.AddToScheme(scheme.Scheme)

	client = &realClient{}

	kubecfgFlags := genericclioptions.NewConfigFlags(false)

	rootCmd = &cobra.Command{
		// We should use "kubectl hns" instead of "kubectl-hns" to invoke the plugin.
		// However, since only the first word of the Use field will be displayed in the
		// "Usage" section of a root command, we set it to "kubectl-hns" here so that both
		// "kubectl" and "hns" will be shown in the "Usage" section.
		Use:     "kubectl-hns",
		Version: version.Version,
		Short:   "Manipulates hierarchical namespaces provided by HNC",
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
	rootCmd.AddCommand(newVersionCmd())
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := k8sClient.CoreV1().Namespaces().Get(ctx, nnm, metav1.GetOptions{}); err != nil {
		fmt.Printf("Error reading namespace %s: %s\n", nnm, err)
		os.Exit(1)
	}
	hier := &api.HierarchyConfiguration{}
	hier.Name = api.Singleton
	hier.Namespace = nnm
	err := hncClient.Get().Resource(api.HierarchyConfigurations).Namespace(nnm).Name(api.Singleton).Do(ctx).Into(hier)
	if err != nil && !errors.IsNotFound(err) {
		fmt.Printf("Error reading hierarchy for %s: %s\n", nnm, err)
		os.Exit(1)
	}
	return hier
}

func (cl *realClient) getAnchorNames(nnm string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var anms []string

	// List all the anchors in the namespace.
	ul := &unstructured.UnstructuredList{}
	ul.SetKind(api.AnchorKind)
	ul.SetAPIVersion(api.AnchorAPIVersion)
	err := hncClient.Get().Resource(api.Anchors).Namespace(nnm).Do(ctx).Into(ul)
	if err != nil && !errors.IsNotFound(err) {
		fmt.Printf("Error listing subnamespace anchors for %s: %s\n", nnm, err)
		os.Exit(1)
	}

	// Create a list of strings of the anchor names.
	for _, inst := range ul.Items {
		anms = append(anms, inst.GetName())
	}

	return anms
}

func (cl *realClient) updateHierarchy(hier *api.HierarchyConfiguration, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nnm := hier.Namespace
	var err error
	if hier.CreationTimestamp.IsZero() {
		err = hncClient.Post().Resource(api.HierarchyConfigurations).Namespace(nnm).Name(api.Singleton).Body(hier).Do(ctx).Error()
	} else {
		err = hncClient.Put().Resource(api.HierarchyConfigurations).Namespace(nnm).Name(api.Singleton).Body(hier).Do(ctx).Error()
	}
	if err != nil {
		fmt.Printf("\nCould not %s.\nReason: %s\n", reason, err)
		os.Exit(1)
	}
}

func (cl *realClient) createAnchor(nnm string, hnnm string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	anchor := &api.SubnamespaceAnchor{}
	anchor.Name = hnnm
	anchor.Namespace = nnm
	err := hncClient.Post().Resource(api.Anchors).Namespace(nnm).Name(hnnm).Body(anchor).Do(ctx).Error()
	if err != nil {
		fmt.Printf("\nCould not create subnamespace anchor.\nReason: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully created %q subnamespace anchor in %q namespace\n", hnnm, nnm)
}

func (cl *realClient) getHNCConfig() *api.HNCConfiguration {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	config := &api.HNCConfiguration{}
	config.Name = api.HNCConfigSingleton
	err := hncClient.Get().Resource(api.HNCConfigSingletons).Name(api.HNCConfigSingleton).Do(ctx).Into(config)
	if err != nil && !errors.IsNotFound(err) {
		fmt.Printf("Error reading the HNC Configuration: %s\n", err)
		os.Exit(1)
	}
	return config
}

func (cl *realClient) updateHNCConfig(config *api.HNCConfiguration) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var err error
	if config.CreationTimestamp.IsZero() {
		err = hncClient.Post().Resource(api.HNCConfigSingletons).Name(api.HNCConfigSingleton).Body(config).Do(ctx).Error()
	} else {
		err = hncClient.Put().Resource(api.HNCConfigSingletons).Name(api.HNCConfigSingleton).Body(config).Do(ctx).Error()
	}
	if err != nil {
		fmt.Printf("\nCould not update the HNC Configuration: %s\n", err)
		os.Exit(1)
	}
}
