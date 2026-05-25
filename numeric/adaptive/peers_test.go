package adaptive

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPeerValues(t *testing.T) {
	Convey("Given a cross-section map", t, func() {
		values := map[string]float64{
			"AAA/EUR": 10,
			"BBB/EUR": 20,
			"CCC/EUR": 30,
		}

		Convey("It should omit the skipped symbol", func() {
			peers := PeerValues(values, "BBB/EUR")

			So(peers, ShouldHaveLength, 2)
			So(peers, ShouldContain, 10.0)
			So(peers, ShouldContain, 30.0)
		})
	})
}
