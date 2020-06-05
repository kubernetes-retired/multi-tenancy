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
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var (
	footnotesByMsg map[string]int
	footnotes      []string
)

var treeCmd = &cobra.Command{
	Use:   "tree TREE",
	Short: "Display one or more hierarchy trees",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		footnotesByMsg = map[string]int{}
		footnotes = []string{}
		defaultList := len(args) == 0
		nsList := args
		if flags.Changed("all-namespaces") {
			nsList = getAllNamespaces()
		}
		if defaultList && !flags.Changed("all-namespaces") {
			fmt.Printf("Error: Must specify the root of the tree(s) to display or else specify --all-namespaces\n")
			os.Exit(1)
		}
		for _, nnm := range nsList {
			hier := client.getHierarchy(nnm)
			// If we're showing the default list, skip all non-root namespaces since they'll be displayed
			// as part of another namespace's tree. Get the text first though because that will reveal if
			// the tree's in a cycle, in which case, even though the NS isn't a root, we'll display it
			// anyway.
			txt, cycle := nameAndFootnotes(hier)
			if defaultList && (!cycle && hier.Spec.Parent != "") {
				continue
			}
			fmt.Println(txt)
			printSubtree("", hier, cycle)
		}
		if len(footnotes) > 0 {
			fmt.Printf("\nConditions:\n")

			for i, n := range footnotes {
				fmt.Printf("%d) %s\n", i+1, n)
			}
		}
	},
}

func printSubtree(prefix string, hier *api.HierarchyConfiguration, inCycle bool) {
	for i, cn := range hier.Status.Children {
		ch := client.getHierarchy(cn)
		txt, cycle := nameAndFootnotes(ch)
		if cycle && inCycle {
			continue
		}
		if i < len(hier.Status.Children)-1 {
			fmt.Printf("%s├── %s\n", prefix, txt)
			printSubtree(prefix+"│   ", ch, cycle)
		} else {
			fmt.Printf("%s└── %s\n", prefix, txt)
			printSubtree(prefix+"    ", ch, cycle)
		}
	}
}

// nameAndFootnotes returns the text to print to describe the namespace, in the form of the
// namespace's name along with references to any footnotes. Example: default (1).
//
// The second param is true if this namespace is part of a cycle, in which case we shouldn't
// recurse.
func nameAndFootnotes(hier *api.HierarchyConfiguration) (string, bool) {
	cycle := false
	notes := []int{}
	for _, cond := range hier.Status.Conditions {
		if cond.Code == api.CritCycle {
			cycle = true
		}
		txt := (string)(cond.Code) + ": " + cond.Msg
		if idx, ok := footnotesByMsg[txt]; ok {
			notes = append(notes, idx)
		} else {
			footnotes = append(footnotes, txt)
			footnotesByMsg[txt] = len(footnotes)
			notes = append(notes, len(footnotes))
		}
	}

	if len(notes) == 0 {
		return hier.Namespace, false
	}
	sort.Ints(notes)
	ns := []string{}
	for _, n := range notes {
		ns = append(ns, strconv.Itoa(n))
	}
	return fmt.Sprintf("%s (%s)", hier.Namespace, strings.Join(ns, ",")), cycle
}

func newTreeCmd() *cobra.Command {
	treeCmd.Flags().BoolP("all-namespaces", "A", false, "Displays all trees on the cluster")
	return treeCmd
}

func getAllNamespaces() []string {
	nsList, err := k8sClient.CoreV1().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Could not list namespaces: %s\n", err)
		os.Exit(1)
	}
	result := []string{}
	for _, each := range nsList.Items {
		result = append(result, each.Name)
	}
	sort.Strings(result)
	return result
}
