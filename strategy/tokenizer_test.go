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
			So((&Tokenizer{Depth: 1, Regions: 13, Threshold: 0.1}).validate(), ShouldNotBeNil)
			So((&Tokenizer{Depth: 1, Regions: 1, Threshold: 0.1}).validate(), ShouldBeNil)
			So((&Tokenizer{Depth: 5, Regions: 12, Threshold: 0.0}).validate(), ShouldBeNil)
		})

		Convey("Token names Top-K regions and extracts threshold strength", func() {
			regions := make([]*store.Region, 12)

			for idx := range 12 {
				regions[idx] = &store.Region{
					Key:        uint8(idx + 1),
					Brightness: float64(12 - idx),
				}
			}

			tok1 := &Tokenizer{Regions: 1}
			name1, strength1 := tok1.Token(regions)
			So(name1, ShouldEqual, "R01")
			So(strength1, ShouldEqual, 12.0)

			tok3 := &Tokenizer{Regions: 3}
			name3, strength3 := tok3.Token(regions)
			So(name3, ShouldEqual, "R01-R02-R03")
			So(strength3, ShouldEqual, 10.0)
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
	})
}
