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
	"strings"

	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Displays information about the hierarchy",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		hier := getHierarchy(nnm)
		fmt.Printf("Hierarchy for namespace %s\n", nnm)
		if hier.Spec.Parent != "" {
			fmt.Printf("  Parent: %s\n", hier.Spec.Parent)
		} else {
			fmt.Printf("  No parent\n")
		}

		childrenAndStatus := map[string]string{}
		for _, cn := range hier.Status.Children {
			childrenAndStatus[cn] = ""
		}
		for _, cn := range hier.Spec.RequiredChildren {
			if _, ok := childrenAndStatus[cn]; ok {
				childrenAndStatus[cn] = "required"
			} else {
				childrenAndStatus[cn] = "MISSING"
			}
		}
		if len(childrenAndStatus) > 0 {
			children := []string{}
			for cn, status := range childrenAndStatus {
				if status == "" {
					children = append(children, cn)
				} else {
					children = append(children, fmt.Sprintf("%s (%s)", cn, status))
				}
			}
			sort.Strings(children)
			fmt.Printf("  Children:\n  - %s\n", strings.Join(children, "\n  - "))
		} else {
			fmt.Printf("  No children\n")
		}
		if hier.Spec.Parent != "" || len(hier.Status.Children) > 0 {
			if len(hier.Status.Conditions) > 0 {
				fmt.Printf("  Conditions:\n")
				for _, c := range hier.Status.Conditions {
					fmt.Printf("  - %v\n", c)
				}
			} else {
				fmt.Printf("  No conditions\n")
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(showCmd)
}
