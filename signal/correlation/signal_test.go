package correlation

import (
	"sync"
	"testing"
	"time"

	nmcorrelation "github.com/theapemachine/nomagique/correlation"
)

func TestBoardAlignmentMedian(t *testing.T) {
	signal := &Signal{paths: new(sync.Map)}

	store := func(symbol string, values []float64) {
		p := &path{}
		base := time.Unix(0, 0)
		for i, v := range values {
			p.samples = append(p.samples, nmcorrelation.Sample{
				At:    base.Add(time.Duration(i) * time.Second),
				Value: v,
			})
		}
		signal.paths.Store(symbol, p)
	}

	store("AAA/USD", []float64{1, 2, 3, 4, 5})
	store("BBB/USD", []float64{1, 2, 3, 4, 5})
	store("CCC/USD", []float64{5, 4, 3, 2, 1})

	score, ok := signal.boardAlignment()
	if !ok {
		t.Fatal("expected defined alignment")
	}
	if score < 0.5 {
		t.Fatalf("aligned paths score = %v, want clearly coupled", score)
	}
}
