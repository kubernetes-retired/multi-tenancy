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

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var configDeleteCmd = &cobra.Command{
	Use:   "delete-type --apiVersion X --kind Y",
	Short: "Delete the HNC configuration of a specific type",
	Example: fmt.Sprintf("  # Delete configuration of a core type\n" +
		"  kubectl hns config delete-type --apiVersion v1 --kind Secret\n\n" +
		"  # Delete configuration of a custom type\n" +
		"  kubectl hns config delete-type --apiversion stable.example.com/v1 --kind CronTab"),
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		apiVersion, _ := flags.GetString("apiVersion")
		kind, _ := flags.GetString("kind")
		config := client.getHNCConfig()

		var newTypes []api.TypeSynchronizationSpec
		exist := false
		for _, t := range config.Spec.Types {
			if t.APIVersion == apiVersion && t.Kind == kind {
				exist = true
			} else {
				newTypes = append(newTypes, t)
			}
		}
		if !exist {
			fmt.Printf("Nothing to delete; No configuration for type with API version: %s, "+
				"kind: %s\n", apiVersion, kind)
			return
		}
		config.Spec.Types = newTypes
		client.updateHNCConfig(config)
		fmt.Printf("Configuration for type with API version: %s, kind: %s is deleted\n", apiVersion, kind)
	},
}

func newConfigDeleteCmd() *cobra.Command {
	configDeleteCmd.Flags().String("apiVersion", "", "API version of the kind")
	configDeleteCmd.Flags().String("kind", "", "Kind to be configured")
	return configDeleteCmd
}
