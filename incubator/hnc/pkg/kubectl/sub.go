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
)

var subCmd = &cobra.Command{
	Use:     "subnamespace <parent> <child>: Creates a subnamespace",
	Aliases: []string{"sub"},
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		cnm := args[1]
		ns := getHierarchy(nnm)                                          // errors if it doesn't exist
		ns.Spec.RequiredChildren = append(ns.Spec.RequiredChildren, cnm) // TODO: dedup
		updateHierarchy(ns, fmt.Sprintf("adding required child %s", cnm))
		fmt.Printf("Added required child %s\n", cnm)
	},
}

func init() {
	rootCmd.AddCommand(subCmd)
}
