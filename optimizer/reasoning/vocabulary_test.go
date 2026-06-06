package reasoning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func frequentDownRows() []types.Measurement {
	categories := []types.CategoryType{
		types.CategoryDenseNeutrality,
		types.CategoryThermalExhaustion,
		types.CategorySystemicBeta,
		types.CategoryMechanicalCollapse,
		types.CategoryFadedExhaustion,
		types.CategoryCausalNoise,
		types.CategoryLaminar,
	}

	price := 100.0
	rows := make([]types.Measurement, 0, len(categories)*4+2)

	for _, category := range categories {
		for range 2 {
			rows = append(rows, types.Measurement{
				Symbol:   "BTC/EUR",
				Category: category,
				SNR:      2,
				Last:     price,
			})

			price *= 0.99
			rows = append(rows, types.Measurement{
				Symbol: "BTC/EUR",
				Last:   price,
			})
		}
	}

	rows = append(rows,
		types.Measurement{
			Symbol:   "BTC/EUR",
			Category: types.CategoryVerticalIgnition,
			SNR:      1.5,
			Last:     price,
		},
		types.Measurement{
			Symbol: "BTC/EUR",
			Last:   price * 1.10,
		},
	)

	return rows
}

func TestDeriveVocabularyRanksForwardEdge(t *testing.T) {
	Convey("Given many frequent categories and one rare category before an up move", t, func() {
		vocab := DeriveVocabulary(frequentDownRows())

		Convey("It should keep the rare forward-edge category inside the seed cap", func() {
			So(len(vocab.Categories), ShouldEqual, maxSeedCategories)
			So(vocab.Categories, ShouldContain, types.CategoryVerticalIgnition)
			So(vocab.Categories[0], ShouldEqual, types.CategoryVerticalIgnition)
		})
	})
}
