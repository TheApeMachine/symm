package audit

// writerQueue is the audit publish queue. The implementation is a lock-free
// bounded ring so producers never contend on a mutex during bursts.
type writerQueue = ringQueue

func newWriterQueue() *writerQueue {
	return newRingQueue()
}
