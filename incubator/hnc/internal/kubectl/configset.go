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

var setTypeCmd = &cobra.Command{
	Use: fmt.Sprintf("set-type --apiVersion X --kind Y <%s|%s|%s>",
		api.Propagate, api.Remove, api.Ignore),
	Short: "Sets the HNC configuration of a specific resources type",
	Example: fmt.Sprintf("  # Set configuration of a core type\n" +
		"  kubectl hns config set-type --apiVersion v1 --kind Secret ignore\n\n" +
		"  # Set configuration of a custom type\n" +
		"  kubectl hns config set-type --apiversion stable.example.com/v1 --kind CronTab propagate"),
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mode := api.SynchronizationMode(args[0])
		flags := cmd.Flags()
		apiVersion, _ := flags.GetString("apiVersion")
		kind, _ := flags.GetString("kind")
		config := client.getHNCConfig()

		exist := false
		for i := 0; i < len(config.Spec.Types); i++ {
			t := &config.Spec.Types[i]
			if t.APIVersion == apiVersion && t.Kind == kind {
				t.Mode = mode
				exist = true
				break
			}
		}

		if !exist {
			config.Spec.Types = append(config.Spec.Types,
				api.TypeSynchronizationSpec{
					APIVersion: apiVersion,
					Kind:       kind,
					Mode:       mode,
				})
		}

		client.updateHNCConfig(config)
	},
}

func newSetTypeCmd() *cobra.Command {
	setTypeCmd.Flags().String("apiVersion", "", "API version of the kind")
	setTypeCmd.Flags().String("kind", "", "Kind to be configured")
	return setTypeCmd
}
