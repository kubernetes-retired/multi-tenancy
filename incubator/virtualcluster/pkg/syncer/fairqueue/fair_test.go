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
	"strconv"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func groupItemWrapper(id string) *reconciler.Request {
	return &reconciler.Request{
		ClusterName: id,
		NamespacedName: types.NamespacedName{
			Namespace: "test",
			Name:      "testing-" + strconv.Itoa(int(time.Now().UnixNano())),
		},
	}
}

func TestBasicAndFair(t *testing.T) {
	// If something is seriously wrong this test will never complete.
	q := NewRateLimitingFairQueue()

	scheduleCounter := make(map[string]int)
	var mu sync.Mutex

	// Start producers
	const producers = 50
	const clusterNum = 5
	const enqueueNum = 50
	producerWG := sync.WaitGroup{}
	producerWG.Add(producers)
	for i := 0; i < clusterNum; i++ {
		clusterName := "tenant" + strconv.Itoa(i)
		scheduleCounter[clusterName] = 0

		for j := 0; j < producers/clusterNum; j++ {
			go func(clusterName string) {
				defer producerWG.Done()
				for j := 0; j < enqueueNum; j++ {
					q.Add(groupItemWrapper(clusterName))
					time.Sleep(time.Millisecond)
				}
			}(clusterName)
		}
	}

	// Start consumers
	const consumers = 10
	consumerWG := sync.WaitGroup{}
	consumerWG.Add(consumers)
	for i := 0; i < consumers; i++ {
		go func(i int) {
			defer consumerWG.Done()
			for {
				item, quit := q.Get()
				if quit {
					return
				}
				v, ok := item.(*reconciler.Request)
				if !ok {
					t.Errorf("unable cast to Item")
				}
				if v.ClusterName == "added after shutdown!" {
					t.Errorf("Got an item added after shutdown.")
				}

				// processing
				time.Sleep(3 * time.Millisecond)
				q.Done(item)

				mu.Lock()
				scheduleCounter[v.ClusterName] += 1
				mu.Unlock()
			}
		}(i)
	}

	producerWG.Wait()
	q.ShutDown()
	q.Add(groupItemWrapper("added after shutdown!"))
	consumerWG.Wait()

	countPerTenant := enqueueNum * producers / clusterNum
	for _, v := range scheduleCounter {
		if v != countPerTenant {
			t.Errorf("schedule results unfair %+v", scheduleCounter)
		}
	}
}

func TestLen(t *testing.T) {
	foo := groupItemWrapper("foo")
	bar := groupItemWrapper("bar")

	q := NewRateLimitingFairQueue()
	q.Add(foo)
	if e, a := 1, q.Len(); e != a {
		t.Errorf("Expected %v, got %v", e, a)
	}
	q.Add(bar)
	if e, a := 2, q.Len(); e != a {
		t.Errorf("Expected %v, got %v", e, a)
	}
	q.Add(foo) // should not increase the queue length.
	if e, a := 2, q.Len(); e != a {
		t.Errorf("Expected %v, got %v", e, a)
	}
}

func TestReinsert(t *testing.T) {
	q := NewRateLimitingFairQueue()
	foo := groupItemWrapper("foo")

	q.Add(foo)

	// Start processing
	i, _ := q.Get()
	v, ok := i.(*reconciler.Request)
	if !ok {
		t.Errorf("unable cast to Item")
	}
	if v.Name != foo.Name {
		t.Errorf("Expected %v, got %v", "foo", i)
	}

	// Add it back while processing
	q.Add(i)

	// Finish it up
	q.Done(i)

	// It should be back on the queue
	i, _ = q.Get()
	v, ok = i.(*reconciler.Request)
	if !ok {
		t.Errorf("unable cast to Item")
	}
	if v.Name != foo.Name {
		t.Errorf("Expected %v, got %v", "foo", i)
	}

	// Finish that one up
	q.Done(i)

	if a := q.Len(); a != 0 {
		t.Errorf("Expected queue to be empty. Has %v items", a)
	}
}

func TestQueueGC(t *testing.T) {
	timeUnit := 1 * time.Millisecond

	fq := NewRateLimitingFairQueue(
		WithIdleQueueCheckPeriod(timeUnit),
		WithQueueExpireDuration(timeUnit),
	)
	q := fq.(*fairQueue)
	foo := groupItemWrapper("foo")

	q.Add(foo)

	if q.GroupNum() != 1 {
		t.Errorf("expected 1 group, got %v", q.GroupNum())
	}

	time.Sleep(100 * timeUnit)
	if q.GroupNum() != 1 {
		t.Errorf("expected 1 group, got %v", q.GroupNum())
	}

	item, _ := q.Get()
	time.Sleep(100 * timeUnit)
	if q.GroupNum() != 0 {
		t.Errorf("expected 0 group, got %v", q.GroupNum())
	}

	q.Done(item)
	time.Sleep(100 * timeUnit)
	if q.GroupNum() != 0 {
		t.Errorf("expected 0 group, got %v", q.GroupNum())
	}
}
