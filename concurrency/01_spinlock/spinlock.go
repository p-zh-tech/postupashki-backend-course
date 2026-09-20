package spinlock

import "sync/atomic"

type Spinlock struct {
	locked atomic.Bool
}

func (s *Spinlock) Lock() {
	for {
		if s.locked.CompareAndSwap(false, true) {
			break
		}
	}
}

func (s *Spinlock) TryLock() bool {
	return s.locked.CompareAndSwap(false, true)
}

func (s *Spinlock) Unlock() {
	if !(s.locked.CompareAndSwap(true, false)) {
		{
			panic("unlock of unlocked spinlock")
		}
	}
}

type TTAS struct {
	locked atomic.Bool
}

func (s *TTAS) Lock() {
	for {
		if !s.locked.Load() {
			if s.locked.CompareAndSwap(false, true) {
				break
			}
		}
	}
}

func (s *TTAS) TryLock() bool {
	return s.locked.CompareAndSwap(false, true)
}

func (s *TTAS) Unlock() {
	if !(s.locked.CompareAndSwap(true, false)) {
		{
			panic("unlock of unlocked spinlock")
		}
	}
}
