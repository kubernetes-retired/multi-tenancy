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
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/printers"
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "List the Multi-Tenancy Benchmarks",

	Run: func(cmd *cobra.Command, args []string) {
		if len(benchmarks) == 0 {
			fmt.Println("No Benchmarks to get.")
			os.Exit(1)
		}
		printBenchmarks()
	},
}

func printBenchmarks() {

	w := printers.GetNewTabWriter(os.Stdout)
	defer w.Flush()

	if err := printContextHeaders(w); err != nil {
		fmt.Println(err.Error())
	}

	for _, b := range benchmarks {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t\n",
			b.ID,
			b.Title,
			b.Category,
			b.BenchmarkType,
			b.ProfileLevel); err != nil {
			fmt.Println(err.Error())
		}
	}
}

func printContextHeaders(out io.Writer) error {
	columnNames := []string{"ID", "NAME", "CATEGORY", "TYPE", "PROFILE LEVEL"}

	_, err := fmt.Fprintf(out, "%s\n", strings.Join(columnNames, "\t"))
	return err
}

func newGetCmd() *cobra.Command {
	return getCmd
}
