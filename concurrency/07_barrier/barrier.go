package barrier

import (
	"sync/atomic"
	"primitives/internal/futex"
)

type Barrier struct {
	need    uint32
	arrived uint32
	round   uint32
}

func New(n int) *Barrier {
	if n <= 0 {
		panic("barrier size must be positive")
	}

	return &Barrier{
		need: uint32(n),
	}
}

func (b *Barrier) Wait() {

	myRound := atomic.LoadUint32(&b.round)

	arrived := atomic.AddUint32(&b.arrived, 1)

	if arrived == b.need {
		atomic.StoreUint32(&b.arrived, 0)
		atomic.AddUint32(&b.round, 1)
		futex.WakeAll(&b.round)
		return
	}

	for {
		if atomic.LoadUint32(&b.round) != myRound {
			return
		}

		futex.Wait(&b.round, myRound)
	}
}
