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

	"github.com/spf13/cobra"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

var setTypeCmd = &cobra.Command{
	Use: fmt.Sprintf("set-type --group X --resource Y <%s|%s|%s>",
		api.Propagate, api.Remove, api.Ignore),
	Short: "Sets the HNC configuration of a specific resources type",
	Example: fmt.Sprintf("  # Set configuration of a core type\n" +
		"  kubectl hns config set-type --resource secrets Ignore\n\n" +
		"  # Set configuration of a custom type\n" +
		"  kubectl hns config set-type --group stable.example.com --resource crontabs Propagate"),
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mode := api.SynchronizationMode(args[0])
		flags := cmd.Flags()
		group, _ := flags.GetString("group")
		resource, _ := flags.GetString("resource")
		force, _ := flags.GetBool("force")
		config := client.getHNCConfig()

		exist := false
		for i := 0; i < len(config.Spec.Types); i++ {
			t := &config.Spec.Types[i]
			if t.Group == group && t.Resource == resource {
				if t.Mode == api.Ignore && mode == api.Propagate && !force {
					fmt.Printf("Switching directly from 'Ignore' to 'Propagate' mode could cause existing %q objects in "+
						"child namespaces to be overwritten by objects from ancestor namespaces.\n", resource)
					fmt.Println("If you are sure you want to proceed with this operation, use the '--force' flag.")
					fmt.Println("If you are not sure and would like to see what source objects would be overwritten," +
						"please switch to 'Remove' first. To see how to enable propagation safely, refer to " +
						"https://github.com/kubernetes-sigs/multi-tenancy/blob/master/incubator/hnc/docs/user-guide/how-to.md#admin-types")
					os.Exit(1)
				}
				t.Mode = mode
				exist = true
				break
			}
		}

		if !exist {
			config.Spec.Types = append(config.Spec.Types,
				api.TypeSynchronizationSpec{
					Group:    group,
					Resource: resource,
					Mode:     mode,
				})
		}

		client.updateHNCConfig(config)
	},
}

func newSetTypeCmd() *cobra.Command {
	setTypeCmd.Flags().String("group", "", "group of the resource")
	setTypeCmd.Flags().String("resource", "", "resource to be configured")
	setTypeCmd.Flags().BoolP("force", "f", false, "Force to set the propagation mode")
	return setTypeCmd
}
