package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestTokenizer(t *testing.T) {
	Convey("Tokenizer", t, func() {
		Convey("validate checks valid parameter ranges", func() {
			So((&Tokenizer{Depth: 0, Regions: 1, Threshold: 0.1}).validate(), ShouldNotBeNil)
			So((&Tokenizer{Depth: 1, Regions: 0, Threshold: 0.1}).validate(), ShouldNotBeNil)
			So((&Tokenizer{Depth: 1, Regions: store.RegionCount + 1, Threshold: 0.1}).validate(), ShouldNotBeNil)
			So((&Tokenizer{Depth: 1, Regions: 1, Threshold: 0.1}).validate(), ShouldBeNil)
			So((&Tokenizer{Depth: 5, Regions: store.RegionCount, Threshold: 0.0}).validate(), ShouldBeNil)
		})

		Convey("Token names Top-K regions and extracts threshold strength", func() {
			regions := make([]*store.Region, store.RegionCount)

			for idx := range store.RegionCount {
				regions[idx] = &store.Region{
					Key:        uint8(idx + 1),
					Brightness: float64(store.RegionCount - idx),
				}
			}

			tok1 := &Tokenizer{Regions: 1}
			name1, strength1 := tok1.Token(regions)
			So(name1, ShouldEqual, "R01")
			So(strength1, ShouldEqual, float64(store.RegionCount))

			tok3 := &Tokenizer{Regions: 3}
			name3, strength3 := tok3.Token(regions)
			So(name3, ShouldEqual, "R01-R02-R03")
			So(strength3, ShouldEqual, float64(store.RegionCount-2))
		})

		Convey("Advance enforces strength threshold and deduplication", func() {
			tok := &Tokenizer{Depth: 3, Regions: 1, Threshold: 0.5}

			path, moved := tok.Advance(nil, "R01", 0.2)
			So(moved, ShouldBeFalse)
			So(len(path), ShouldEqual, 0)

			path, moved = tok.Advance(path, "R01", 0.6)
			So(moved, ShouldBeTrue)
			So(path, ShouldResemble, []string{"R01"})

			path, moved = tok.Advance(path, "R01", 0.8)
			So(moved, ShouldBeFalse)
			So(path, ShouldResemble, []string{"R01"})

			path, moved = tok.Advance(path, "R02", 0.7)
			So(moved, ShouldBeTrue)
			So(path, ShouldResemble, []string{"R01", "R02"})

			path, moved = tok.Advance(path, "R03", 0.9)
			So(moved, ShouldBeTrue)
			So(path, ShouldResemble, []string{"R01", "R02", "R03"})

			path, moved = tok.Advance(path, "R04", 1.0)
			So(moved, ShouldBeTrue)
			So(path, ShouldResemble, []string{"R02", "R03", "R04"})
		})

		Convey("Active returns regions clearing threshold", func() {
			regions := []*store.Region{
				{Key: 3, Brightness: 0.9},
				{Key: 7, Brightness: 0.6},
				{Key: 1, Brightness: 0.4},
			}
			tok := &Tokenizer{Threshold: 0.5}
			active, maxStrength := tok.Active(regions)
			So(active, ShouldResemble, []string{"R03", "R07"})
			So(maxStrength, ShouldEqual, 0.9)
		})

		Convey("StepGrammar induces rules from repeated sequence", func() {
			tok := &Tokenizer{Depth: 4, Threshold: 0.5}
			grammar := NewGrammar()
			var last string

			phrase, moved := tok.StepGrammar(grammar, &last, []string{"R01", "R02", "R03"})
			So(moved, ShouldBeFalse)
			So(phrase, ShouldBeNil)

			phrase, moved = tok.StepGrammar(grammar, &last, []string{"R01", "R02"})
			So(moved, ShouldBeTrue)
			So(phrase, ShouldResemble, []string{"R01", "R02"})
		})

		Convey("MatchSuffix finds longest matching suffix action", func() {
			actions := map[string]string{
				"R01/R02":     "noop",
				"R03/R01/R02": "enter",
			}

			tokens := []string{"R05", "R03", "R01", "R02"}
			matchedPath, action := MatchSuffix(tokens, actions)
			So(matchedPath, ShouldEqual, "R03/R01/R02")
			So(action, ShouldEqual, "enter")

			tokens = []string{"R05", "R01", "R02"}
			matchedPath, action = MatchSuffix(tokens, actions)
			So(matchedPath, ShouldEqual, "R01/R02")
			So(action, ShouldEqual, "noop")

			tokens = []string{"R05", "R04"}
			matchedPath, action = MatchSuffix(tokens, actions)
			So(matchedPath, ShouldEqual, "")
			So(action, ShouldEqual, "")
		})
	})
}
