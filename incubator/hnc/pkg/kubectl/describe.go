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

var describeCmd = &cobra.Command{
	Use:   "describe NAMESPACE",
	Short: "Displays information about the hierarchy configuration",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		fmt.Printf("Hierarchy configuration for namespace %s\n", nnm)
		hier := client.getHierarchy(nnm)
		hnsnms := client.getHierarchicalNamespacesNames(nnm)

		// Parent
		if hier.Spec.Parent != "" {
			fmt.Printf("  Parent: %s\n", hier.Spec.Parent)
		} else {
			fmt.Printf("  No parent\n")
		}

		// Children
		childrenAndStatus := map[string]string{}
		for _, cn := range hier.Status.Children {
			childrenAndStatus[cn] = ""
		}
		for _, cn := range hnsnms {
			if _, ok := childrenAndStatus[cn]; ok {
				childrenAndStatus[cn] = "Owned"
			} else {
				childrenAndStatus[cn] = "Missing"
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

		// Early exit if no conditions
		if len(hier.Status.Conditions) == 0 {
			fmt.Printf("  No conditions\n")
			return
		}

		// Conditions
		fmt.Printf("  Conditions:\n")
		for _, c := range hier.Status.Conditions {
			fmt.Printf("  - %s: %s\n", c.Code, c.Msg)
			if len(c.Affects) == 0 {
				continue
			}
			fmt.Printf("    - Affected by this condition:\n")
			for _, a := range c.Affects {
				if a.Name != "" {
					if a.Group == "" {
						a.Group = "core"
					}
					fmt.Printf("      - %s/%s (%s/%s/%s\n", a.Namespace, a.Name, a.Group, a.Version, a.Kind)
				} else {
					fmt.Printf("      - %s\n", a.Namespace)
				}
			}
		}
	},
}

func newDescribeCmd() *cobra.Command {
	return describeCmd
}
