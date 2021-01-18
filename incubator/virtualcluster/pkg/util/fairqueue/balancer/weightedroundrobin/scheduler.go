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

package weightedroundrobin

import (
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/fairqueue/balancer"
)

type node struct {
	Key    string
	Weight int
}

// Weighted Round-Robin Scheduling.
// wrr is not thread safe.
// doc: http://kb.linuxvirtualserver.org/wiki/Weighted_Round-Robin_Scheduling
type wrr struct {
	nodes  []*node
	keySet map[string]struct{}
	n      int
	gcd    int
	maxW   int
	i      int
	cw     int
}

func NewWeightedRR() balancer.Scheduler {
	return &wrr{
		keySet: make(map[string]struct{}),
	}
}

func (w *wrr) Next() string {
	if w.n == 0 {
		return ""
	}

	if w.n == 1 {
		return w.nodes[0].Key
	}

	for {
		w.i = (w.i + 1) % w.n
		if w.i == 0 {
			w.cw = w.cw - w.gcd
			if w.cw <= 0 {
				w.cw = w.maxW
				if w.cw == 0 {
					return ""
				}
			}
		}
		if w.nodes[w.i].Weight >= w.cw {
			return w.nodes[w.i].Key
		}
	}
}

func (w *wrr) Add(ref string, weight int) {
	if _, exists := w.keySet[ref]; exists {
		return
	}
	weighted := &node{Key: ref, Weight: weight}
	if weight > 0 {
		if w.gcd == 0 { // first item
			w.gcd = weight
			w.maxW = weight
			w.i = -1
			w.cw = 0
		} else {
			// calculate gcd and maxW incrementally
			w.gcd = gcd(w.gcd, weight)
			if w.maxW < weight {
				w.maxW = weight
			}
		}
	}
	w.nodes = append(w.nodes, weighted)
	w.keySet[ref] = struct{}{}
	w.n++
}

func (w *wrr) weightGcd() int {
	divisor := -1
	for _, n := range w.nodes {
		if divisor == -1 {
			divisor = n.Weight
		} else {
			divisor = gcd(divisor, n.Weight)
		}
	}
	return divisor
}

func (w *wrr) weightMax() int {
	max := -1
	for _, n := range w.nodes {
		if n.Weight > max {
			max = n.Weight
		}
	}
	return max
}

func gcd(x, y int) int {
	var t int
	for {
		t = x % y
		if t > 0 {
			x = y
			y = t
		} else {
			return y
		}
	}
}

func (w *wrr) Remove(ref string) {
	if _, exists := w.keySet[ref]; !exists {
		return
	}
	for i := 0; i < len(w.nodes); i++ {
		if w.nodes[i].Key == ref {
			w.nodes = append(w.nodes[:i], w.nodes[i+1:]...)
			if w.i >= i {
				w.i = w.i - 1
			}
			break
		}
	}

	delete(w.keySet, ref)
	w.n = len(w.nodes)
	w.cw = 0
	w.maxW = w.weightMax()
	w.gcd = w.weightGcd()
}

func (w *wrr) Clear() {
	w.keySet = make(map[string]struct{})
	w.nodes = []*node{}
	w.n = 0
	w.gcd = 0
	w.maxW = 0
	w.i = -1
	w.cw = 0
}
