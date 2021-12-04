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
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/printers"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var getCmd = &cobra.Command{
	Use:   "get [benchmark|benchmarks] [<benchmark ID>]",
	Short: "display one or many benchmarks.",

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := getResource(args); err != nil {
			return err
		}

		if err := filterBenchmarks(cmd, args); err != nil {
			return err
		}
		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(printBenchmarks())
	},
}

func printBenchmarks() error {

	errs := []error{}

	if len(benchmarks) == 0 {
		return fmt.Errorf("No benchmarks found")
	}

	w := printers.GetNewTabWriter(os.Stdout)
	defer w.Flush()

	if err := printContextHeaders(w); err != nil {
		return err
	}

	for _, b := range benchmarks {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t\n",
			b.ID,
			b.Title,
			b.Category,
			b.BenchmarkType,
			b.ProfileLevel); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

func printContextHeaders(out io.Writer) error {
	columnNames := []string{"ID", "NAME", "CATEGORY", "TYPE", "PROFILE LEVEL"}

	_, err := fmt.Fprintf(out, "%s\n", strings.Join(columnNames, "\t"))
	return err
}

func newGetCmd() *cobra.Command {
	return getCmd
}
