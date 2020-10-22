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

var setResourceCmd = &cobra.Command{
	Use: fmt.Sprintf("set-resource RESOURCE [--group GROUP] [--force] --mode <%s|%s|%s>",
		api.Propagate, api.Remove, api.Ignore),
	Short: "Sets the HNC configuration of a specific resource",
	Example: fmt.Sprintf("  # Set configuration of a core type\n" +
		"  kubectl hns config set-resource secrets --mode Ignore\n\n" +
		"  # Set configuration of a custom type\n" +
		"  kubectl hns config set-resource crontabs --group stable.example.com --mode Propagate"),
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resource := args[0]
		flags := cmd.Flags()
		group, _ := flags.GetString("group")
		modeStr, _ := flags.GetString("mode")
		mode := api.SynchronizationMode(modeStr)
		force, _ := flags.GetBool("force")
		config := client.getHNCConfig()

		exist := false
		for i := 0; i < len(config.Spec.Resources); i++ {
			r := &config.Spec.Resources[i]
			if r.Group == group && r.Resource == resource {
				if r.Mode == api.Ignore && mode == api.Propagate && !force {
					fmt.Printf("Switching directly from 'Ignore' to 'Propagate' mode could cause existing %q objects in "+
						"child namespaces to be overwritten by objects from ancestor namespaces.\n", resource)
					fmt.Println("If you are sure you want to proceed with this operation, use the '--force' flag.")
					fmt.Println("If you are not sure and would like to see what source objects would be overwritten," +
						"please switch to 'Remove' first. To see how to enable propagation safely, refer to " +
						"https://github.com/kubernetes-sigs/multi-tenancy/blob/master/incubator/hnc/docs/user-guide/how-to.md#admin-types")
					os.Exit(1)
				}
				r.Mode = mode
				exist = true
				break
			}
		}

		if !exist {
			config.Spec.Resources = append(config.Spec.Resources,
				api.ResourceSpec{
					Group:    group,
					Resource: resource,
					Mode:     mode,
				})
		}

		client.updateHNCConfig(config)
	},
}

func newSetResourceCmd() *cobra.Command {
	setResourceCmd.Flags().String("group", "", "The group of the resource; may be omitted for core resources (or explicitly set to the empty string)")
	setResourceCmd.Flags().String("mode", "", "The synchronization mode: one of Propagate, Remove or Ignore")
	setResourceCmd.Flags().BoolP("force", "f", false, "Allow the synchronization mode to be changed directly from Ignore to Propagate despite the dangers of doing so")
	return setResourceCmd
}
