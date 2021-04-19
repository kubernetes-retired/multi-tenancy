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
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

var describeCmd = &cobra.Command{
	Use:   "describe NAMESPACE",
	Short: "Displays information about the hierarchy configuration",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nnm := args[0]
		fmt.Printf("Hierarchy configuration for namespace %s\n", nnm)
		hier := client.getHierarchy(nnm)
		as := client.getAnchorStatus(nnm)

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
		for cn, st := range as {
			if _, ok := childrenAndStatus[cn]; ok {
				childrenAndStatus[cn] = "subnamespace"
			} else {
				childrenAndStatus[cn] = st + " subnamespace"
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

		// Conditions
		describeConditions(hier.Status.Conditions)

		// Events
		describeEvents(nnm)
	},
}

func describeConditions(cond []api.Condition) {
	if len(cond) == 0 {
		fmt.Printf("  No conditions\n")
		return
	}
	fmt.Printf("  Conditions:\n")
	for _, c := range cond {
		fmt.Printf("  - %s (%s): %s\n", c.Type, c.Reason, c.Message)
	}
}

func describeEvents(nnm string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events, _ := k8sClient.CoreV1().Events(nnm).List(ctx, metav1.ListOptions{})
	// filter out HNC events only
	hncEvents := []v1.Event{}
	for _, event := range events.Items {
		if event.Source.Component == "hnc.x-k8s.io" {
			hncEvents = append(hncEvents, event)
		}
	}
	if len(hncEvents) == 0 {
		fmt.Printf("\nNo recent HNC events for objects in this namespace\n")
		return
	}
	fmt.Printf("\nEvents from the objects in namespace %s\n", nnm)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
	fmt.Fprintln(w, "Last Seen\tReason\tObject\tMessage")
	set := make(map[string]bool)
	// sort the events by time so that we always show the latest if there's duplicate events
	sort.Slice(hncEvents, func(i, j int) bool {
		return hncEvents[i].LastTimestamp.Time.After(hncEvents[j].LastTimestamp.Time)
	})
	for _, event := range hncEvents {
		obj := event.InvolvedObject.Kind + ":" + event.InvolvedObject.Namespace + "/" + event.InvolvedObject.Name
		if set[event.Reason+obj] {
			continue
		}
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\n",
			duration.HumanDuration(time.Since(event.LastTimestamp.Time)),
			event.Reason,
			obj,
			strings.TrimSpace(event.Message),
		)
		set[event.Reason+obj] = true
	}
	w.Flush()
}

func newDescribeCmd() *cobra.Command {
	return describeCmd
}
