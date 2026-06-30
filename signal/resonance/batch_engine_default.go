//go:build !darwin || !cgo

package resonance

func newBatchEngine(arch []int, alpha float64, batchSize int) (batchEngine, error) {
	return newPortableBatchEngine(arch, alpha, batchSize)
}
