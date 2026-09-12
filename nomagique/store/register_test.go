package store_test

import (
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
