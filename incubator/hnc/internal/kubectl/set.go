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

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
)

//hcUpdates struct stores name of namespaces against type of flag passed
type hcUpdates struct {
	root     bool
	parent   string
	allowCD  bool
	forbidCD bool
}

var setCmd = &cobra.Command{
	Use:   "set NAMESPACE",
	Short: "Sets hierarchical properties of the given namespace",
	Example: `	# Make 'foo' the parent of 'bar'
	kubectl hns set bar --parent foo
	kubectl hns set bar -p foo

	# Make 'foo' a root (remove 'bar' as its parent)
	kubectl hns set bar --root
	kubectl hns set bar -r

	# Not allowed: give 'bar' a parent and make it a root at the same time
	kubectl hns set bar --root --parent foo # error

	# Allow 'foo', or any of its descendants, to be cascading deleted
	kubectl hns set foo --allowCascadingDelete
	kubectl hns set foo -a

	# Forbids cascading deletion on 'foo' and its subtree (unless specifically
	# allowed on any descendants).
	kubectl hns set foo --forbidCascadingDelete
	kubectl hns set foo -f`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		flags := cmd.Flags()
		parent, _ := flags.GetString("parent")
		allowCD, _ := flags.GetBool("allowCascadingDelete")
		forbidCD, _ := flags.GetBool("forbidCascadingDelete")

		updates := hcUpdates{
			root:     flags.Changed("root"),
			parent:   parent,
			allowCD:  allowCD,
			forbidCD: forbidCD,
		}

		updateHC(client, updates, nnm)
	},
}

func updateHC(cl Client, updates hcUpdates, nnm string) {
	hc := cl.getHierarchy(nnm)
	oldpnm := hc.Spec.Parent
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

	setAllowCascadingDelete(hc, nnm, updates.allowCD, updates.forbidCD, &numChanges)

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

func setAllowCascadingDelete(hc *api.HierarchyConfiguration, nnm string, allow, forbid bool, numChanges *int) {
	if allow && forbid {
		fmt.Printf("Cannot set both --allowCascadingDelete and --forbidCascadingDelete\n")
		os.Exit(1)
	}

	if !allow && !forbid {
		// nothing specified
		return
	}

	// We now know that allow != forbid, so we can just look at allow
	if hc.Spec.AllowCascadingDelete == allow {
		fmt.Printf("Cascading deletion for '%s' is already set to %t; unchanged\n", nnm, allow)
	} else {
		hc.Spec.AllowCascadingDelete = allow
		if allow {
			fmt.Printf("Allowing cascading deletion on '%s'\n", nnm)
		} else {
			fmt.Printf("Forbidding cascading deletion on '%s'\n", nnm)
		}
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
	setCmd.Flags().BoolP("root", "r", false, "Removes the current parent namespace, making this namespace a root")
	setCmd.Flags().StringP("parent", "p", "", "Sets the parent namespace")
	setCmd.Flags().BoolP("allowCascadingDelete", "a", false, "Allows cascading deletion of its subnamespaces.")
	setCmd.Flags().BoolP("forbidCascadingDelete", "f", false, "Protects cascading deletion of its subnamespaces.")
	return setCmd
}
