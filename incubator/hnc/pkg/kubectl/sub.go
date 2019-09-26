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

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
)

var subCmd = &cobra.Command{
	Use:     "subnamespace <parent> <child>: Creates a subnamespace",
	Aliases: []string{"sub"},
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		_ = getHierarchy(nnm) // errors if it doesn't exist
		snm := args[1]
		sns := &corev1.Namespace{}
		sns.Name = snm
		if _, err := k8sClient.CoreV1().Namespaces().Create(sns); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		shr := getHierarchy(snm)
		shr.Spec.Parent = nnm
		updateHierarchy(shr, fmt.Sprintf("setting parent of %s to %s", snm, nnm)) // TODO: clean up
		fmt.Printf("Created %s as subnamespace of %s\n", snm, nnm)
	},
}

func init() {
	rootCmd.AddCommand(subCmd)
}
