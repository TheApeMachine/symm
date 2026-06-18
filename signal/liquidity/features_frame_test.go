package liquidity

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestSignalFeaturePayloadFrame(testingTB *testing.T) {
	Convey("Given a cross-section with more peers than the old frame allowed", testingTB, func() {
		signal := NewSignal(context.Background(), newTestPool(testingTB))
		So(signal, ShouldNotBeNil)

		peers := make([]float64, 1100)

		for index := range peers {
			peers[index] = float64(index + 1)
		}

		samples := depthFeaturesPayload(100, peers, 1, false)

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

		Convey("When Measure reads the wide peer frame", func() {
			insertFeatures(signal, "ETH/USD", samples...)

			result := signal.Measure(measurementQuery("ETH/USD"))

			So(result, ShouldNotBeNil)
			So(datura.Peek[string](result, "scope"), ShouldEqual, "ETH/USD")
			So(datura.Peek[int](result, "classifier.category"), ShouldBeGreaterThan, 0)
			So(fmt.Sprintf("%d", len(peers)), ShouldEqual, "1100")
		})
	})
}
