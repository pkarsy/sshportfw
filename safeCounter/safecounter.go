// a very simple package offering a thread safe counter
package safeCounter

import "sync"

type SafeCounter struct {
	sync.Mutex // Guard for the counter
	counter    int
}

func (c *SafeCounter) Inc() int {
	c.Lock()
	defer c.Unlock()
	if c.counter < 0 {
		c.counter = 0
	}
	c.counter++
	return c.counter
}

func (c *SafeCounter) Dec() int {
	c.Lock()
	defer c.Unlock()
	if c.counter > 0 {
		c.counter--
	} else {
		c.counter = 0
	}
	return c.counter
}

// Creates a new counter with atomic increase and decrease
func New() *SafeCounter {
	return &SafeCounter{}
}
