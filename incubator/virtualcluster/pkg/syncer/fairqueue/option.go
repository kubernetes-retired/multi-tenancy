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
	"time"

	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/util/workqueue"
)

type option struct {
	// queueExpireDuration is the expire duration that the queue should be gc.
	queueExpireDuration time.Duration
	// gcTick is the period to do idle queue gc.
	gcTick *time.Ticker
	// clock tracks time for delayed firing
	clock clock.Clock

	// heartbeat ensures we wait no more than maxWait before firing
	heartbeat clock.Ticker

	rateLimiter workqueue.RateLimiter
}

var defaultConfig = option{
	queueExpireDuration: 5 * time.Minute,
	gcTick:              time.NewTicker(1 * time.Minute),
	clock:               clock.RealClock{},
	heartbeat:           clock.RealClock{}.NewTicker(maxWait),
	rateLimiter:         workqueue.DefaultControllerRateLimiter(),
}

type OptConfig func(*option)

// WithRateLimiter update the rate limiter.
func WithRateLimiter(rateLimiter workqueue.RateLimiter) OptConfig {
	return func(o *option) {
		o.rateLimiter = rateLimiter
	}
}

// WithIdleQueueCheckPeriod update the idle queue check period.
func WithIdleQueueCheckPeriod(period time.Duration) OptConfig {
	return func(o *option) {
		o.gcTick = time.NewTicker(period)
	}
}

// WithQueueExpireDuration update the queue expire duration.
func WithQueueExpireDuration(expireDuration time.Duration) OptConfig {
	return func(o *option) {
		o.queueExpireDuration = expireDuration
	}
}
