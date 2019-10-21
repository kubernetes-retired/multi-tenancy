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
)

var setCmd = &cobra.Command{
	Use:     "set",
	Short:   "Allows setting namespace hierarchy",
	Aliases: []string{"set"},
	Args:    cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		hc := getHierarchy(nnm)
		oldpnm := hc.Spec.Parent
		flags := cmd.Flags()
		numChanges := 0

		if flags.Changed("root") && flags.Changed("parent") {
			fmt.Println("Cannot set both --root and --parent at the same time")
			os.Exit(1)
		}

		if flags.Changed("root") {
			if oldpnm == "" {
				fmt.Printf("%s is already a root namespace; unchanged", nnm)
			} else {
				hc.Spec.Parent = ""
				fmt.Printf("Unsetting the parent of %s (was previously %s)\n", nnm, oldpnm)
				numChanges++
			}
		}

		if flags.Changed("parent") {
			pnm, _ := flags.GetString("parent")
			if oldpnm == pnm {
				fmt.Printf("Parent of %s is already %s; unchanged\n", nnm, pnm)
			} else {
				hc.Spec.Parent = pnm
				if oldpnm == "" {
					fmt.Printf("Setting the parent of %s to %s\n", nnm, pnm)
				} else {
					fmt.Printf("Changing the parent of %s from %s to %s\n", nnm, oldpnm, pnm)
				}
				numChanges++
			}
		}

		if flags.Changed("requiredChild") {
			rcns, _ := flags.GetStringArray("requiredChild")

			for _, rcn := range rcns {
				if childNamespaceExists(hc, rcn) {
					fmt.Printf("Required child %s already present in %s\n", rcn, nnm)
					continue
				}
				hc.Spec.RequiredChildren = append(hc.Spec.RequiredChildren, rcn)
				fmt.Printf("Adding required child %s\n", rcn)
				numChanges++
			}
		}

		if flags.Changed("optionalChild") {
			cnm, _ := flags.GetString("optionalChild")
			found := false
			newRCs := []string{}
			for _, rc := range hc.Spec.RequiredChildren {
				if rc == cnm {
					found = true
					continue
				}
				newRCs = append(newRCs, rc)
			}

			if !found {
				fmt.Printf("%s is not a required child of %s\n", cnm, nnm)
			} else {
				fmt.Printf("Making required child %s optional\n", cnm)
				hc.Spec.RequiredChildren = newRCs
				numChanges++
			}
		}

		if numChanges > 0 {
			updateHierarchy(hc, fmt.Sprintf("setting hierarchical configuration of %s", nnm))
			word := "property"
			if numChanges > 1 {
				word = "properties"
			}
			fmt.Printf("Succesfully updated %d %s of the hierarchical configuration of %s\n", numChanges, word, nnm)
		} else {
			fmt.Printf("No changes made\n")
		}
	},
}

func newSetCmd() *cobra.Command {
	setCmd.Flags().Bool("root", false, "Turns namespace into root namespace")
	setCmd.Flags().String("parent", "", "Parent namespace")
	setCmd.Flags().StringArray("requiredChild", []string{""}, "Required Child namespace")
	setCmd.Flags().String("optionalChild", "", "Turns a required child namespace into an optional child namespace")
	return setCmd
}
