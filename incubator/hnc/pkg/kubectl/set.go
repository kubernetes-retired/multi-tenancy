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
	"strings"

	"github.com/spf13/cobra"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

//hcUpdates struct stores name of namespaces against type of flag passed
type hcUpdates struct {
	root                 bool
	parent               string
	allowCascadingDelete bool
}

var setCmd = &cobra.Command{
	Use:   "set NAMESPACE",
	Short: "Sets hierarchical properties of the given namespace",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		flags := cmd.Flags()
		parent, _ := flags.GetString("parent")
		allowCascadingDelete, _ := flags.GetBool("allowCascadingDelete")

		updates := hcUpdates{
			root:                 flags.Changed("root"),
			parent:               parent,
			allowCascadingDelete: allowCascadingDelete,
		}

		updateHC(client, updates, nnm)
	},
}

func updateHC(cl Client, updates hcUpdates, nnm string) {
	hc := cl.getHierarchy(nnm)
	oldpnm := hc.Spec.Parent
	oldacd := hc.Spec.AllowCascadingDelete
	numChanges := 0

	if updates.root && updates.parent != "" {
		fmt.Println("Cannot give the namespace a parent and make it a root at the same time")
		os.Exit(1)
	}

	if updates.root {
		setRoot(hc, oldpnm, nnm, &numChanges)
	}

	if updates.parent != "" {
		setParent(hc, oldpnm, updates.parent, nnm, &numChanges)
	}

	setAllowCascadingDelete(hc, nnm, oldacd, updates.allowCascadingDelete, &numChanges)

	if numChanges > 0 {
		cl.updateHierarchy(hc, fmt.Sprintf("update the hierarchical configuration of %s", nnm))
		word := "property"
		if numChanges > 1 {
			word = "properties"
		}
		fmt.Printf("Succesfully updated %d %s of the hierarchical configuration of %s\n", numChanges, word, nnm)
	} else {
		fmt.Printf("No changes made\n")
	}
}

func setRoot(hc *api.HierarchyConfiguration, oldpnm, nnm string, numChanges *int) {
	if oldpnm == "" {
		fmt.Printf("%s is already a root namespace; unchanged \n", nnm)
	} else {
		hc.Spec.Parent = ""
		fmt.Printf("Unsetting the parent of %s (was previously %s)\n", nnm, oldpnm)
		*numChanges++
	}
}

func setParent(hc *api.HierarchyConfiguration, oldpnm, pnm, nnm string, numChanges *int) {
	if oldpnm == pnm {
		fmt.Printf("Parent of %s is already %s; unchanged\n", nnm, pnm)
	} else {
		hc.Spec.Parent = pnm
		if oldpnm == "" {
			fmt.Printf("Setting the parent of %s to %s\n", nnm, pnm)
		} else {
			fmt.Printf("Changing the parent of %s from %s to %s\n", nnm, oldpnm, pnm)
		}
		*numChanges++
	}
}

func setAllowCascadingDelete(hc *api.HierarchyConfiguration, nnm string, oldacd, acd bool, numChanges *int) {
	if oldacd == acd {
		fmt.Printf("%s allowCascadingDelete is already %t; unchanged\n", nnm, acd)
	} else {
		hc.Spec.AllowCascadingDelete = acd
		fmt.Printf("Changing %s allowCascadingDelete to %t\n", nnm, acd)
		*numChanges++
	}
}

func normalizeStringArray(in []string) []string {
	out := []string{}
	for _, val := range in {
		for _, s := range strings.Split(val, ",") {
			out = append(out, s)
		}
	}
	return out
}

func newSetCmd() *cobra.Command {
	setCmd.Flags().Bool("root", false, "Turns namespace into root namespace")
	setCmd.Flags().String("parent", "", "Parent namespace")
	setCmd.Flags().Bool("allowCascadingDelete", false, "Allows cascading deletion of its self-serve subnamespaces.")
	return setCmd
}
