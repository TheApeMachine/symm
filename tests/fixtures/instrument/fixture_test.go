package instrument

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestNewFixture(t *testing.T) {
	Convey("Given the instrument fixture package", t, func() {
		Convey("When a snapshot fixture is created", func() {
			fixture := NewFixture(SNAPSHOT, 1)

			Convey("Then it should emit one instrument snapshot with assets and pairs", func() {
				var frame map[string]any
				for payload := range fixture.Generate() {
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				}

				data := frame["data"].(map[string]any)
				pair := data["pairs"].([]any)[0].(map[string]any)

				So(frame["channel"], ShouldEqual, "instrument")
				So(frame["type"], ShouldEqual, "snapshot")
				So(data["assets"], ShouldNotBeEmpty)
				So(data["pairs"], ShouldNotBeEmpty)
				So(pair["status"], ShouldEqual, "online")
			})
		})

		Convey("When an update fixture is created", func() {
			fixture := NewFixture(UPDATE, 3)

			Convey("Then it should generate instrument update frames", func() {
				count := 0

				for payload := range fixture.Generate() {
					var frame map[string]any
					So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
					data := frame["data"].(map[string]any)
					pairs := data["pairs"].([]any)
					pair := pairs[0].(map[string]any)

					So(frame["type"], ShouldEqual, "update")
					So(pairs, ShouldHaveLength, 1)
					So(pair["symbol"], ShouldEqual, "MATIC/USD")
					count++
				}

				So(count, ShouldEqual, 3)
			})
		})

		Convey("When a simulated market snapshot is created", func() {
			symbol := testtypes.NewSymbol("SIM1/USD", 100.0, 42)
			symbol.QuantityPrecision = 5
			symbol.OrderMinimum = 0.01234
			symbol.CostMinimum = 2.75
			fixture := NewMarket([]*testtypes.Symbol{symbol})

			Convey("Then Kraken execution rules retain their exact JSON values", func() {
				var frame map[string]any

				for payload := range fixture.Generate() {
					decoder := json.NewDecoder(bytes.NewReader(payload))
					decoder.UseNumber()
					So(decoder.Decode(&frame), ShouldBeNil)
				}

				data := frame["data"].(map[string]any)
				pair := data["pairs"].([]any)[0].(map[string]any)
				So(pair["qty_precision"].(json.Number).String(), ShouldEqual, "5")
				So(pair["qty_increment"].(json.Number).String(), ShouldEqual, "0.00001")
				So(pair["qty_min"].(json.Number).String(), ShouldEqual, "0.01234")
				So(pair["cost_min"].(json.Number).String(), ShouldEqual, "2.75")
				So(pair["tick_size"].(json.Number).String(), ShouldEqual, "0.01")
				So(pair["price_increment"].(json.Number).String(), ShouldEqual, "0.01")
			})
		})

		Convey("When simulated pairs span different price scales", func() {
			fixture := NewMarket([]*testtypes.Symbol{
				testtypes.NewSymbol("LOW/USD", 0.00012345, 1),
				testtypes.NewSymbol("HIGH/USD", 987654321.0, 2),
			})

			Convey("Then each pair should advertise its own quoting increment", func() {
				var frame map[string]any

				for payload := range fixture.Generate() {
					decoder := json.NewDecoder(bytes.NewReader(payload))
					decoder.UseNumber()
					So(decoder.Decode(&frame), ShouldBeNil)
				}

				data := frame["data"].(map[string]any)
				pairs := data["pairs"].([]any)
				low := pairs[0].(map[string]any)
				high := pairs[1].(map[string]any)

				So(low["price_increment"].(json.Number).String(),
					ShouldEqual, "0.00000001")
				So(high["price_increment"].(json.Number).String(),
					ShouldEqual, "10000")
			})
		})
	})
}
