/*
Copyright 2020 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
)

// Factory provides abstractions that allow the Kubectl command to be extended across multiple types
// of resources and different API sets.
type Factory interface {
	// GenericClient from controller runtime
	GenericClient() (client.Client, error)

	// KubernetesClientSet gives you back an external clientset
	KubernetesClientSet() (kubernetes.Interface, error)

	// VirtualClusterClientSet is the virtualcluster clientset
	VirtualClusterClientSet() (vcclient.Interface, error)
}

type factoryImpl struct {
	config *rest.Config
}

func NewFactory() (Factory, error) {
	kubecfgFlags := genericclioptions.NewConfigFlags(true)
	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	return &factoryImpl{config: config}, nil
}

func (f *factoryImpl) GenericClient() (client.Client, error) {
	return client.New(f.config, client.Options{Scheme: scheme.Scheme})
}

func (f *factoryImpl) KubernetesClientSet() (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(f.config)
}

func (f *factoryImpl) VirtualClusterClientSet() (vcclient.Interface, error) {
	return vcclient.NewForConfig(f.config)
}

func UsageErrorf(cmd *cobra.Command, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s\nSee '%s -h' for help and examples", msg, cmd.CommandPath())
}

func CheckErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// getYamlContent reads the yaml content from the `yamlPath`
func getYamlContent(yamlPath string) ([]byte, error) {
	if isURL(yamlPath) {
		// read from an URL
		resp, err := http.Get(yamlPath)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		yamlContent, err := ioutil.ReadAll(resp.Body)
		return yamlContent, nil
	}
	// read from a file
	yamlContent, err := ioutil.ReadFile(yamlPath)
	return yamlContent, err
}

// isURL checks if `path` is an URL
func isURL(path string) bool {
	return strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://")
}
