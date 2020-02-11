/*
Copyright 2019 The Kubernetes Authors.

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
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"

	"k8s.io/apiserver/pkg/util/term"
	"k8s.io/client-go/tools/leaderelection"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"

	"k8s.io/klog"

	"github.com/spf13/cobra"

	syncerconfig "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/cmd/syncer/app/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/cmd/syncer/app/options"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer"
)

func NewSyncerCommand(stopChan <-chan struct{}) *cobra.Command {
	s, err := options.NewResourceSyncerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	cmd := &cobra.Command{
		Use: "syncer",
		Long: `The resource syncer is a daemon that watches tenant masters to
keep tenant requests are synchronized to super master by creating corresponding
custom resources on behalf of the tenant users in super master, following the
resource isolation policy specified in Tenant CRD.`,
		Run: func(cmd *cobra.Command, args []string) {
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

func Run(cc *syncerconfig.CompletedConfig, stopCh <-chan struct{}) error {
	ss := syncer.New(&cc.ComponentConfig,
		cc.SecretClient,
		cc.VirtualClusterInformer,
		cc.SuperMasterClient,
		cc.SuperMasterInformerFactory)

	// Prepare the event broadcaster.
	if cc.Broadcaster != nil && cc.SuperMasterClient != nil {
		cc.Broadcaster.StartRecordingToSink(stopCh)
	}

	// Start all informers.
	go cc.VirtualClusterInformer.Informer().Run(stopCh)
	cc.SuperMasterInformerFactory.Start(stopCh)

	// Wait for all caches to sync before resource sync.
	cc.SuperMasterInformerFactory.WaitForCacheSync(stopCh)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	// Prepare a reusable runCommand function.
	run := startSyncer(ctx, ss, cc, stopCh)

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

	// Leader election is disabled, so runCommand inline until done.
	run(ctx)
	return fmt.Errorf("finished without leader elect")
}

func startSyncer(ctx context.Context, s syncer.Bootstrap, cc *syncerconfig.CompletedConfig, stopCh <-chan struct{}) func(context.Context) {
	return func(ctx context.Context) {
		s.Run(stopCh)
		go func() {
			s.ListenAndServe(net.JoinHostPort(cc.Address, cc.Port), cc.CertFile, cc.KeyFile)
		}()
		go func() {
			// start a pprof http server
			klog.Fatal(http.ListenAndServe(":6060", nil))
		}()
		<-ctx.Done()
	}
}
