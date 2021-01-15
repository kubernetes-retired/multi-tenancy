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
	"time"

	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/fairqueue/balancer"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/fairqueue/balancer/weightedroundrobin"
)

type Item interface {
	GroupName() string
}

type fairQueue struct {
	option

	// balancer choose which queue to go.
	balancer balancer.Scheduler
	// queueGroup group each queue by a unique key.
	queueGroup map[string]*fifoQueue

	// length is the sum of queues size.
	length int

	// dirty defines all of the items that need to be processed.
	dirty set

	// Things that are currently being processed are in the processing set.
	// These things may be simultaneously in the dirty set. When we finish
	// processing something and remove it from this set, we'll check if
	// it's in the dirty set, and if so, add it to the queue.
	processing set

	cond *sync.Cond

	shuttingDown bool

	// stopCh lets us signal a shutdown to the waiting loop
	stopCh chan struct{}
	// stopOnce guarantees we only signal shutdown a single time
	stopOnce sync.Once

	// waitingForAddCh is a buffered channel that feeds waitingForAdd
	waitingForAddCh chan *waitFor
}

func NewRateLimitingFairQueue(opts ...OptConfig) workqueue.RateLimitingInterface {
	o := defaultConfig
	for _, opt := range opts {
		opt(&o)
	}
	ret := &fairQueue{
		option:          o,
		balancer:        weightedroundrobin.NewWeightedRR(),
		queueGroup:      make(map[string]*fifoQueue),
		dirty:           make(set),
		processing:      make(set),
		cond:            sync.NewCond(&sync.Mutex{}),
		stopCh:          make(chan struct{}),
		waitingForAddCh: make(chan *waitFor, 1000),
	}

	go ret.waitingLoop()
	go ret.cleanIdleQueue()

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

	group := item.GroupName()
	fifo, exists := q.queueGroup[group]
	if !exists {
		fifo = NewFIFOQueue()
		q.queueGroup[group] = fifo
		// TODO(zhuangqh): weight aware fair queue after introducing priority to vc crd.
		// filled in `1` here and weightroundrobin will downgrade to roundrobin.
		q.balancer.Add(group, 1)
	}

	fifo.Add(item)
	q.length++

	q.cond.Signal()
}

func (q *fairQueue) Len() int {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return q.length
}

func (q *fairQueue) GroupNum() int {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return len(q.queueGroup)
}

func (q *fairQueue) Get() (item interface{}, shutdown bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for q.length == 0 && !q.shuttingDown {
		q.cond.Wait()
	}
	if q.length == 0 {
		// We must be shutting down.
		return nil, true
	}

	var nextGroup string
	for {
		nextGroup = q.balancer.Next()
		if q.queueGroup[nextGroup].Len() != 0 {
			break
		}
	}

	item, _ = q.queueGroup[nextGroup].Get()
	q.length--
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

	fifo, exists := q.queueGroup[item.GroupName()]
	if !exists {
		if q.dirty.has(item) {
			fifo = NewFIFOQueue()
			q.queueGroup[item.GroupName()] = fifo
		}
		return
	}

	if q.dirty.has(item) {
		fifo.Add(item)
		q.length++
		q.cond.Signal()
	}
}

func (q *fairQueue) ShutDown() {
	q.stopOnce.Do(func() {
		q.cond.L.Lock()
		defer q.cond.L.Unlock()
		q.shuttingDown = true
		q.cond.Broadcast()
		close(q.stopCh)
		q.heartbeat.Stop()
	})
}

func (q *fairQueue) ShuttingDown() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	return q.shuttingDown
}

func (q *fairQueue) cleanIdleQueue() {
	for {
		select {
		case <-q.stopCh:
			return
		case now := <-q.gcTick.C:
			q.checkAndCleanIdleQueue(now)
		}
	}
}

func (q *fairQueue) checkAndCleanIdleQueue(now time.Time) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for group, fifo := range q.queueGroup {
		lastActiveTime := fifo.LastActiveTime()
		// delete this group if its active time is far away from now.
		if lastActiveTime.Add(q.queueExpireDuration).Before(now) && fifo.Len() == 0 {
			q.balancer.Remove(group)
			delete(q.queueGroup, group)
			klog.V(4).Infof("fairqueue: queue %v idle for more than %v, removed", group, q.queueExpireDuration)
		}
	}
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
