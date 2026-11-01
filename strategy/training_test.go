package strategy

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

func testPrice() *broker.Price {
	return &broker.Price{
		Books: broker.NewBook(context.Background(), spot.NewNormalizer()),
		Tape:  broker.NewTape(),
	}
}

func TestTrainingLightBroker(t *testing.T) {
	Convey("Given mover volume and day move above floors", t, func() {
		price := testPrice()
		floor := system.Cfg.Risk.MinMoverVolumeUSD
		price.SetTicker(
			"AAA/USD",
			floor+1,
			system.Cfg.Risk.MinMoverChangePct+0.1,
		)

		training := NewTraining(context.Background(), price, nil)
		regions := make([]*runtime.Region, store.RegionCount)

		for key := uint8(0); key < store.RegionCount; key++ {
			regions[key] = &runtime.Region{Symbol: "AAA/USD", Key: key}
		}

		training.lightBroker("AAA/USD", regions)
		training.grid.Emit(regions, false)

		So(regions[10].Brightness, ShouldBeGreaterThan, 0)
		So(regions[9].Brightness, ShouldEqual, 0)
		So(regions[12].Brightness, ShouldBeGreaterThan, 0)
		So(regions[11].Brightness, ShouldEqual, 0)
	})

	Convey("Quiet volume lights the volume veto", t, func() {
		price := testPrice()
		price.SetTicker("QUIET/USD", 1, 5)

		training := NewTraining(context.Background(), price, nil)
		regions := make([]*runtime.Region, store.RegionCount)

		for key := uint8(0); key < store.RegionCount; key++ {
			regions[key] = &runtime.Region{Symbol: "QUIET/USD", Key: key}
		}

		training.lightBroker("QUIET/USD", regions)
		training.grid.Emit(regions, false)

		So(regions[9].Brightness, ShouldBeGreaterThan, 0)
		So(regions[10].Brightness, ShouldEqual, 0)
	})

	Convey("Board.Tradable rejects a lit veto region", t, func() {
		regions := make([]*runtime.Region, store.RegionCount)
		for key := uint8(0); key < store.RegionCount; key++ {
			regions[key] = &runtime.Region{Key: key}
		}
		store.Shine(regions[10])
		store.Shine(regions[9])
		So(Board(regions).Tradable(), ShouldBeFalse)
	})
}

func TestStepCutRegime(t *testing.T) {
	Convey("stepCut emits one dominant action regime per tick", t, func() {
		training := NewTraining(context.Background(), nil, nil)
		regions := make([]*runtime.Region, store.RegionCount)

		for key := uint8(0); key < store.RegionCount; key++ {
			regions[key] = &runtime.Region{Symbol: "BTC/USD", Key: key}
		}

		store.Shine(regions[10])
		store.Shine(regions[18])
		regions[0].Brightness = 0.8
		regions[0].Deformation = 2.4
		regions[0].Counts = 1
		regions[2].Brightness = 0.3

		cut := training.stepCut("BTC/USD", regions)
		So(cut.String(), ShouldEqual, "R00")
	})

	Convey("stepCut builds a multi-tick precursor over time", t, func() {
		training := NewTraining(context.Background(), nil, nil)
		symbol := "ETH/USD"

		tick := func(key uint8, brightness float64) tokenizer.Tokens {
			regions := make([]*runtime.Region, store.RegionCount)
			for k := uint8(0); k < store.RegionCount; k++ {
				regions[k] = &runtime.Region{Symbol: symbol, Key: k}
			}
			regions[key].Brightness = brightness
			return training.stepCut(symbol, regions)
		}

		_ = tick(2, 0.5)
		_ = tick(0, 0.7)
		cut := tick(6, 0.9)
		So(cut.String(), ShouldEqual, "R02/R00/R06")
	})
}
