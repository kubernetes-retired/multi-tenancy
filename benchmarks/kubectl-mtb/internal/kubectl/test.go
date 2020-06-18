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
package kubectl

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	tenant          string
	tenantNamespace string
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run the Multi-Tenancy Benchmarks",

	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(validateFlags(cmd))
		cmdutil.CheckErr(runTests())
	},
}

// Validation of the flag inputs
func validateFlags(cmd *cobra.Command) error {
	tenant, _ = cmd.Flags().GetString("tenant-admin")
	if tenant == "" {
		return fmt.Errorf("tenant-admin must be set via --tenant-admin or -t")
	}

	tenantNamespace, _ = cmd.Flags().GetString("namespace")
	if tenantNamespace == "" {
		return fmt.Errorf("tenant namespace must be set via --namespace or -n")
	}
	return nil
}

func runTests() error {

	kubecfgFlags := genericclioptions.NewConfigFlags(false)

	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		return err
	}

	// create the K8s clientset
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	tenantConfig := config
	tenantConfig.Impersonate.UserName = tenant

	// create the tenant clientset
	tenantClient, err := kubernetes.NewForConfig(tenantConfig)
	if err != nil {
		return err
	}

	for _, b := range benchmarks {
		_, err := b.PreRun(tenantNamespace, k8sClient, tenantClient)
		if err != nil {
			b.PreExit = 1
		}
		_, err := b.Run(tenantNamespace, k8sClient, tenantClient)

		if err != nil {
			return err
		}
	}
	return nil
}

func newTestCmd() *cobra.Command {
	testCmd.Flags().StringP("namespace", "n", "", "name of tenant-admin namespace")
	testCmd.Flags().StringP("tenant-admin", "t", "", "name of tenant service account")

	return testCmd
}
