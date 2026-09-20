package waitgroup

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type WaitGroup struct {
	count uint32
}

func (wg *WaitGroup) Add(delta int) {
	for {
		old := atomic.LoadUint32(&wg.count)

		if delta < 0 && uint32(-delta) > old {
			panic("negative WaitGroup counter")
		}

		newCount := uint32(int64(old) + int64(delta))

		if atomic.CompareAndSwapUint32(&wg.count, old, newCount) {
			if newCount == 0 && old != 0 {
				futex.WakeAll(&wg.count)
			}
			return
		}
	}
}

func (wg *WaitGroup) Done() {
	wg.Add(-1)
}

func (wg *WaitGroup) Wait() {
	for {
		count := atomic.LoadUint32(&wg.count)

		if count == 0 {
			return
		}

		futex.Wait(&wg.count, count)
	}
}
