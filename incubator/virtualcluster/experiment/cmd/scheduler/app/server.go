/*
Copyright 2020 The Kubernetes Authors.

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

package app

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apiserver/pkg/util/term"
	"k8s.io/client-go/tools/leaderelection"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/klog"

	schedulerappconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/cmd/scheduler/app/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/cmd/scheduler/app/options"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/version/verflag"
)

func NewSchedulerCommand(stopChan <-chan struct{}) *cobra.Command {
	s, err := options.NewSchedulerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	cmd := &cobra.Command{
		Use:  "scheduler",
		Long: `The scheduler handles tenant namespace scheduling over super clusters.`,
		Run: func(cmd *cobra.Command, args []string) {
			verflag.PrintAndExitIfRequested()
			c, err := s.Config()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}

			if err := Run(c.Complete(), stopChan); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		},
	}

	fs := cmd.Flags()
	namedFlagSets := s.Flags()
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name())

	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}
	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), namedFlagSets, cols)
		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStdout(), namedFlagSets, cols)
	})

	return cmd
}

// Run start the scheduler.
func Run(cc *schedulerappconfig.CompletedConfig, stopCh <-chan struct{}) error {
	scheduler := scheduler.New(&cc.ComponentConfig,
		cc.VirtualClusterClient,
		cc.VirtualClusterInformer,
		cc.SuperClusterClient,
		cc.SuperClusterInformer,
		cc.MetaClusterClient,
		cc.MetaClusterInformerFactory,
		cc.Recorder)

	// Start all informers.
	go cc.VirtualClusterInformer.Informer().Run(stopCh)
	go cc.SuperClusterInformer.Informer().Run(stopCh)

	cc.MetaClusterInformerFactory.Start(stopCh)
	// Wait for all caches to sync before resource sync.
	cc.MetaClusterInformerFactory.WaitForCacheSync(stopCh)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Prepare a reusable runCommand function.
	run := startScheduler(ctx, scheduler, stopCh)

	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	if cc.LeaderElection != nil {
		cc.LeaderElection.Callbacks = leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				klog.Fatalf("leaderelection lost")
			},
		}
		leaderElector, err := leaderelection.NewLeaderElector(*cc.LeaderElection)
		if err != nil {
			return fmt.Errorf("couldn't create leader elector: %v", err)
		}

		leaderElector.Run(ctx)
		return fmt.Errorf("lost lease")
	}

	run(ctx)
	return fmt.Errorf("finished without leader elect")
}

func startScheduler(ctx context.Context, s *scheduler.Scheduler, stopCh <-chan struct{}) func(context.Context) {
	return func(ctx context.Context) {
		s.Run(stopCh)
		<-ctx.Done()
	}
}
