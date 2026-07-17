package correlation

import (
	"testing"
	"time"

	nomcorrelation "github.com/theapemachine/nomagique/correlation"
)

func BenchmarkSectionReturns(b *testing.B) {
	section := NewSection()
	start := time.Unix(0, 0).UTC()
	step := time.Second
	samples := make([]nomcorrelation.Sample, 0, 256)

	for index := range cap(samples) {
		at := start.Add(time.Duration(index) * step)
		value := 100 + float64(index)
		samples = append(samples, nomcorrelation.Sample{
			At:    at,
			Value: value,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		section.returns(samples, nil)
	}
}
