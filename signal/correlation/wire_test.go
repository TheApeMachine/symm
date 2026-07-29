package correlation

import (
	"context"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestCalculateWiresFocusedPeerOnAltBatch proves a focus-gated UI still receives
correlation frames for a warmed peer when only an alt ticker arrives.
*/
func TestCalculateWiresFocusedPeerOnAltBatch(t *testing.T) {
	Convey("Given a warmed two-symbol cohort with focus on A", t, func() {
		types.SetFocus("A/USD")
		Reset(func() { types.SetFocus("") })

		ui := make(chan []byte, 4)
		signal := NewSignal(context.Background(), ui)
		start := time.Unix(1_700_000_000, 0).UTC()

		for index := range 40 {
			at := start.Add(time.Duration(index) * time.Second)
			signal.Calculate([]kraken.TickerData{
				{
					Symbol:    "A/USD",
					Timestamp: at,
					Last:      decimal.NewFromFloat64(100 + float64(index)*0.2),
				},
				{
					Symbol:    "B/USD",
					Timestamp: at.Add(200 * time.Millisecond),
					Last:      decimal.NewFromFloat64(50 + float64(index)*0.1),
				},
			}, nil, nil)
		}

		drain(ui)

		Convey("When only B ticks, It still wires A for the focus gate", func() {
			at := start.Add(40 * time.Second)
			rows := signal.Calculate([]kraken.TickerData{
				{
					Symbol:    "B/USD",
					Timestamp: at,
					Last:      decimal.NewFromFloat64(54),
				},
			}, nil, nil)

			focused := 0

			for _, row := range rows {
				if row != nil && row.Symbol == "A/USD" {
					focused++
				}
			}

			So(focused, ShouldBeGreaterThan, 0)

			select {
			case frame := <-ui:
				var decoded map[string]any
				So(sonic.Unmarshal(frame, &decoded), ShouldBeNil)
				So(decoded["measurements"], ShouldNotBeNil)
			case <-time.After(time.Second):
				So("ui frame", ShouldEqual, "received")
			}
		})
	})
}

func drain(ui <-chan []byte) {
	for {
		select {
		case <-ui:
		default:
			return
		}
	}
}
