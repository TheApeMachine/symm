package trader

/*
consume alternates one frozen ingress prefix with one fair manifold advance.
Events arriving during an expensive advance remain in the lock-free ingress
and are applied before the following symbol advances.
*/
func (level3 *Level3) consume() {
	defer level3.stopped.Store(true)

	for level3.ctx.Err() == nil {
		drained := level3.drain()
		processed := level3.process()
		advanced := level3.advance()

		if drained == 0 && processed == 0 && !advanced {
			level3.wait()
		}
	}
}

func (level3 *Level3) wait() {
	select {
	case <-level3.ctx.Done():
	case <-level3.wake:
	}
}

/*
drain takes exactly the prefix visible at method entry.
*/
func (level3 *Level3) drain() int {
	frozen := level3.ring.Len()
	drained := 0

	for range frozen {
		frame := level3.ring.Pop()

		if frame.sequence == 0 {
			break
		}

		if err := level3.frames.Add(frame); err != nil {
			level3.fail(err)
			return drained
		}

		drained++
	}

	return drained
}

/*
process observes every contiguous frame in claimed observation order.
*/
func (level3 *Level3) process() int {
	processed := 0

	for {
		frame, ok := level3.frames.Next()

		if !ok {
			return processed
		}

		level3.observe(frame)
		processed++

		if level3.ctx.Err() != nil {
			return processed
		}
	}
}

/*
advance gives one ready symbol one turn, then returns to ingress immediately.
*/
func (level3 *Level3) advance() bool {
	symbol, ok := level3.scheduler.Next()

	if !ok {
		return false
	}

	level3.analyzer.AdvanceLevel3(symbol)
	return true
}
