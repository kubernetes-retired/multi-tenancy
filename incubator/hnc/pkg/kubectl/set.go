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
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

//flagValues struct stores name of namespaces against type of flag passed
type flagValues struct {
	root                bool
	parentString        string
	requiredChildArray  []string
	optionalChildString string
}

var setCmd = &cobra.Command{
	Use:     "set",
	Short:   "Allows setting namespace hierarchy",
	Aliases: []string{"set"},
	Args:    cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		flags := cmd.Flags()
		parentString, _ := flags.GetString("parent")
		requiredChildArray, _ := flags.GetStringArray("requiredChild")
		optionalChildString, _ := flags.GetString("optionalChild")

		flagValues := flagValues{
			root:                flags.Changed("root"),
			parentString:        parentString,
			requiredChildArray:  requiredChildArray,
			optionalChildString: optionalChildString,
		}

		msgs, err := setCmdFunc(client, flagValues, nnm)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}

		for _, each := range msgs {
			fmt.Print(each)
		}

	},
}

func setCmdFunc(cl Client, flagValues flagValues, nnm string) ([]string, error) {
	msg := make([]string, 0)
	hc := cl.getHierarchy(nnm)
	oldpnm := hc.Spec.Parent
	numChanges := 0

	if flagValues.root && flagValues.parentString != "" {
		return msg, errors.New("Cannot set both --root and --parent at the same time \n")
	}

	if flagValues.root {
		setRoot(hc, oldpnm, nnm, &msg, &numChanges)
	}

	if flagValues.parentString != "" {
		setParent(hc, oldpnm, flagValues.parentString, nnm, &msg, &numChanges)
	}

	if flagValues.requiredChildArray != nil {
		setRequiredChild(hc, flagValues.requiredChildArray, nnm, &msg, &numChanges)
	}

	if flagValues.optionalChildString != "" {
		setOptionalChild(hc, flagValues.optionalChildString, nnm, &msg, &numChanges)
	}

	if numChanges > 0 {
		cl.updateHierarchy(hc, fmt.Sprintf("setting hierarchical configuration of %s", nnm))
		word := "property"
		if numChanges > 1 {
			word = "properties"
		}
		msg = append(msg, fmt.Sprintf("Succesfully updated %d %s of the hierarchical configuration of %s\n", numChanges, word, nnm))
	} else {
		msg = append(msg, fmt.Sprintf("No changes made\n"))
	}
	return msg, nil
}

func setRoot(hc *api.HierarchyConfiguration, oldpnm, nnm string, msg *[]string, numChanges *int) {
	if oldpnm == "" {
		*msg = append(*msg, fmt.Sprintf("%s is already a root namespace; unchanged \n", nnm))
	} else {
		hc.Spec.Parent = ""
		*msg = append(*msg, fmt.Sprintf("Unsetting the parent of %s (was previously %s)\n", nnm, oldpnm))
		*numChanges++
	}
}

func setParent(hc *api.HierarchyConfiguration, oldpnm, pnm, nnm string, msg *[]string, numChanges *int) {
	if oldpnm == pnm {
		*msg = append(*msg, fmt.Sprintf("Parent of %s is already %s; unchanged\n", nnm, pnm))
	} else {
		hc.Spec.Parent = pnm
		if oldpnm == "" {
			*msg = append(*msg, fmt.Sprintf("Setting the parent of %s to %s\n", nnm, pnm))
		} else {
			*msg = append(*msg, fmt.Sprintf("Changing the parent of %s from %s to %s\n", nnm, oldpnm, pnm))
		}
		*numChanges++
	}
}

func setRequiredChild(hc *api.HierarchyConfiguration, rcns []string, nnm string, msg *[]string, numChanges *int) {
	for _, rcn := range rcns {
		if childNamespaceExists(hc, rcn) {
			*msg = append(*msg, fmt.Sprintf("Required child %s already present in %s\n", rcn, nnm))
			continue
		}
		hc.Spec.RequiredChildren = append(hc.Spec.RequiredChildren, rcn)
		*msg = append(*msg, fmt.Sprintf("Adding required child %s\n", rcn))
		*numChanges++
	}
}

func setOptionalChild(hc *api.HierarchyConfiguration, cnm, nnm string, msg *[]string, numChanges *int) {
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
		*msg = append(*msg, fmt.Sprintf("%s is not a required child of %s\n", cnm, nnm))
	} else {
		*msg = append(*msg, fmt.Sprintf("Making required child %s optional\n", cnm))
		hc.Spec.RequiredChildren = newRCs
		*numChanges++
	}
}

func newSetCmd() *cobra.Command {
	setCmd.Flags().Bool("root", false, "Turns namespace into root namespace")
	setCmd.Flags().String("parent", "", "Parent namespace")
	setCmd.Flags().StringArray("requiredChild", []string{""}, "Required Child namespace")
	setCmd.Flags().String("optionalChild", "", "Turns a required child namespace into an optional child namespace")
	return setCmd
}