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

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var treeCmd = &cobra.Command{
	Use:   "tree",
	Short: "Displays the hierarchy tree rooted at the given namespace",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		hier := getHierarchy(nnm)
		fmt.Println(nnm)
		printSubtree("", hier)
	},
}

func printSubtree(prefix string, hier *tenancy.Hierarchy) {
	for i, c := range hier.Status.Children {
		if i < len(hier.Status.Children)-1 {
			fmt.Printf("%s├── %s\n", prefix, c)
			printSubtree(prefix+"|   ", getHierarchy(c))
		} else {
			fmt.Printf("%s└── %s\n", prefix, c)
			printSubtree(prefix+"    ", getHierarchy(c))
		}
	}
}

func init() {
	rootCmd.AddCommand(treeCmd)
}
