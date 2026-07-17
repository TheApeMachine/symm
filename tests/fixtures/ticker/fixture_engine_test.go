package ticker

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
)

func TestNewWithEngine(t *testing.T) {
	Convey("Given a custom ticker engine", t, func() {
		raw := []byte(`{"channel":"ticker","type":"update","data":[{"symbol":"MATIC/USD","bid":0.10,"ask":0.11,"last":0.105,"volume":100,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)
		engine := tests.NewEngine(4).
			Drift(0.002).
			Jitter(0.01).
			Seed(11).
			VolumeAdd(3).
			Interval(2 * time.Second)

		fixture := NewWithEngine(UPDATE, engine, raw)

		Convey("When frames are generated", func() {
			lasts := lastPrices(t, fixture)

			repeat := NewWithEngine(UPDATE, engine, raw)

			Convey("Then the custom engine should drive a repeatable stream", func() {
				So(len(lasts), ShouldEqual, 4)
				So(lasts[0], ShouldBeGreaterThan, 0.105)
				So(lastPrices(t, repeat), ShouldResemble, lasts)
			})
		})
	})
}

func lastPrices(t testing.TB, fixture *Fixture) []float64 {
	t.Helper()

	lasts := make([]float64, 0)

	for frame := range fixture.Frames() {
		var payload map[string]any

		if err := sonic.Unmarshal(frame.Payload, &payload); err != nil {
			t.Fatal(err)
		}

		row := payload["data"].([]any)[0].(map[string]any)
		lasts = append(lasts, row["last"].(float64))
	}

	return lasts
}
