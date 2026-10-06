package cognition

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func drive(
	primitive core.Primitive,
	text map[string]string,
	numbers map[string]float64,
) (*data.Adapter, error) {
	adapter := data.NewAdapter(nil, data.NewState(data.NewMap()))

	if len(text) > 0 {
		issued := data.NewTextMap()

		for key, value := range text {
			issued.Values[key] = value
		}

		for range adapter.Next(data.NewValue(issued)) {
		}

		if err := adapter.Error(); err != nil {
			return adapter, err
		}
	}

	if len(numbers) > 0 {
		issued := data.NewOutputMap()

		for key, value := range numbers {
			issued.Values[key] = value
		}

		for range adapter.Next(data.NewValue(issued)) {
		}

		if err := adapter.Error(); err != nil {
			return adapter, err
		}
	}

	for range primitive.Next(data.NewValue(adapter)) {
	}

	if err := primitive.Error(); err != nil {
		return adapter, err
	}

	return adapter, adapter.Error()
}

func number(adapter *data.Adapter, key string) (float64, error) {
	var values data.Map[float64]

	for pointer := range adapter.Next(data.NewValue(data.NewMap(key, key))) {
		values = *(*data.Map[float64])(pointer)
	}

	if err := adapter.Error(); err != nil {
		return 0, err
	}

	return values.Values[key], nil
}

func literal(adapter *data.Adapter, key string) (string, error) {
	var values data.Map[string]

	for pointer := range adapter.Next(data.NewValue(data.NewLiteral(key))) {
		values = *(*data.Map[string])(pointer)
	}

	if err := adapter.Error(); err != nil {
		return "", err
	}

	return values.Values[key], nil
}

func TestAssociateNext(t *testing.T) {
	Convey("Given an association of ctx with enter", t, func() {
		memory := NewAssociate()
		reading, err := drive(memory, map[string]string{
			"context": "ctx",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})

		So(err, ShouldBeNil)
		records, recordsErr := number(reading, "records")
		So(recordsErr, ShouldBeNil)
		So(records, ShouldEqual, 2)

		Convey("Recall reads that class back", func() {
			recalled, recallErr := drive(NewRecall(memory), map[string]string{
				"context": "ctx",
				"stance":  "",
			}, nil)

			So(recallErr, ShouldBeNil)
			winner, winnerErr := literal(recalled, "winner")
			So(winnerErr, ShouldBeNil)
			So(winner, ShouldEqual, "enter")
		})
	})

	Convey("Given an empty context", t, func() {
		_, err := drive(NewAssociate(), map[string]string{
			"context": "",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})

		So(errors.Is(err, core.ErrDomain), ShouldBeTrue)
	})

	Convey("Given graded feedback of zero", t, func() {
		memory := NewAssociate()
		_, err := drive(memory, map[string]string{
			"context": "ctx",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 0,
			"graded":   core.Unit,
		})

		So(err, ShouldBeNil)

		recalled, recallErr := drive(NewRecall(memory), map[string]string{
			"context": "ctx",
			"stance":  "",
		}, nil)
		So(recallErr, ShouldBeNil)
		winner, winnerErr := literal(recalled, "winner")
		So(winnerErr, ShouldBeNil)
		So(winner, ShouldEqual, "")

		census, censusErr := drive(NewCensus(memory), nil, nil)
		So(censusErr, ShouldBeNil)
		records, recordsErr := number(census, "records")
		So(recordsErr, ShouldBeNil)
		So(records, ShouldEqual, 1)
	})
}

func TestAssociateRejectsWait(t *testing.T) {
	Convey("Given a wait class", t, func() {
		_, err := drive(NewAssociate(), map[string]string{
			"context": "R0/R1",
			"class":   "wait",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})

		So(errors.Is(err, core.ErrDomain), ShouldBeTrue)
	})
}

func TestAssociatePrunesUselessBasin(t *testing.T) {
	Convey("Given an enter basin dampened below the useful floor", t, func() {
		memory := NewAssociate()
		_, err := drive(memory, map[string]string{
			"context": "R0/R1",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		// Strong negative feedback: 0.75 / (1+16) = 0.044 < basinFloor (1/16).
		_, err = drive(memory, map[string]string{
			"context": "R0/R1",
			"class":   "enter",
		}, map[string]float64{
			"feedback": -16,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		Convey("Recall abstains — the losing sequence was pruned", func() {
			recalled, recallErr := drive(NewRecall(memory), map[string]string{
				"context": "R0/R1",
				"stance":  "",
			}, nil)
			So(recallErr, ShouldBeNil)
			winner, winnerErr := literal(recalled, "winner")
			So(winnerErr, ShouldBeNil)
			So(winner, ShouldEqual, "")
		})
	})
}
