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

var setParentCmd = &cobra.Command{
	Use:     "set-parent <namespace> <parent>: sets the parent of the given namespace",
	Aliases: []string{"set"},
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		pnm := args[1]
		hier := getHierarchy(nnm)
		oldPNM := hier.Spec.Parent
		if oldPNM == pnm {
			fmt.Printf("Parent of %s is already %s; unchanged\n", nnm, pnm)
			return
		}
		hier.Spec.Parent = pnm
		updateHierarchy(hier, fmt.Sprintf("setting the parent of %s to %s", nnm, pnm))
		if oldPNM == "" {
			fmt.Printf("Set the parent of %s to %s\n", nnm, pnm)
		} else {
			fmt.Printf("Changed the parent of %s from %s to %s\n", nnm, oldPNM, pnm)
		}
	},
}

func init() {
	rootCmd.AddCommand(setParentCmd)
}
