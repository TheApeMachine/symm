package store_test

import (
	"fmt"
	"sync"
	"sync/atomic"
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
			query := store.NewQuery[int](subject, data.ActionIdentify, 7)
			value := data.Read[int](register.Next(data.NewValue(*query)))

			So(subject.Identity(), ShouldEqual, 0)
			So(value, ShouldEqual, 7)
			So(register.Error(), ShouldBeNil)
		})

		Convey("a read outside the register is a shape failure", func() {
			query := store.NewQuery[int](subject, data.ActionRead)
			data.Read[int](register.Next(data.NewValue(*query)))

			So(register.Error(), ShouldNotBeNil)
		})
	})

	Convey("Given a register with one identified slot", t, func() {
		register := store.NewRegister[int]()
		subject := &slot{}

		identify := store.NewQuery[int](subject, data.ActionIdentify, 7)
		data.Read[int](register.Next(data.NewValue(*identify)))

		Convey("a read returns the stored value", func() {
			query := store.NewQuery[int](subject, data.ActionRead)
			value := data.Read[int](register.Next(data.NewValue(*query)))

			So(value, ShouldEqual, 7)
		})

		Convey("a write into the identified slot replaces the value", func() {
			write := store.NewQuery[int](subject, data.ActionWrite, 9)
			data.Read[int](register.Next(data.NewValue(*write)))

			query := store.NewQuery[int](subject, data.ActionRead)
			value := data.Read[int](register.Next(data.NewValue(*query)))

			So(value, ShouldEqual, 9)
			So(register.Error(), ShouldBeNil)
		})

		Convey("a write from an unidentified subject is a shape failure", func() {
			stranger := &slot{id: 3}
			write := store.NewQuery[int](stranger, data.ActionWrite, 9)
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

func TestRegisterConcurrency(t *testing.T) {
	Convey("Given a register populated with measurement slots", t, func() {
		register := store.NewRegister[*data.Measurement[float64]]()
		const nodeCount = 16
		slots := make([]*measSlot, nodeCount)

		for index := 0; index < nodeCount; index++ {
			slots[index] = &measSlot{}
			meas := data.NewMeasurement[float64](fmt.Sprintf("node-%d", index), map[string]data.Metric[float64]{
				"price": data.NewMetric[float64]("price", data.UnitDimensionless, data.TimescaleInstantaneous, 100, 100),
				"qty":   data.NewMetric[float64]("qty", data.UnitDimensionless, data.TimescaleInstantaneous, 1, 1),
			})
			meas.Label = "BTC/USD"
			meas.Metadata["peer-interest"] = "*"

			identify := store.NewQuery[*data.Measurement[float64]](slots[index], data.ActionIdentify, meas)
			data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*identify)))
		}

		Convey("concurrent reads and writes execute without race conditions or map panics", func() {
			var waitGroup sync.WaitGroup
			var nilCount atomic.Int64
			iterations := 100

			for index := 0; index < nodeCount; index++ {
				slotIndex := index
				waitGroup.Add(2)

				go func() {
					defer waitGroup.Done()

					for iterCount := 0; iterCount < iterations; iterCount++ {
						query := store.NewQuery[*data.Measurement[float64]](slots[slotIndex], data.ActionRead)
						query.SetPeerLimit(3)
						val := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*query)))

						if val != nil {
							val.Metrics["price"] = data.NewMetric[float64](
								"price", data.UnitDimensionless, data.TimescaleInstantaneous,
								float64(100+iterCount), float64(100+iterCount),
							)
						}

						writeQuery := store.NewQuery[*data.Measurement[float64]](slots[slotIndex], data.ActionWrite, val)
						data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*writeQuery)))
					}
				}()

				go func() {
					defer waitGroup.Done()

					for iterCount := 0; iterCount < iterations; iterCount++ {
						query := store.NewQuery[*data.Measurement[float64]](slots[slotIndex], data.ActionRead)
						query.SetPeerLimit(3)
						val := data.Read[*data.Measurement[float64]](register.Next(data.NewValue(*query)))

						if val == nil {
							nilCount.Add(1)
						}
					}
				}()
			}

			waitGroup.Wait()
			So(nilCount.Load(), ShouldEqual, 0)
			So(register.Error(), ShouldBeNil)
		})
	})
}
