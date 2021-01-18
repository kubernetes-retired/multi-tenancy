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

import "time"

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

type fifoQueue struct {
	// queue defines the order in which we will work on items. Every
	// element of queue should be in the dirty set and not in the
	// processing set.
	queue []t

	// lastActiveTime is the last known timestamp this queue is active.
	lastActiveTime time.Time
}

func NewFIFOQueue() *fifoQueue {
	return &fifoQueue{
		lastActiveTime: time.Now(),
	}
}

func (q *fifoQueue) Add(item interface{}) {
	q.queue = append(q.queue, item)
	q.lastActiveTime = time.Now()
}

// Get pop the queue head. indicate whether the queue is empty.
func (q *fifoQueue) Get() (item interface{}, empty bool) {
	if len(q.queue) == 0 {
		return nil, true
	}

	item, q.queue = q.queue[0], q.queue[1:]

	q.lastActiveTime = time.Now()

	return item, false
}

func (q *fifoQueue) Len() int {
	return len(q.queue)
}

func (q *fifoQueue) LastActiveTime() time.Time {
	return q.lastActiveTime
}
