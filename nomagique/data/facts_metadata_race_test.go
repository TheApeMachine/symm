package data

import (
	"fmt"
	"sync"
	"testing"
)

/*
TestFactsMetadataConcurrentReadWrite proves Facts never races Set/DeleteMetadata.
 morphologylevel3 Finalizer → Facts was the fatal concurrent map path.
*/
func TestFactsMetadataConcurrentReadWrite(t *testing.T) {
	m := NewMeasurement[float64]("morphology:level3", map[string]Metric[float64]{
		"morphology_change": {Raw: 0.1},
	})

	var wg sync.WaitGroup
	start := make(chan struct{})
	workers := 24

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			for n := 0; n < 400; n++ {
				switch id % 4 {
				case 0:
					m.SetMetadata(MetadataSupport, fmt.Sprintf("%d", n+2))
					m.SetMetadata(MetadataDivergence, "0.01")
					m.SetMetadata(MetadataNoiseVariance, "1")
				case 1:
					m.DeleteMetadata(MetadataDivergence)
					m.DeleteMetadata(MetadataNoiseVariance)
				case 2:
					_ = m.Facts()
					m.Finalize()
				default:
					_ = m.MetadataSnapshot()
					m.EnsureMetadata()
					_, _ = m.GetMetadata(MetadataSupport)
				}
			}
		}(i)
	}

	close(start)
	wg.Wait()
}
