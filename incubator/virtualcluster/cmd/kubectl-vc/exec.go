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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

const (
	execExample = `
	# Switch to a virtualcluster
	kubectl vc exec -n foo bar

	# Specific vc by namespaced name
	kubectl vc exec foo/bar

	# Customize kubeconfig file path
	kubectl vc exec --kubeconfig-file-dir /path/to/file foo/bar`
)

type ExecOption struct {
	client      client.Client
	vcclient    vcclient.Interface
	namespace   string
	name        string
	kubeFileDir string
}

func NewCmdExec(f Factory) *cobra.Command {
	o := &ExecOption{}

	cmd := &cobra.Command{
		Use:     "exec VC_NAME",
		Short:   "Switch to virtualcluster workspace",
		Example: execExample,
		Run: func(cmd *cobra.Command, args []string) {
			CheckErr(o.Complete(f, cmd, args))
			CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVarP(&o.namespace, "namespace", "n", metav1.NamespaceDefault, "If present, the namespace scope for this CLI request")
	cmd.Flags().StringVar(&o.kubeFileDir, "kubeconfig-file-dir", filepath.Join(os.Getenv("HOME"), ".kube/vc/"), "The directory to place the kubeconfig of specific vc")

	return cmd
}

func (o *ExecOption) Complete(f Factory, cmd *cobra.Command, args []string) error {
	var err error
	o.vcclient, err = f.VirtualClusterClientSet()
	if err != nil {
		return err
	}

	o.client, err = f.GenericClient()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return UsageErrorf(cmd, "VC_NAME should not be empty")
	}

	o.name = args[0]
	if strings.Contains(o.name, "/") {
		namespacedName := strings.SplitN(o.name, "/", 2)
		o.namespace = namespacedName[0]
		o.name = namespacedName[1]
	}

	return nil
}

func (o *ExecOption) Run() error {
	kbFilePath, err := o.placeVCKubeconfig(o.namespace, o.name)
	if err != nil {
		return err
	}
	fmt.Printf("kubeconfig for virtualcluster %s/%s is placed at:\n\n\t%s\n\n", o.namespace, o.name, kbFilePath)

	return enterVCShell(kbFilePath, o.namespace, o.name)
}

func (o *ExecOption) placeVCKubeconfig(ns, name string) (string, error) {
	vc, err := o.vcclient.TenancyV1alpha1().VirtualClusters(ns).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	cv, err := o.vcclient.TenancyV1alpha1().ClusterVersions().Get(vc.Spec.ClusterVersionName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrapf(err, "cluster version not found")
	}

	kbBytes, err := genKubeConfig(o.client, vc, cv)
	if err != nil {
		return "", err
	}

	if err = os.MkdirAll(o.kubeFileDir, 0755); err != nil {
		return "", err
	}

	kbFilePath := filepath.Join(o.kubeFileDir, conversion.ToClusterKey(vc)+".kubeconfig")
	err = ioutil.WriteFile(kbFilePath, kbBytes, 0644)

	return kbFilePath, err
}

func enterVCShell(kbFilePath, ns, name string) error {
	warningPrompt := "!!"
	if isSmartTerminal() {
		warningPrompt = "❗"
	}
	fmt.Printf("%s You are now at VirtualCluster %s/%s\n", warningPrompt, ns, name)
	fmt.Printf("%s use regular kubectl commands to operate vc in this temporary workspace\n", warningPrompt)
	fmt.Printf("%s type 'exit' to exit\n", warningPrompt)

	c := exec.Command(os.Getenv("SHELL"))
	c.Env = append(os.Environ(),
		fmt.Sprintf("KUBECONFIG=%v", kbFilePath),
		fmt.Sprintf("PS1=[\\u@vc:\\[\033[01;32m\\]%s/%s\\[\033[00m\\] \\W]\\$ ", ns, name),
	)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	defer func() {
		fmt.Printf("%s exit VirtualCluster %s/%s\n", warningPrompt, ns, name)
	}()
	return c.Run()
}

func isSmartTerminal() bool {
	// Explicit request for no ANSI escape codes
	// https://no-color.org/
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// Explicitly dumb terminals are not smart
	// https://en.wikipedia.org/wiki/Computer_terminal#Dumb_terminals
	if os.Getenv("TERM") == "dumb" {
		return false
	}

	// On Windows WT_SESSION is set by the modern terminal component.
	// Older terminals have poor support for UTF-8, VT escape codes, etc.
	if runtime.GOOS == "windows" && os.Getenv("WT_SESSION") == "" {
		return false
	}

	return true
}
