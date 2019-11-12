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

package manager

import "sync"

// ControllerManager manages number of controllers. It starts their caches, waits for those to sync,
// then starts the controllers.
// A ControllerManager is required to start controllers.
type ControllerManager struct {
	controllers map[Controller]struct{}
}

// New creates a Manager.
func New() *ControllerManager {
	return &ControllerManager{controllers: make(map[Controller]struct{})}
}

// Controller is the interface used by ControllerManager to start the controllers and get their caches (beforehand).
type Controller interface {
	Start(stop <-chan struct{}) error
}

// AddController adds a controller to the ControllerManager.
func (m *ControllerManager) AddController(c Controller) {
	m.controllers[c] = struct{}{}
}

// Start gets all the unique caches of the controllers it manages, starts them,
// then starts the controllers as soon as their respective caches are synced.
// Start blocks until an error or stop is received.
func (m *ControllerManager) Start(stop <-chan struct{}) error {
	errCh := make(chan error)

	wg := &sync.WaitGroup{}
	wg.Add(len(m.controllers))

	for co := range m.controllers {
		go func(co Controller) {
			defer wg.Done()
			if err := co.Start(stop); err != nil {
				errCh <- err
			}
		}(co)
	}

	wg.Wait()

	select {
	case <-stop:
		return nil
	case err := <-errCh:
		return err
	}
}
