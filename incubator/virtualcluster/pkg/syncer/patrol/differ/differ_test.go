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

package differ

import (
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

func makeObject(ns, name string) metav1.Object {
	return &metav1.ObjectMeta{
		Namespace: ns,
		Name:      name,
	}
}

func TestDifferSet(t *testing.T) {
	s := NewDiffSet()
	if s.Len() != 0 {
		t.Errorf("Expected len=0: %d", s.Len())
	}

	a := ClusterObject{Key: "n1/a", Object: makeObject("n1", "a")}
	b := ClusterObject{Key: "n1/b", Object: makeObject("n1", "b")}
	c := ClusterObject{Key: "n1/c", Object: makeObject("n1", "c")}

	s.Insert(a)
	if s.Len() != 1 {
		t.Errorf("Expected len=2: %d", s.Len())
	}
	s.Insert(b)
	if s.Has(c) {
		t.Errorf("Unexpected contents: %s", c)
	}
	if !s.Has(a) {
		t.Errorf("Missing contents: %s", a)
	}
	s.Delete(a)
	if s.Has(a) {
		t.Errorf("Unexpected contents: %s", a)
	}
	if s.Len() != 1 {
		t.Errorf("Expected len=1: %d", s.Len())
	}
	if !s.GetKeys().Has("n1/b") {
		t.Errorf("Missing key: %s", "n1/b")
	}
	s.Clear()
	if s.Len() > 0 {
		t.Errorf("Expected len=0: %d", s.Len())
	}
}

func TestDifferSetDifference(t *testing.T) {
	ta := ClusterObject{Key: "t1-n1/a", OwnerCluster: "t1", Object: makeObject("n1", "a")}
	a := ClusterObject{Key: "t1-n1/a", Object: makeObject(conversion.ToSuperMasterNamespace("t1", "n1"), "a")}
	tb := ClusterObject{Key: "t1-n1/b", OwnerCluster: "t1", Object: makeObject("n1", "b")}
	c := ClusterObject{Key: "t1-n1/c", Object: makeObject(conversion.ToSuperMasterNamespace("t1", "n1"), "c")}

	vSet := NewDiffSet(ta, tb)
	pSet := NewDiffSet(a, c)

	addCounter := make(map[string]int)
	updateCounter := make(map[string]int)
	deleteCounter := make(map[string]int)

	d := HandlerFuncs{
		AddFunc: func(obj ClusterObject) {
			addCounter[obj.Key] = addCounter[obj.Key] + 1
		},
		UpdateFunc: func(obj1, obj2 ClusterObject) {
			updateCounter[obj1.Key] = updateCounter[obj1.Key] + 1
			updateCounter[obj2.Key] = updateCounter[obj1.Key]
		},
		DeleteFunc: func(obj ClusterObject) {
			deleteCounter[obj.Key] = deleteCounter[obj.Key] + 1
		},
	}

	vSet.Difference(pSet, d)

	expectedAddCounter := map[string]int{tb.Key: 1}
	expectedUpdateCounter := map[string]int{ta.Key: 1}
	expectedDeleteCounter := map[string]int{c.Key: 1}

	if !equality.Semantic.DeepEqual(addCounter, expectedAddCounter) {
		t.Errorf("Expected addCounter %+v, got %+v", expectedAddCounter, addCounter)
	}
	if !equality.Semantic.DeepEqual(updateCounter, expectedUpdateCounter) {
		t.Errorf("Expected updateCounter %+v, got %+v", expectedUpdateCounter, updateCounter)
	}
	if !equality.Semantic.DeepEqual(deleteCounter, expectedDeleteCounter) {
		t.Errorf("Expected deleteCounter %+v, got %+v", expectedDeleteCounter, deleteCounter)
	}
}

func Benchmark_Difference_1000(b *testing.B) {
	b.ReportAllocs()
	rand.Seed(time.Now().UnixNano())

	groupNum := 50
	totalNum := 1000

	pSet := NewDiffSet()
	for i := 0; i < groupNum; i++ {
		for j := 0; j < totalNum/groupNum; j++ {
			cluster := strconv.Itoa(i)
			obj := makeObject(conversion.ToSuperMasterNamespace(cluster, "n"), strconv.Itoa(j))
			pSet.Insert(ClusterObject{
				Object: obj,
				Key:    DefaultClusterObjectKey(obj, ""),
			})
		}
	}

	vSet := NewDiffSet()
	for i := 0; i < groupNum; i++ {
		for j := 0; j < totalNum/groupNum; j++ {
			obj := makeObject("n", strconv.Itoa(j))
			cluster := strconv.Itoa(i)
			vSet.Insert(ClusterObject{
				Object:       obj,
				OwnerCluster: cluster,
				Key:          DefaultClusterObjectKey(obj, cluster),
			})
		}
	}

	counter := make(map[string]int)
	var mu sync.Mutex

	b.ResetTimer()
	vSet.Difference(pSet, HandlerFuncs{
		UpdateFunc: func(obj1, obj2 ClusterObject) {
			// workload
			time.Sleep(time.Millisecond)
			mu.Lock()
			counter[obj1.Key] += 1
			mu.Unlock()
		},
	})
	b.StopTimer()

	if len(counter) != totalNum {
		b.Errorf("expected num=%d, got %d", totalNum, len(counter))
	}
	for _, count := range counter {
		if count != 1 {
			b.Errorf("count more than 1: %d", count)
		}
	}
}
