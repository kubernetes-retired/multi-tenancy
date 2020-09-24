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

package fairqueue

import (
	"sync"

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/fairqueue/balancer"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/fairqueue/balancer/weightedroundrobin"
)

type Item interface {
	Group() string
}

type fairQueue struct {
	// balancer choose which queue to go.
	balancer balancer.Scheduler
	// queueGroup group each queue by a unique key.
	// each queue defines the order in which we will work on items. Every
	// element of queue should be in the dirty set and not in the
	// processing set.
	queueGroup map[string][]t

	// dirty defines all of the items that need to be processed.
	dirty set

	// Things that are currently being processed are in the processing set.
	// These things may be simultaneously in the dirty set. When we finish
	// processing something and remove it from this set, we'll check if
	// it's in the dirty set, and if so, add it to the queue.
	processing set

	cond *sync.Cond

	shuttingDown bool

	rateLimiter workqueue.RateLimiter

	// clock tracks time for delayed firing
	clock clock.Clock

	// stopCh lets us signal a shutdown to the waiting loop
	stopCh chan struct{}

	// heartbeat ensures we wait no more than maxWait before firing
	heartbeat clock.Ticker

	// waitingForAddCh is a buffered channel that feeds waitingForAdd
	waitingForAddCh chan *waitFor
}

type empty struct{}
type t interface{}
type set map[t]empty

func (s set) has(item t) bool {
	_, exists := s[item]
	return exists
}

func (s set) insert(item t) {
	s[item] = empty{}
}

func (s set) delete(item t) {
	delete(s, item)
}

func NewRateLimitingFairQueue(rateLimiter workqueue.RateLimiter) workqueue.RateLimitingInterface {
	return newRateLimitingFairQueue(clock.RealClock{}, rateLimiter)
}

func newRateLimitingFairQueue(clock clock.RealClock, rateLimiter workqueue.RateLimiter) workqueue.RateLimitingInterface {
	ret := &fairQueue{
		balancer:        weightedroundrobin.NewWeightedRR(),
		queueGroup:      make(map[string][]t),
		dirty:           set{},
		processing:      set{},
		rateLimiter:     rateLimiter,
		cond:            sync.NewCond(&sync.Mutex{}),
		clock:           clock,
		heartbeat:       clock.NewTicker(maxWait),
		stopCh:          make(chan struct{}),
		waitingForAddCh: make(chan *waitFor, 1000),
	}

	go ret.waitingLoop()

	return ret
}

func (q *fairQueue) Add(obj interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	if q.shuttingDown {
		return
	}
	item, ok := obj.(Item)
	if !ok {
		return
	}

	if q.dirty.has(item) {
		return
	}

	q.dirty.insert(item)
	if q.processing.has(item) {
		return
	}

	group := item.Group()
	_, exists := q.queueGroup[group]
	if !exists {
		q.queueGroup[group] = []t{}
		q.balancer.Add(group, 1)
	}

	q.queueGroup[group] = append(q.queueGroup[group], item)
	q.cond.Signal()
}

func (q *fairQueue) Len() int {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return q.len()
}

func (q *fairQueue) len() int {
	var total = 0
	for _, q := range q.queueGroup {
		total += len(q)
	}
	return total
}

func (q *fairQueue) Get() (item interface{}, shutdown bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for q.len() == 0 && !q.shuttingDown {
		q.cond.Wait()
	}
	if q.len() == 0 {
		// We must be shutting down.
		return nil, true
	}

	var nextGroup string
	// TODO(zhuangqh): asynchronously queue gc.
	for {
		nextGroup = q.balancer.Next()
		if len(q.queueGroup[nextGroup]) == 0 {
			nextGroup = q.balancer.Next()
		} else {
			break
		}
	}

	item, q.queueGroup[nextGroup] = q.queueGroup[nextGroup][0], q.queueGroup[nextGroup][1:]

	q.processing.insert(item)
	q.dirty.delete(item)

	return item, false
}

func (q *fairQueue) Done(obj interface{}) {
	item, ok := obj.(Item)
	if !ok {
		return
	}

	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.processing.delete(item)
	if q.dirty.has(item) {
		group := item.Group()
		q.queueGroup[group] = append(q.queueGroup[group], item)
		q.cond.Signal()
	}
}

func (q *fairQueue) ShutDown() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	q.shuttingDown = true
	q.cond.Broadcast()
}

func (q *fairQueue) ShuttingDown() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	return q.shuttingDown
}

func (q *fairQueue) AddRateLimited(item interface{}) {
	q.AddAfter(item, q.rateLimiter.When(item))
}

func (q *fairQueue) Forget(item interface{}) {
	q.rateLimiter.Forget(item)
}

func (q *fairQueue) NumRequeues(item interface{}) int {
	return q.rateLimiter.NumRequeues(item)
}
