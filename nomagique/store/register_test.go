package store_test

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestRegister(t *testing.T) {
	Convey("Given an empty register", t, func() {
		register := store.NewRegister[int]()
		subject := transport.NewAddress[int]()

		Convey("identify appends the payload and stamps the subject with its slot", func() {
			query := store.NewQuery[int, int](subject, core.Identify)
			value := sequence.Read[int](register.Next(query.Next(sequence.NewValue(7))))

			So(subject.Identity(), ShouldEqual, 0)
			So(value, ShouldEqual, 7)
			So(register.Error(), ShouldBeNil)
		})

		Convey("a read outside the register is a shape failure", func() {
			subject.Identify(5)
			query := store.NewQuery[int, int](subject, core.Read)
			sequence.Read[int](register.Next(query.Next(nil)))

			So(register.Error(), ShouldNotBeNil)
		})
	})

	Convey("Given a register with one identified slot", t, func() {
		register := store.NewRegister[int]()
		subject := transport.NewAddress[int]()

		identify := store.NewQuery[int, int](subject, core.Identify)
		sequence.Read[int](register.Next(identify.Next(sequence.NewValue(7))))

		Convey("a read returns the stored value", func() {
			query := store.NewQuery[int, int](subject, core.Read)
			value := sequence.Read[int](register.Next(query.Next(nil)))

			So(value, ShouldEqual, 7)
		})

		Convey("a write into the identified slot replaces the value", func() {
			write := store.NewQuery[int, int](subject, core.Write)
			sequence.Read[int](register.Next(write.Next(sequence.NewValue(9))))

			query := store.NewQuery[int, int](subject, core.Read)
			value := sequence.Read[int](register.Next(query.Next(nil)))

			So(value, ShouldEqual, 9)
			So(register.Error(), ShouldBeNil)
		})
	})
}

func TestRegisterPeers(t *testing.T) {
	Convey("Given a register populated with measurement slots", t, func() {
		register := store.NewRegister[*data.Measurement[float64]]()
		const nodeCount = 4
		subjects := make([]*transport.Address[int], nodeCount)

		for index := 0; index < nodeCount; index++ {
			subjects[index] = transport.NewAddress[int]()
			meas := data.NewMeasurement(fmt.Sprintf("node-%d", index), map[string]data.Metric[float64]{
				"price": data.NewMetric[float64]("price", data.UnitDimensionless, data.TimescaleInstantaneous, float64(100+index), float64(100+index)),
			})
			meas.Label = "BTC/USD"
			meas.Metadata["peer-interest"] = "*"

			identify := store.NewQuery[int, *data.Measurement[float64]](subjects[index], core.Identify)
			sequence.Read[*data.Measurement[float64]](register.Next(identify.Next(sequence.NewValue(meas))))
		}

		Convey("a read populates peers from matching slots", func() {
			query := store.NewQuery[int, *data.Measurement[float64]](subjects[0], core.Read)
			val := sequence.Read[*data.Measurement[float64]](register.Next(query.Next(nil)))

			So(val, ShouldNotBeNil)
			So(len(val.Peers), ShouldEqual, 3)
			So(val.Peers[0].Source, ShouldEqual, "node-1")
			So(val.Peers[1].Source, ShouldEqual, "node-2")
			So(val.Peers[2].Source, ShouldEqual, "node-3")
		})
	})
}
