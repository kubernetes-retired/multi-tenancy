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

package engine

import (
	"sync"

	internalcache "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/cache"
)

type Engine interface {
	ScheduleNamespace(*internalcache.Namespace) (*internalcache.Namespace, error)
	ReScheduleNamespace(*internalcache.Namespace) (*internalcache.Namespace, error)
	UnReserveNamespace(key string) error
	RollBackNamespace(*internalcache.Namespace) error
}

var _ Engine = &schedulerEngine{}

type schedulerEngine struct {
	mu sync.RWMutex

	cache internalcache.Cache
}

func NewSchedulerEngine(schedulerCache internalcache.Cache) Engine {
	return &schedulerEngine{cache: schedulerCache}
}

func (e *schedulerEngine) ScheduleNamespace(namespace *internalcache.Namespace) (*internalcache.Namespace, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return nil, nil
}

func (e *schedulerEngine) ReScheduleNamespace(namespace *internalcache.Namespace) (*internalcache.Namespace, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return nil, nil
}

func (e *schedulerEngine) UnReserveNamespace(key string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return nil
}

func (e *schedulerEngine) RollBackNamespace(namespace *internalcache.Namespace) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return nil
}
