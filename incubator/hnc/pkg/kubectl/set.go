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
	"sort"
	"strings"

	"github.com/spf13/cobra"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

//hcUpdates struct stores name of namespaces against type of flag passed
type hcUpdates struct {
	root             bool
	parent           string
	requiredChildren []string
	optionalChildren []string
}

var setCmd = &cobra.Command{
	Use:   "set <namespace>",
	Short: "Sets hierarchical properties of the given namespace",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		flags := cmd.Flags()
		parent, _ := flags.GetString("parent")
		requiredChildren, _ := flags.GetStringArray("requiredChild")
		optionalChildren, _ := flags.GetStringArray("optionalChild")
		requiredChildren = normalizeStringArray(requiredChildren)
		optionalChildren = normalizeStringArray(optionalChildren)

		updates := hcUpdates{
			root:             flags.Changed("root"),
			parent:           parent,
			requiredChildren: requiredChildren,
			optionalChildren: optionalChildren,
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

	if len(updates.requiredChildren) != 0 {
		setRequiredChildren(hc, updates.requiredChildren, nnm, &numChanges)
	}

	if len(updates.optionalChildren) != 0 {
		setOptionalChildren(hc, updates.optionalChildren, nnm, &numChanges)
	}

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

func setRequiredChildren(hc *api.HierarchyConfiguration, rcns []string, nnm string, numChanges *int) {
	for _, rcn := range rcns {
		if childNamespaceExists(hc, rcn) {
			fmt.Printf("Required child %s already present in %s\n", rcn, nnm)
			continue
		}
		hc.Spec.RequiredChildren = append(hc.Spec.RequiredChildren, rcn)
		fmt.Printf("Adding required child (subnamespace) %s to %s\n", rcn, nnm)
		*numChanges++
	}
	sort.Strings(hc.Spec.RequiredChildren)
}

func setOptionalChildren(hc *api.HierarchyConfiguration, ocns []string, nnm string, numChanges *int) {
	existingRCs := map[string]bool{}
	for _, rc := range hc.Spec.RequiredChildren {
		existingRCs[rc] = true
	}

	for _, oc := range ocns {
		if existingRCs[oc] {
			fmt.Printf("Making %s a regular child of %s\n", oc, nnm)
			delete(existingRCs, oc)
			*numChanges++
		} else {
			fmt.Printf("%s is not a required child of %s\n", oc, nnm)
		}
	}

	newRCs := []string{}
	for k, _ := range existingRCs {
		newRCs = append(newRCs, k)
	}
	if len(newRCs) == 0 {
		hc.Spec.RequiredChildren = nil
	} else {
		sort.Strings(newRCs)
		hc.Spec.RequiredChildren = newRCs
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
	setCmd.Flags().StringArray("requiredChild", []string{""}, "Specifies a required child (subnamespace). If the child does not exist, it will be created. Multiple values may be comma delimited.")
	setCmd.Flags().StringArray("optionalChild", []string{""}, "Turns a required child namespaces into optional child namespaces. Multiple values may be comma delimited.")
	return setCmd
}
