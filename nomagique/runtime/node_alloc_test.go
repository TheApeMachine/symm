package runtime

import (
	"context"
	"testing"

	"github.com/theapemachine/symm/nomagique/data"
)

type discardNode struct {
	arena *data.ArenaOwner
}

func (discardNode) Start(ctx context.Context) error { return nil }
func (discardNode) Name() string                    { return "discard" }
func (d discardNode) Arena() *data.ArenaOwner       { return d.arena }

func (d discardNode) Step(prior *data.Measurement) *data.Measurement {
	if prior == nil {
		return nil
	}

	out := d.arena.NewMeasurement("discard")
	out.Label = prior.Label
	out.SeqIdx = prior.SeqIdx
	out.At = prior.At
	out.From = prior.From
	out.Peers = []*data.Measurement{prior}
	return out
}

func BenchmarkNodeAllocations(b *testing.B) {
	arena := data.NewArenaOwner(1024)
	node := discardNode{arena: arena}
	consumer := NewConsumer(node, 1024, 1023)

	prior := data.NewMeasurement("ingress")
	prior.Label = "BTC/USD"
	prior.SeqIdx = 1

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		consumer.Step(prior, int64(i))
	}
}
