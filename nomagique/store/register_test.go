package store_test

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/store"
)

func TestRegister(t *testing.T) {
	Convey("Given an empty register", t, func() {
		register := store.NewRegister[int]()
		subject := store.NewQuery[int, int](nil, data.ActionNone)

		Convey("identify appends the payload and stamps the subject with its slot", func() {
			query := store.NewQuery[int, int](subject, data.ActionIdentify, sequence.NewValue(7))
			value := sequence.Read[int](register.Next(sequence.NewValue(*query)))

			So(subject.Identity(), ShouldEqual, 0)
			So(value, ShouldEqual, 7)
			So(register.Error(), ShouldBeNil)
		})

		Convey("a read outside the register is a shape failure", func() {
			query := store.NewQuery[int, int](subject, data.ActionRead, nil)
			sequence.Read[int](register.Next(sequence.NewValue(*query)))

			So(register.Error(), ShouldNotBeNil)
		})
	})

	Convey("Given a register with one identified slot", t, func() {
		register := store.NewRegister[int]()
		subject := store.NewQuery[int, int](nil, data.ActionNone)

		identify := store.NewQuery[int, int](subject, data.ActionIdentify, sequence.NewValue(7))
		sequence.Read[int](register.Next(sequence.NewValue(*identify)))

		Convey("a read returns the stored value", func() {
			query := store.NewQuery[int, int](subject, data.ActionRead, nil)
			value := sequence.Read[int](register.Next(sequence.NewValue(*query)))

			So(value, ShouldEqual, 7)
		})

		Convey("a write into the identified slot replaces the value", func() {
			write := store.NewQuery[int, int](subject, data.ActionWrite, sequence.NewValue(9))
			sequence.Read[int](register.Next(sequence.NewValue(*write)))

			query := store.NewQuery[int, int](subject, data.ActionRead, nil)
			value := sequence.Read[int](register.Next(sequence.NewValue(*query)))

			So(value, ShouldEqual, 9)
			So(register.Error(), ShouldBeNil)
		})

		Convey("a write from an unidentified subject is a shape failure", func() {
			stranger := &store.Query[int, int]{Address: 3}
			write := store.NewQuery[int, int](stranger, data.ActionWrite, sequence.NewValue(9))
			sequence.Read[int](register.Next(sequence.NewValue(*write)))

			So(register.Error(), ShouldNotBeNil)
		})
	})
}

func TestRegisterPeers(t *testing.T) {
	Convey("Given a register populated with measurement slots", t, func() {
		register := store.NewRegister[*data.Measurement[float64]]()
		const nodeCount = 4
		slots := make([]*store.Query[int, *data.Measurement[float64]], nodeCount)

		for index := 0; index < nodeCount; index++ {
			slots[index] = store.NewQuery[int, *data.Measurement[float64]](nil, data.ActionNone)
			meas := data.NewMeasurement(fmt.Sprintf("node-%d", index), map[string]data.Metric[float64]{
				"price": data.NewMetric[float64]("price", data.UnitDimensionless, data.TimescaleInstantaneous, float64(100+index), float64(100+index)),
			})
			meas.Label = "BTC/USD"
			meas.Metadata["peer-interest"] = "*"

			identify := store.NewQuery[int, *data.Measurement[float64]](slots[index], data.ActionIdentify, sequence.NewValue(meas))
			sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*identify)))
		}

		Convey("a read populates peers from matching slots", func() {
			query := store.NewQuery[int, *data.Measurement[float64]](slots[0], data.ActionRead, nil)
			val := sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*query)))

			So(val, ShouldNotBeNil)
			So(len(val.Peers), ShouldEqual, 3)
			So(val.Peers[0].Source, ShouldEqual, "node-1")
			So(val.Peers[1].Source, ShouldEqual, "node-2")
			So(val.Peers[2].Source, ShouldEqual, "node-3")
		})

		Convey("peer limit restricts the scan range", func() {
			query := store.NewQuery[int, *data.Measurement[float64]](slots[2], data.ActionRead, nil)
			query.SetPeerLimit(2)
			val := sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*query)))

			So(val, ShouldNotBeNil)
			So(len(val.Peers), ShouldEqual, 2)
			So(val.Peers[0].Source, ShouldEqual, "node-0")
			So(val.Peers[1].Source, ShouldEqual, "node-1")
		})

		Convey("a later write to a cloned working copy does not mutate a live peer snapshot", func() {
			readPeer := store.NewQuery[int, *data.Measurement[float64]](slots[0], data.ActionRead, nil)
			holder := sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*readPeer)))
			So(holder, ShouldNotBeNil)
			So(len(holder.Peers), ShouldEqual, 3)

			snapshot := holder.Peers[0]
			So(snapshot.Source, ShouldEqual, "node-1")
			So(snapshot.Metrics["price"].Center, ShouldEqual, 101)

			workingQuery := store.NewQuery[int, *data.Measurement[float64]](slots[1], data.ActionRead, nil)
			working := sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*workingQuery)))
			So(working, ShouldNotBeNil)
			working.Metrics["price"] = data.NewMetric[float64](
				"price", data.UnitDimensionless, data.TimescaleInstantaneous, 999, 999,
			)

			write := store.NewQuery[int, *data.Measurement[float64]](slots[1], data.ActionWrite, sequence.NewValue(working))
			sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*write)))

			So(snapshot.Metrics["price"].Center, ShouldEqual, 101)

			reread := store.NewQuery[int, *data.Measurement[float64]](slots[0], data.ActionRead, nil)
			updated := sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*reread)))
			So(updated.Peers[0].Metrics["price"].Center, ShouldEqual, 999)
		})

		Convey("sequential read-write cycles update the slot value", func() {
			for iterCount := 0; iterCount < 10; iterCount++ {
				readQuery := store.NewQuery[int, *data.Measurement[float64]](slots[0], data.ActionRead, nil)
				val := sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*readQuery)))
				So(val, ShouldNotBeNil)

				val.Metrics["price"] = data.NewMetric[float64](
					"price", data.UnitDimensionless, data.TimescaleInstantaneous,
					float64(200+iterCount), float64(200+iterCount),
				)

				writeQuery := store.NewQuery[int, *data.Measurement[float64]](slots[0], data.ActionWrite, sequence.NewValue(val))
				sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*writeQuery)))
			}

			finalQuery := store.NewQuery[int, *data.Measurement[float64]](slots[0], data.ActionRead, nil)
			final := sequence.Read[*data.Measurement[float64]](register.Next(sequence.NewValue(*finalQuery)))
			So(final, ShouldNotBeNil)
			So(final.Metrics["price"].Center, ShouldEqual, 209)
			So(register.Error(), ShouldBeNil)
		})
	})
}
