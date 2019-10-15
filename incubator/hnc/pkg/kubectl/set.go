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

var setCmd = &cobra.Command{
	Use:     "set",
	Short:   "Allows setting namespace hierarchy",
	Aliases: []string{"set"},
	Args:    cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		hier := getHierarchy(nnm)
		oldpnm := hier.Spec.Parent

		if cmd.Flags().Changed("root") {
			if oldpnm == "" {
				fmt.Printf("%s is already a root namespace; unchanged", nnm)
				return
			}
			hier.Spec.Parent = ""
			updateHierarchy(hier, fmt.Sprintf("unsetting the parent of %s", nnm))
			fmt.Printf("Unset the parent of %s (was previously %s)\n", nnm, oldpnm)
			return
		}

		if cmd.Flags().Changed("parent") {
			pnm, _ := cmd.Flags().GetString("parent")
			if oldpnm == pnm {
				fmt.Printf("Parent of %s is already %s; unchanged\n", nnm, pnm)
				return
			}
			hier.Spec.Parent = pnm
			updateHierarchy(hier, fmt.Sprintf("setting the parent of %s to %s", nnm, pnm))
			if oldpnm == "" {
				fmt.Printf("Set the parent of %s to %s\n", nnm, pnm)
			} else {
				fmt.Printf("Changed the parent of %s from %s to %s\n", nnm, oldpnm, pnm)
			}
		}

		if cmd.Flags().Changed("requiredChild") {
			rcns, _ := cmd.Flags().GetStringArray("requiredChild")

			for _, rcn := range rcns {
				ns := getHierarchy(nnm)
				if childNamespaceExists(ns, rcn) {
					fmt.Printf("Required child %s already present in %s\n", rcn, nnm)
					continue
				}
				ns.Spec.RequiredChildren = append(ns.Spec.RequiredChildren, rcn)
				updateHierarchy(ns, fmt.Sprintf("adding required child %s", rcn))
				fmt.Printf("Added required child %s\n", rcn)
			}
		}
	},
}

func init() {
	setCmd.Flags().Bool("root", false, "Turns namespace into root namespace")
	setCmd.Flags().String("parent", "", "Parent namespace")
	setCmd.Flags().StringArray("requiredChild", []string{""}, "Required Child namespace")
	rootCmd.AddCommand(setCmd)
}
