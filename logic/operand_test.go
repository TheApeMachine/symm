package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
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
			measurement := datura.Acquire("pumpdump", datura.APPJSON).
				WithRole("measurement").
				WithScope("BTC/USD").
				WithPayload(datura.Map[any]{
					"output": datura.Map[any]{
						"value":      float64(CategoryIndex(CategoryOrganicTrend)),
						"confidence": 0.25,
					},
				}.Marshal())

			value, err := operand.Resolve([]*datura.Artifact{measurement}, nil)

			Convey("Then the operand is false without treating absent source evidence as malformed data", func() {
				So(err, ShouldBeNil)
				So(value, ShouldEqual, 0)
			})
		})
	})

	Convey("Given an eigenmode operand", t, func() {
		operand := ConditionOperand{
			Type: SubjectEigenmode,
			Eigenmode: datura.Map[any]{
				"mode": "momentum",
			},
		}

		Convey("When the measurements do not carry eigenmode evidence", func() {
			measurement := datura.Acquire("cvd", datura.APPJSON).
				WithRole("measurement").
				WithScope("BTC/USD").
				WithPayload(datura.Map[any]{
					"output": datura.Map[any]{
						"confidence": 0.25,
					},
				}.Marshal())

			value, err := operand.Resolve([]*datura.Artifact{measurement}, nil)

			Convey("Then the operand is false without hiding malformed eigenmode payloads", func() {
				So(err, ShouldBeNil)
				So(value, ShouldEqual, 0)
			})
		})

		Convey("When the eigenmode payload is structurally invalid", func() {
			measurement := datura.Acquire("manifold", datura.APPJSON).
				WithRole("measurement").
				WithScope("BTC/USD").
				WithPayload(datura.Map[any]{
					"output": datura.Map[any]{
						"eigenmode": datura.Map[any]{
							"labels":    []string{"momentum"},
							"origins":   []float64{1},
							"energies":  []float64{1},
							"coupling":  []float64{},
							"threshold": 0.5,
						},
					},
				}.Marshal())

			_, err := operand.Resolve([]*datura.Artifact{measurement}, nil)

			Convey("Then the invalid payload is still rejected", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}
