/*
Copyright 2021 The Kubernetes Authors.

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

// package runnable enables to you to extend the syncer as a framework with
// embedded blocking packages like http servers.
package runnable

import (
	"sync"
)

var (
	SyncerRunnableRegister RunnableRegister
)

// Runnable exposes an interface that allows you to start any additional
// "servers", this means you can extend the syncer to support standalone
// http servers, implement webhook backends, etc.
type Runnable interface {
	// Start starts running the component.  The component will stop running
	// when the channel is closed.
	Start(stopCh <-chan struct{}) error
}

// RunnableRegister holds all the runnable types for running
type RunnableRegister struct {
	sync.RWMutex
	runnable []Runnable
}

// Add will add a new runnable server
func (reg *RunnableRegister) Add(r Runnable) {
	reg.Lock()
	defer reg.Unlock()
	reg.runnable = append(reg.runnable, r)
}

// List returns the list of registered runnables for initialization.
func (reg *RunnableRegister) List() []Runnable {
	reg.RLock()
	defer reg.RUnlock()
	return reg.runnable
}
