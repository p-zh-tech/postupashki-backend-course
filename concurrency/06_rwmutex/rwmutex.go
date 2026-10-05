package rwmutex

import (
	"sync/atomic"

	"primitives/internal/futex"
)

const writer uint32 = 1 << 31

type RWMutex struct {
	state          uint32
	waiters        uint32
	waitingWriters uint32
}

func (rw *RWMutex) RLock() {
	for {
		state := atomic.LoadUint32(&rw.state)

		// Если есть активный писатель или ожидающий писатель —
		// новые читатели не заходят.
		if state&writer != 0 || atomic.LoadUint32(&rw.waitingWriters) != 0 {
			atomic.AddUint32(&rw.waiters, 1)
			futex.Wait(&rw.state, state)
			atomic.AddUint32(&rw.waiters, ^uint32(0))
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
			// Последний читатель будит ожидающих.
			if newState == 0 && atomic.LoadUint32(&rw.waiters) != 0 {
				futex.WakeAll(&rw.state)
			}
			return
		}
	}
}

func (rw *RWMutex) Lock() {
	// Объявляем, что появился ожидающий писатель.
	atomic.AddUint32(&rw.waitingWriters, 1)
	defer atomic.AddUint32(&rw.waitingWriters, ^uint32(0))

	for {
		if atomic.CompareAndSwapUint32(&rw.state, 0, writer) {
			return
		}

		state := atomic.LoadUint32(&rw.state)

		// Между CAS и Load состояние могло стать 0.
		// В этом случае нельзя засыпать на futex(0).
		if state == 0 {
			continue
		}

		atomic.AddUint32(&rw.waiters, 1)
		futex.Wait(&rw.state, state)
		atomic.AddUint32(&rw.waiters, ^uint32(0))
	}
}

func (rw *RWMutex) Unlock() {
	if !atomic.CompareAndSwapUint32(&rw.state, writer, 0) {
		panic("unlock of unlocked rwmutex")
	}

	if atomic.LoadUint32(&rw.waiters) != 0 {
		futex.WakeAll(&rw.state)
	}
}
