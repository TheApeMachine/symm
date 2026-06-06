package hawkes

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func hawkesTestCategories() map[string]types.CategoryType {
	return map[string]types.CategoryType{
		"organic":    types.CategoryOrganic,
		"frenzy":     types.CategoryFrenzy,
		"saturation": types.CategorySaturation,
		"exhaustion": types.CategoryExhaustion,
	}
}

func TestNewHawkesSymbol(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a Hawkes symbol state", t, func() {
		symbol := NewHawkesSymbol(nil, hawkesTestCategories())

		Convey("It should initialize cooldown from config", func() {
			So(symbol.fitCooldown, ShouldBeGreaterThan, 0)
			So(symbol.tracked, ShouldNotBeNil)
		})
	})
}

func TestHawkesSymbolMeasure(t *testing.T) {
	testconfig.Load(t)

	Convey("Given trade ticks", t, func() {
		symbol := NewHawkesSymbol(nil, hawkesTestCategories())
		now := time.Now()
		ticks := make([]market.TradeUpdate, 0, 20)

		for index := range 20 {
			side := "buy"

			if index%2 == 1 {
				side = "sell"
			}

			ticks = append(ticks, market.TradeUpdate{
				Symbol:    "BTC/EUR",
				Price:     50_000 + float64(index),
				Qty:       0.01,
				Side:      side,
				Timestamp: now.Add(time.Duration(index) * time.Second),
			})
		}

		measurement, _, err := symbol.Measure(ticks, now.Add(20*time.Second))

		Convey("It should emit a Hawkes measurement after enough events", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, types.SourceHawkes)
		})
	})
}
