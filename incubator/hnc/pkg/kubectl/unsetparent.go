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
)

var unsetParentCmd = &cobra.Command{
	Use:     "unset-parent <namespace>: unsets the parent of the given namespace",
	Aliases: []string{"unset"},
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		hier := getHierarchy(nnm)
		oldPNM := hier.Spec.Parent
		if oldPNM == "" {
			fmt.Printf("%s is already a root namespace; unchanged", nnm)
			return
		}
		hier.Spec.Parent = ""
		updateHierarchy(hier, fmt.Sprintf("unsetting the parent of %s", nnm))
		fmt.Printf("Unset the parent of %s (was previously %s)\n", nnm, oldPNM)
	},
}

func init() {
	rootCmd.AddCommand(unsetParentCmd)
}
