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

var configDescribeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Displays information about the HNC configuration",
	Run: func(cmd *cobra.Command, args []string) {
		config := client.getHNCConfig()

		fmt.Println("Synchronized resources:")
		for _, r := range config.Status.Resources {
			action := ""
			switch r.Mode {
			case api.Propagate:
				action = "Propagating"
			case api.Remove:
				action = "Removing"
			default:
				action = "Ignoring"
			}
			fmt.Printf("* %s: %s (%s/%s)\n", action, r.Resource, r.Group, r.Version)
		}
		fmt.Print("\nConditions:\n")
		for _, c := range config.Status.Conditions {
			fmt.Printf("%s (%s): %s\n", c.Type, c.Reason, c.Message)
		}
	},
}

func newConfigDescribeCmd() *cobra.Command {
	return configDescribeCmd
}
