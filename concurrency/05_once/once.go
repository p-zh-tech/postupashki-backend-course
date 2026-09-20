package once

import (
	"sync/atomic"

	"primitives/internal/futex"
)

const (
	idle = iota
	running
	done
)

type Once struct {
	state uint32
}

func (o *Once) Do(f func()) {
	if atomic.LoadUint32(&o.state) == done {
		return
	}

	if atomic.CompareAndSwapUint32(&o.state, idle, running) {
		defer func() {
			atomic.StoreUint32(&o.state, done)
			futex.WakeAll(&o.state)
		}()

		f()
		return
	}
	for {
		state := atomic.LoadUint32(&o.state)

		if state == done {
			return
		}

		futex.Wait(&o.state, running)
	}
}

func (o *Once) Done() bool {
	return atomic.LoadUint32(&o.state) == done
}
