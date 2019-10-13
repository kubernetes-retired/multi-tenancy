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
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var (
	footnotesByMsg map[string]int
	footnotes      []string
)

var treeCmd = &cobra.Command{
	Use:   "tree",
	Short: "Displays the hierarchy tree rooted at the given namespace",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		footnotesByMsg = map[string]int{}
		footnotes = []string{}
		hier := getHierarchy(nnm)
		fmt.Println(nameAndFootnotes(hier, nnm))
		printSubtree("", hier)

		if len(footnotes) > 0 {
			fmt.Printf("\nConditions:\n")
			for i, n := range footnotes {
				fmt.Printf("%d) %s\n", i+1, n)
			}
		}
	},
}

func printSubtree(prefix string, hier *tenancy.HierarchyConfiguration) {
	if hier == nil {
		return
	}
	for i, cn := range hier.Status.Children {
		ch := getHierarchy(cn)
		tx := nameAndFootnotes(ch, cn)
		if i < len(hier.Status.Children)-1 {
			fmt.Printf("%s├── %s\n", prefix, tx)
			printSubtree(prefix+"|   ", ch)
		} else {
			fmt.Printf("%s└── %s\n", prefix, tx)
			printSubtree(prefix+"    ", ch)
		}
	}
}

func nameAndFootnotes(hier *tenancy.HierarchyConfiguration, nnm string) string {
	if hier == nil {
		return fmt.Sprintf("%s (not found on apiserver)", nnm)
	}
	notes := []int{}
	for _, cond := range hier.Status.Conditions {
		txt := cond.Msg
		if idx, ok := footnotesByMsg[txt]; ok {
			notes = append(notes, idx)
		} else {
			footnotes = append(footnotes, txt)
			footnotesByMsg[cond.Msg] = len(footnotes)
			notes = append(notes, len(footnotes))
		}
	}

	if len(notes) == 0 {
		return hier.Namespace
	}
	sort.Ints(notes)
	ns := []string{}
	for _, n := range notes {
		ns = append(ns, strconv.Itoa(n))
	}
	return fmt.Sprintf("%s (%s)", hier.Namespace, strings.Join(ns, ","))
}

func init() {
	rootCmd.AddCommand(treeCmd)
}
