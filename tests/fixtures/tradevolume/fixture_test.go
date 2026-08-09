package tradevolume

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestNewProfiles(t *testing.T) {
	Convey("Given a symbol-specific maker and taker schedule", t, func() {
		symbol := testtypes.NewSymbol("PROFILE/USD", 100, 61)
		symbol.TakerFeePercent = 0.31
		symbol.MakerFeePercent = 0.19
		fixture := NewProfiles([]*testtypes.Symbol{symbol})
		var payload []byte

		for frame := range fixture.Generate() {
			payload = frame
		}

		wire := map[string]any{}
		So(json.Unmarshal(payload, &wire), ShouldBeNil)
		result, _ := wire["result"].(map[string]any)
		fees, _ := result["fees"].(map[string]any)
		makers, _ := result["fees_maker"].(map[string]any)
		taker, _ := fees["PROFILEUSD"].(map[string]any)
		maker, _ := makers["PROFILEUSD"].(map[string]any)

		Convey("Both REST fee maps should carry the declared percentages", func() {
			So(taker["fee"], ShouldEqual, "0.31")
			So(maker["fee"], ShouldEqual, "0.19")
		})
	})
}

func BenchmarkNewProfiles(b *testing.B) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("PROFILE1/USD", 100, 61),
		testtypes.NewSymbol("PROFILE2/USD", 200, 62),
	}

	for b.Loop() {
		_ = NewProfiles(symbols)
	}
}
