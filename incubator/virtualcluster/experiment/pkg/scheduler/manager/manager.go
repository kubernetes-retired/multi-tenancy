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
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

// WatchManager manages number of resource watchers.
type WatchManager struct {
	resourceWatchers map[ResourceWatcher]struct{}
	listeners        []listener.ClusterChangeListener
}

func New() *WatchManager {
	return &WatchManager{resourceWatchers: make(map[ResourceWatcher]struct{})}
}

// ResourceWatcher is the interface used by WatchManager to manage multiple resource watchers.
type ResourceWatcher interface {
	reconciler.DWReconciler
	GetMCController() *mc.MultiClusterController
	GetListener() listener.ClusterChangeListener
	Start(stopCh <-chan struct{}) error
}

type ResourceWatcherNew func(*schedulerconfig.SchedulerConfiguration) (ResourceWatcher, error)

// AddResourceWatcher adds a resource watcher to the WatchManager.
func (m *WatchManager) AddResourceWatcher(s ResourceWatcher) {
	m.resourceWatchers[s] = struct{}{}
	l := s.GetListener()
	if l == nil {
		panic("resource watcher should provide listener")
	}
	m.listeners = append(m.listeners, l)
}

func (m *WatchManager) GetListeners() []listener.ClusterChangeListener {
	return m.listeners
}

func (m *WatchManager) GetResourceWatcherByMCControllerName(name string) ResourceWatcher {
	for s := range m.resourceWatchers {
		if s.GetMCController().GetControllerName() == name {
			return s
		}
	}
	return nil
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
