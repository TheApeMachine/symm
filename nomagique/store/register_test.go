package store_test

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
slot is a test subject: the register stamps its identity through the query.
*/
type slot struct {
	id int
}

func (s *slot) Identity() int { return s.id }

func (s *slot) Identify(id int) data.Identifiable[int] {
	s.id = id

	return s
}

func TestRegister(t *testing.T) {
	Convey("Given an empty register", t, func() {
		register := store.NewRegister[int]()
		subject := &slot{}

		Convey("identify appends the payload and stamps the subject with its slot", func() {
			query := store.NewQuery(subject, data.ActionIdentify, data.NewValue(7))
			value := data.Read[int](register.Next(data.NewValue(*query)))

			So(subject.Identity(), ShouldEqual, 0)
			So(value, ShouldEqual, 7)
			So(register.Error(), ShouldBeNil)
		})

		Convey("a read outside the register is a shape failure", func() {
			query := store.NewQuery(subject, data.ActionRead, nil)
			data.Read[int](register.Next(data.NewValue(*query)))

			So(register.Error(), ShouldNotBeNil)
		})
	})

	Convey("Given a register with one identified slot", t, func() {
		register := store.NewRegister[int]()
		subject := &slot{}

		identify := store.NewQuery(subject, data.ActionIdentify, data.NewValue(7))
		data.Read[int](register.Next(data.NewValue(*identify)))

		Convey("a read returns the stored value", func() {
			query := store.NewQuery(subject, data.ActionRead, nil)
			value := data.Read[int](register.Next(data.NewValue(*query)))

			So(value, ShouldEqual, 7)
		})

		Convey("a write into the identified slot replaces the value", func() {
			write := store.NewQuery(subject, data.ActionWrite, data.NewValue(9))
			data.Read[int](register.Next(data.NewValue(*write)))

			query := store.NewQuery(subject, data.ActionRead, nil)
			value := data.Read[int](register.Next(data.NewValue(*query)))

			So(value, ShouldEqual, 9)
			So(register.Error(), ShouldBeNil)
		})

		Convey("a write from an unidentified subject is a shape failure", func() {
			stranger := &slot{id: 3}
			write := store.NewQuery(stranger, data.ActionWrite, data.NewValue(9))
			data.Read[int](register.Next(data.NewValue(*write)))

			So(register.Error(), ShouldNotBeNil)
		})
	})
}

type measSlot struct {
	id int
}

func (slotItem *measSlot) Identity() int { return slotItem.id }

func (slotItem *measSlot) Identify(id int) data.Identifiable[*data.Measurement[float64]] {
	slotItem.id = id
	return slotItem
}

func TestRegisterPeers(t *testing.T) {
	Convey("Given a register populated with measurement slots", t, func() {
		register := store.NewRegister[*data.Measurement[float64]]()
		const nodeCount = 4
		slots := make([]*measSlot, nodeCount)

		for index := 0; index < nodeCount; index++ {
			slots[index] = &measSlot{}
			meas := data.NewMeasurement(fmt.Sprintf("node-%d", index), map[string]data.Metric[float64]{
				"price": data.NewMetric[float64]("price", data.UnitDimensionless, data.TimescaleInstantaneous, float64(100+index), float64(100+index)),
			})
			meas.Label = "BTC/USD"
			meas.Metadata["peer-interest"] = "*"

			identify := store.NewQuery(slots[index], data.ActionIdentify, data.NewValue(meas))
			data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*identify)))
		}

		Convey("a read populates peers from matching slots", func() {
			query := store.NewQuery(slots[0], data.ActionRead, nil)
			val := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*query)))

			So(val, ShouldNotBeNil)
			So(len(val.Peers), ShouldEqual, 3)
			So(val.Peers[0].Source, ShouldEqual, "node-1")
			So(val.Peers[1].Source, ShouldEqual, "node-2")
			So(val.Peers[2].Source, ShouldEqual, "node-3")
		})

		Convey("peer limit restricts the scan range", func() {
			query := store.NewQuery(slots[2], data.ActionRead, nil)
			query.SetPeerLimit(2)
			val := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*query)))

			So(val, ShouldNotBeNil)
			So(len(val.Peers), ShouldEqual, 2)
			So(val.Peers[0].Source, ShouldEqual, "node-0")
			So(val.Peers[1].Source, ShouldEqual, "node-1")
		})

		Convey("a later write to a cloned working copy does not mutate a live peer snapshot", func() {
			readPeer := store.NewQuery(slots[0], data.ActionRead, nil)
			holder := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*readPeer)))
			So(holder, ShouldNotBeNil)
			So(len(holder.Peers), ShouldEqual, 3)

			snapshot := holder.Peers[0]
			So(snapshot.Source, ShouldEqual, "node-1")
			So(snapshot.Metrics["price"].Center, ShouldEqual, 101)

			workingQuery := store.NewQuery(slots[1], data.ActionRead, nil)
			working := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*workingQuery)))
			So(working, ShouldNotBeNil)
			working.Metrics["price"] = data.NewMetric[float64](
				"price", data.UnitDimensionless, data.TimescaleInstantaneous, 999, 999,
			)

			write := store.NewQuery(slots[1], data.ActionWrite, data.NewValue(working))
			data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*write)))

			So(snapshot.Metrics["price"].Center, ShouldEqual, 101)

			reread := store.NewQuery(slots[0], data.ActionRead, nil)
			updated := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*reread)))
			So(updated.Peers[0].Metrics["price"].Center, ShouldEqual, 999)
		})

		Convey("sequential read-write cycles update the slot value", func() {
			for iterCount := 0; iterCount < 10; iterCount++ {
				readQuery := store.NewQuery(slots[0], data.ActionRead, nil)
				val := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*readQuery)))
				So(val, ShouldNotBeNil)

				val.Metrics["price"] = data.NewMetric[float64](
					"price", data.UnitDimensionless, data.TimescaleInstantaneous,
					float64(200+iterCount), float64(200+iterCount),
				)

				writeQuery := store.NewQuery(slots[0], data.ActionWrite, data.NewValue(val))
				data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*writeQuery)))
			}

			finalQuery := store.NewQuery(slots[0], data.ActionRead, nil)
			final := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*finalQuery)))
			So(final, ShouldNotBeNil)
			So(final.Metrics["price"].Center, ShouldEqual, 209)
			So(register.Error(), ShouldBeNil)
		})
	})
}
