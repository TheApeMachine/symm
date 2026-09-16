package types

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
)

func TestResonanceArtifactEncodeWire(t *testing.T) {
	Convey("The scatter receives actual settled coordinates without the unfocused hierarchy", t, func() {
		artifact := &ResonanceArtifact{
			Symbol: "ETH/USD", At: time.Unix(100, 0), ForwardCurve: []float64{0.4, 0.2},
			Snapshot: &learning.ManifoldReading{
				Latent: []float64{0.2, -0.4, 0.8},
				Layers: []learning.ResonanceLayerWire{{State: []float64{1, 2}}},
			},
		}
		for _, focused := range []bool{false, true, false} {
			wire := artifact.EncodeWire(focused)
			So(wire.Symbol, ShouldEqual, artifact.Symbol)
			So(wire.At, ShouldEqual, artifact.At.UnixNano())
			So(wire.Embedding, ShouldResemble, []float64{0.2, -0.4})
			So(len(wire.Latent) > 0, ShouldEqual, focused)
			So(len(wire.Layers) > 0, ShouldEqual, focused)
			So(len(wire.ForwardCurve) > 0, ShouldEqual, focused)
		}
		So(artifact.Snapshot.Latent, ShouldResemble, []float64{0.2, -0.4, 0.8})

		Convey("Missing coordinates are not invented", func() {
			artifact.Snapshot.Latent = []float64{1}
			So(artifact.EncodeWire(false).Embedding, ShouldBeNil)
			artifact.Snapshot = nil
			So(artifact.EncodeWire(false).Embedding, ShouldBeNil)
		})
	})
}

func BenchmarkResonanceArtifactEncodeWire(b *testing.B) {
	artifact := &ResonanceArtifact{Symbol: "ETH/USD", Snapshot: &learning.ManifoldReading{
		Latent: make([]float64, 64), Layers: []learning.ResonanceLayerWire{{State: make([]float64, 64)}},
	}}
	for _, focused := range []bool{false, true} {
		name := "embedding"

		if focused {
			name = "focused"
		}

		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				artifact.EncodeWire(focused)
			}
		})
	}
}
