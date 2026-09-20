package rwmutex

import (
	"sync/atomic"
	"primitives/internal/futex"
)

const writer uint32 = 1 << 31

type RWMutex struct {
	state uint32
}

func (rw *RWMutex) RLock() {
	for {
		state := atomic.LoadUint32(&rw.state)

		// Если писатель активен — ждём.
		if state&writer != 0 {
			futex.Wait(&rw.state, state)
			continue
		}

		if atomic.CompareAndSwapUint32(&rw.state, state, state+1) {
			return
		}
	}
}

func (rw *RWMutex) RUnlock() {
	for {
		state := atomic.LoadUint32(&rw.state)

		if state == 0 || state&writer != 0 {
			panic("runlock of unlocked rwmutex")
		}

		newState := state - 1

		if atomic.CompareAndSwapUint32(&rw.state, state, newState) {
			if newState == 0 {
				futex.WakeAll(&rw.state)
			}
			return
		}
	}
}

func (rw *RWMutex) Lock() {
	for {
		if atomic.CompareAndSwapUint32(&rw.state, 0, writer) {
			return
		}

		state := atomic.LoadUint32(&rw.state)
		futex.Wait(&rw.state, state)
	}
}

func (rw *RWMutex) Unlock() {
	if !atomic.CompareAndSwapUint32(&rw.state, writer, 0) {
		panic("unlock of unlocked rwmutex")
	}
	futex.WakeAll(&rw.state)
}
