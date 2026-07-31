package utils

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEachKey(t *testing.T) {
	Convey("Given a flat multi-key frame", t, func() {
		raw := []byte(`{"balances":[1],"holdings":[2],"noise":true}`)
		seen := map[string]string{}

		Convey("It visits every top-level key once", func() {
			So(EachKey(raw, func(key string, value []byte) bool {
				seen[key] = string(value)
				return true
			}), ShouldBeNil)
			So(seen, ShouldResemble, map[string]string{
				"balances": `[1]`,
				"holdings": `[2]`,
				"noise":    `true`,
			})
		})
	})
}

func BenchmarkEachKeyFatFrame(b *testing.B) {
	particles := make([]byte, 0, 256*1024)
	particles = append(particles, `{"manifold_particles":[`...)

	for index := range 2000 {
		if index > 0 {
			particles = append(particles, ',')
		}

		particles = append(particles, `{"cell_x":1,"cell_z":2,"phase":0.1,"amplitude":1}`...)
	}

	particles = append(particles, `]}`...)
	b.ReportAllocs()

	for b.Loop() {
		_ = EachKey(particles, func(key string, value []byte) bool {
			_ = key
			_ = value
			return true
		})
	}
}

func TestGetStringSlice(t *testing.T) {
	Convey("Given JSON containing symbol arrays", t, func() {
		raw := []byte(`{"book": {"params":{"symbol":["XBT/USD","ETH/USD"]}}}`)

		Convey("It returns typed string values", func() {
			So(
				GetStringSlice(raw, "book", "params", "symbol"),
				ShouldResemble,
				[]string{"XBT/USD", "ETH/USD"},
			)
		})
	})

	Convey("Given JSON with mixed symbol array types", t, func() {
		raw := []byte(`{"book": {"params":{"symbol":["XBT/USD", 42]}}}`)

		Convey("It should return nil when any element is non-string", func() {
			So(
				GetStringSlice(raw, "book", "params", "symbol"),
				ShouldBeNil,
			)
		})
	})
}

func BenchmarkGetStringSlice(b *testing.B) {
	raw := []byte(`{"book":{"params":{"symbol":["BTC/USD","ETH/USD","SOL/USD","XRP/USD","DOGE/USD"]}}}`)
	b.ReportAllocs()

	for b.Loop() {
		_ = GetStringSlice(raw, "book", "params", "symbol")
	}
}
