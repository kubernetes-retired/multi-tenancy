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

package manager

import (
	"sync"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
)

// ControllerManager manages number of resource syncers. It starts their caches, waits for those to sync,
// then starts the controllers.
type ControllerManager struct {
	resourceSyncers map[ResourceSyncer]struct{}
}

type ResourceSyncerOptions struct {
	MCOptions     *mc.Options
	UWOptions     *uw.Options
	PatrolOptions *pa.Options
	IsFake        bool
}

func New() *ControllerManager {
	return &ControllerManager{resourceSyncers: make(map[ResourceSyncer]struct{})}
}

// ResourceSyncer is the interface used by ControllerManager to manage multiple resource syncers.
type ResourceSyncer interface {
	listener.ClusterChangeListener
	StartUWS(stopCh <-chan struct{}) error
	StartDWS(stopCh <-chan struct{}) error
	StartPatrol(stopCh <-chan struct{}) error
	Reconcile(request reconciler.Request) (reconciler.Result, error)
	BackPopulate(string) error
	PatrollerDo()
}

// AddController adds a resource syncer to the ControllerManager.
func (m *ControllerManager) AddResourceSyncer(s ResourceSyncer) {
	m.resourceSyncers[s] = struct{}{}
	listener.AddListener(s)
}

// Start gets all the unique caches of the controllers it manages, starts them,
// then starts the controllers as soon as their respective caches are synced.
// Start blocks until an error or stop is received.
func (m *ControllerManager) Start(stop <-chan struct{}) error {
	errCh := make(chan error)

	wg := &sync.WaitGroup{}
	wg.Add(len(m.resourceSyncers) * 3)

	for s := range m.resourceSyncers {
		go func(s ResourceSyncer) {
			defer wg.Done()
			if err := s.StartDWS(stop); err != nil {
				errCh <- err
			}
		}(s)
		// start UWS syncer
		go func(s ResourceSyncer) {
			defer wg.Done()
			if err := s.StartUWS(stop); err != nil {
				errCh <- err
			}
		}(s)
		// start periodic checker
		go func(s ResourceSyncer) {
			defer wg.Done()
			if err := s.StartPatrol(stop); err != nil {
				errCh <- err
			}
		}(s)
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		return nil
	case <-stop:
		return nil
	case err := <-errCh:
		return err
	}
}
