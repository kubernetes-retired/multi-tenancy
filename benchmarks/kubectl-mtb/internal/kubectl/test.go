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
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
)

var (
	tenant          string
	tenantNamespace string
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run the Multi-Tenancy Benchmarks",

	Run: func(cmd *cobra.Command, args []string) {
		validateFlags(cmd)
		runTests()
	},
}

// Validation of the flag inputs
func validateFlags(cmd *cobra.Command) {
	tenant, _ = cmd.Flags().GetString("tenant-admin")
	if tenant == "" {
		color.Red("Error: tenant-admin must be set via --tenant-admin or -t")
		os.Exit(1)
	}

	tenantNamespace, _ = cmd.Flags().GetString("namespace")
	if tenantNamespace == "" {
		color.Red("Error: tenant namespace must be set via --namespace or -n")
		os.Exit(1)
	}
}

func runTests() {

	kubecfgFlags := genericclioptions.NewConfigFlags(false)

	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// create the K8s clientset
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	impersonateServiceAccount := "system:serviceaccount:" + tenantNamespace + ":" + tenant
	tenantConfig := config
	tenantConfig.Impersonate.UserName = impersonateServiceAccount

	// create the tenant clientset
	tenantClient, err := kubernetes.NewForConfig(tenantConfig)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for _, b := range benchmarks {
		_, err := b.Run(tenantNamespace, k8sClient, tenantClient)
		if err != nil {
			fmt.Println(err)
		}
	}

}

func newTestCmd() *cobra.Command {
	testCmd.Flags().StringP("namespace", "n", "", "name of tenant-admin namespace")
	testCmd.Flags().StringP("tenant-admin", "t", "", "name of tenant service account")

	return testCmd
}