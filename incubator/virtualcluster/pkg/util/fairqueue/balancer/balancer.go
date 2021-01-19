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

package balancer

// Scheduler performs load balancer algorithm.
type Scheduler interface {
	// Next find the next selected item.
	Next() string
	// Add adds the new item to selection pool.
	Add(id string, weight int)
	// Remove remove an item from pool.
	Remove(id string)
	// Clear remove all of the items and reset the scheduler state.
	Clear()
}
