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

	schedulerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
)

// WatchManager manages number of resource watchers.
type WatchManager struct {
	resourceWatchers map[ResourceWatcher]struct{}
}

func New() *WatchManager {
	return &WatchManager{resourceWatchers: make(map[ResourceWatcher]struct{})}
}

// ResourceWatcher is the interface used by WatchManager to manage multiple resource watchers.
type ResourceWatcher interface {
	listener.ClusterChangeListener
	Start(stopCh <-chan struct{}) error
}

type ResourceWatcherNew func(*schedulerconfig.SchedulerConfiguration) (ResourceWatcher, error)

// AddResourceWatcher adds a resource watcher to the WatchManager.
func (m *WatchManager) AddResourceWatcher(s ResourceWatcher) {
	m.resourceWatchers[s] = struct{}{}
	listener.AddListener(s)
}

func (m *WatchManager) Start(stop <-chan struct{}) error {
	errCh := make(chan error)

	wg := &sync.WaitGroup{}
	wg.Add(len(m.resourceWatchers))

	for s := range m.resourceWatchers {
		go func(s ResourceWatcher) {
			defer wg.Done()
			if err := s.Start(stop); err != nil {
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
