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
	"os"

	"github.com/urfave/cli"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/version"
)

func main() {
	app := cli.NewApp()
	app.Name = "kubectl-vc"
	app.Usage = "VirtualCluster Command tool"
	app.Version = version.BriefVersion()

	app.Commands = []cli.Command{
		createCommand,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func newGenericK8sClient() (client.Client, error) {
	kubecfgFlags := genericclioptions.NewConfigFlags(true)
	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{Scheme: scheme.Scheme})
}

func newVCClient() (vcclient.Interface, error) {
	kubecfgFlags := genericclioptions.NewConfigFlags(true)
	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	return vcclient.NewForConfig(config)
}
