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

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

var configDeleteCmd = &cobra.Command{
	Use:   "delete-type --group X --resource Y",
	Short: "Delete the HNC configuration of a specific type",
	Example: fmt.Sprintf("  # Delete configuration of a core type\n" +
		"  kubectl hns config delete-type --resource secrets\n\n" +
		"  # Delete configuration of a custom type\n" +
		"  kubectl hns config delete-type --group stable.example.com --resource crontabs"),
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		group, _ := flags.GetString("group")
		resource, _ := flags.GetString("resource")
		config := client.getHNCConfig()

		var newRscs []api.ResourceSpec
		exist := false
		for _, r := range config.Spec.Resources {
			if r.Group == group && r.Resource == resource {
				exist = true
			} else {
				newRscs = append(newRscs, r)
			}
		}
		if !exist {
			fmt.Printf("Nothing to delete; No configuration for type with group: %s, "+
				"resource: %s\n", group, resource)
			return
		}
		config.Spec.Resources = newRscs
		client.updateHNCConfig(config)
		fmt.Printf("Configuration for type with group: %s, resource: %s is deleted\n", group, resource)
	},
}

func newConfigDeleteCmd() *cobra.Command {
	configDeleteCmd.Flags().String("group", "", "group of the resource")
	configDeleteCmd.Flags().String("resource", "", "resource to be configured")
	return configDeleteCmd
}
