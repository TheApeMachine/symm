package liquidity

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
)

func TestSignalFeaturePayloadFrame(testingTB *testing.T) {
	Convey("Given a cross-section with more peers than the old frame allowed", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		peers := make([]float64, 1100)

		for index := range peers {
			peers[index] = float64(index + 1)
		}

		samples := depthFeatureBatch(100, peers)

		Convey("When depth features are encoded", func() {
			payload, marshalErr := json.Marshal(samples)
			So(marshalErr, ShouldBeNil)
			So(len(payload), ShouldBeGreaterThan, 0)

			var decoded []float64

			unmarshalErr := json.Unmarshal(payload, &decoded)
			So(unmarshalErr, ShouldBeNil)
			So(len(decoded), ShouldBeGreaterThan, 4)
			So(decoded[1], ShouldEqual, float64(len(peers)))
		})
	})
}
