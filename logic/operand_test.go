package logic

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConditionOperandResolve(t *testing.T) {
	Convey("Given a source-specific category operand", t, func() {
		operand := ConditionOperand{
			Source: SourceCVD,
			Type:   SubjectCategory,
			Category: &Category{
				Type: CategoryAggressiveDrive,
			},
		}

		Convey("When the current symbol has not received that source yet", func() {
			measurement := &Measurement{
				Source:       SourcePumpDump,
				Symbol:       "BTC/USD",
				At:           time.Now(),
				Distribution: map[CategoryType]float64{CategoryOrganicTrend: 0.25},
				Confidence:   0.25,
			}

			value, err := operand.Resolve([]*Measurement{measurement}, nil)

			Convey("Then the operand is false without treating absent source evidence as malformed data", func() {
				So(err, ShouldBeNil)
				So(value, ShouldEqual, 0)
			})
		})
	})

	Convey("Given an eigenmode operand", t, func() {
		operand := ConditionOperand{
			Type: SubjectEigenmode,
			Eigenmode: map[string]any{
				"mode": "momentum",
			},
		}

		Convey("When the measurements do not carry eigenmode evidence", func() {
			measurement := &Measurement{
				Source:       SourceCVD,
				Symbol:       "BTC/USD",
				At:           time.Now(),
				Distribution: map[CategoryType]float64{CategoryAggressiveDrive: 0.25},
				Confidence:   0.25,
			}

			value, err := operand.Resolve([]*Measurement{measurement}, nil)

			Convey("Then the operand is false without hiding malformed eigenmode payloads", func() {
				So(err, ShouldBeNil)
				So(value, ShouldEqual, 0)
			})
		})

		Convey("When the eigenmode payload is structurally invalid", func() {
			measurement := &Measurement{
				Source:       SourceManifold,
				Symbol:       "BTC/USD",
				At:           time.Now(),
				Distribution: map[CategoryType]float64{CategorySystemicHerd: 0.25},
				Confidence:   0.25,
				Eigenmode: Eigenmode{
					Labels:    []string{"momentum"},
					Origins:   []float64{1},
					Energies:  []float64{1},
					Coupling:  []float64{},
					Threshold: 0.5,
				},
			}

			_, err := operand.Resolve([]*Measurement{measurement}, nil)

			Convey("Then the invalid payload is still rejected", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}
