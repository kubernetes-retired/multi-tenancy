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
	"math/rand"
	"strconv"
	"testing"
	"time"
)

func Test_RR_Next(t *testing.T) {
	wrr := NewWeightedRR()

	// case rr
	wrr.Add("a", 1)
	wrr.Add("b", 1)
	wrr.Add("c", 1)

	scheduleCounter := make(map[string]int)

	for i := 0; i < 666; i++ {
		s := wrr.Next()
		scheduleCounter[s]++
	}

	if scheduleCounter["a"] != 222 || scheduleCounter["b"] != 222 || scheduleCounter["c"] != 222 {
		t.Errorf("schdule result is unfair: %+v", scheduleCounter)
	}

	// case wrr
	wrr.Clear()
	wrr.Add("a", 1)
	wrr.Add("b", 2)
	wrr.Add("c", 3)
	wrr.Add("d", 4)

	scheduleCounter = make(map[string]int)

	for i := 0; i < 1000; i++ {
		s := wrr.Next()
		scheduleCounter[s]++
	}

	if scheduleCounter["a"] != 100 || scheduleCounter["b"] != 200 || scheduleCounter["c"] != 300 || scheduleCounter["d"] != 400 {
		t.Errorf("schdule result is unfair: %+v", scheduleCounter)
	}

	// case wrr after remove node
	wrr.Remove("b")
	wrr.Remove("c")

	scheduleCounter = make(map[string]int)

	for i := 0; i < 1000; i++ {
		s := wrr.Next()
		scheduleCounter[s]++
	}

	if scheduleCounter["a"] != 200 || scheduleCounter["d"] != 800 {
		t.Errorf("schdule result is unfair: %+v", scheduleCounter)
	}

	// case wrr remove to 0
	wrr.Next()      // move iterator to middle
	wrr.Remove("b") // duplicate remove
	wrr.Remove("a")
	wrr.Remove("d")

	if next := wrr.Next(); next != "" {
		t.Errorf("non empty after resize to 0")
	}
}

func Benchmark_WRR_10_Next(b *testing.B) {
	b.ReportAllocs()
	rand.Seed(time.Now().UnixNano())
	wrr := NewWeightedRR()
	for i := 0; i < 10; i++ {
		wrr.Add("n"+strconv.Itoa(i), rand.Intn(100))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrr.Next()
	}
}

func Benchmark_WRR_100_Next(b *testing.B) {
	b.ReportAllocs()
	rand.Seed(time.Now().UnixNano())
	wrr := NewWeightedRR()
	for i := 0; i < 100; i++ {
		wrr.Add("n"+strconv.Itoa(i), rand.Intn(100))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrr.Next()
	}
}
