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
