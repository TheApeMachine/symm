package runtime

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/nomagique/data"
)

type discardNode struct{}

func (discardNode) Start(ctx context.Context) error { return nil }
func (discardNode) Name() string                      { return "discard" }

func (discardNode) Step(input *StageInput, output *data.Measurement[float64]) *data.Measurement[float64] {
	return output
}

func BenchmarkNodeAllocations(b *testing.B) {
	input := data.NewMeasurement[float64]("test", nil)
	stageInputs := make([]*StageInput, 1024)
	for i := 0; i < 1024; i++ {
		stageInputs[i] = NewStageInput(int64(i), input, nil)
	}
	consumer := NewConsumer(discardNode{}, 1024, 1023, stageInputs)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		consumer.Handle(int64(i), int64(i))
	}
}
