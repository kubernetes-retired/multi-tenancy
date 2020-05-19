package stats

import "sync/atomic"

type counter int32

func (c *counter) incr() {
	i := int32(*c)
	atomic.AddInt32(&i, 1)
	*c = counter(i)
}

func (c *counter) decr() {
	i := int32(*c)
	atomic.AddInt32(&i, -1)
	*c = counter(i)
}
