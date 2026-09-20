package mutex

import (
	"primitives/internal/futex"
	"sync/atomic"
)

const (
	free = iota
	held
	contended
)

type Mutex struct {
	state uint32
}

func (m *Mutex) Lock() {
	if atomic.CompareAndSwapUint32(&m.state, free, held) {
		return
	}
	for {
		if atomic.CompareAndSwapUint32(&m.state, free, contended) {
			return
		}
		atomic.CompareAndSwapUint32(&m.state, held, contended)
		futex.Wait(&m.state, contended)
	}
}

func (m *Mutex) TryLock() bool {
	return atomic.CompareAndSwapUint32(&m.state, free, held)
}

func (m *Mutex) Unlock() {
	old := atomic.SwapUint32(&m.state, free)

	switch old {
	case free:
		panic("unlock of unlocked mutex")

	case held:
		return

	case contended:
		futex.Wake(&m.state)
		return

	default:
		panic("invalid mutex state")
	}
}
