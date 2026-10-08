package cognition

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

type DriveResult struct {
	Numbers map[string]float64
	Texts   map[string]string
}

func drive(
	primitive core.Primitive,
	text map[string]string,
	numbers map[string]float64,
) (*DriveResult, error) {
	result := &DriveResult{
		Numbers: make(map[string]float64),
		Texts:   make(map[string]string),
	}

	switch p := primitive.(type) {
	case *Associate:
		rec := &Record{
			Context:  text["context"],
			Class:    text["class"],
			Feedback: numbers["feedback"],
			Graded:   numbers["graded"],
		}
		var vals [2]float64
		idx := 0

		for ptr := range p.Next(data.NewValue(unsafe.Pointer(rec)).Next(nil)) {
			if idx < 2 {
				vals[idx] = *(*float64)(ptr)
				idx++
			}
		}

		if err := p.Error(); err != nil {
			return result, err
		}

		result.Numbers["records"] = vals[0]
		result.Numbers["span"] = vals[1]

	case *Reinforce:
		var updated float64

		for ptr := range p.Next(data.NewValue(numbers["probability"], numbers["count"], numbers["feedback"], numbers["graded"]).Next(nil)) {
			updated = *(*float64)(ptr)
		}

		if err := p.Error(); err != nil {
			return result, err
		}

		result.Numbers["probability"] = updated

	case *Recall:
		query := &RecallQuery{
			Context: text["context"],
			Stance:  text["stance"],
		}
		var res *RecallResult

		for ptr := range p.Next(data.NewValue(unsafe.Pointer(query)).Next(nil)) {
			res = (*RecallResult)(ptr)
		}

		if err := p.Error(); err != nil {
			return result, err
		}

		if res != nil {
			result.Texts["winner"] = res.Winner
			result.Texts["runner_up"] = res.RunnerUp
			result.Numbers["confidence"] = res.Confidence
			result.Numbers["contrast"] = res.Contrast
			result.Numbers["support"] = res.Support
			result.Numbers["ambiguity"] = res.Ambiguity

			if res.HasSurprisal {
				result.Numbers["surprisal"] = res.Surprisal
			}
		}

	case *Train:
		rec := &TrainRecord{
			Context:  text["context"],
			Class:    text["class"],
			Feedback: numbers["feedback"],
			Graded:   numbers["graded"],
		}
		var vals [2]float64
		idx := 0

		for ptr := range p.Next(data.NewValue(unsafe.Pointer(rec)).Next(nil)) {
			if idx < 2 {
				vals[idx] = *(*float64)(ptr)
				idx++
			}
		}

		if err := p.Error(); err != nil {
			return result, err
		}

		result.Numbers["records"] = vals[0]
		result.Numbers["span"] = vals[1]

	case *Census:
		var res *CensusResult

		for ptr := range p.Next(nil) {
			res = (*CensusResult)(ptr)
		}

		if err := p.Error(); err != nil {
			return result, err
		}

		if res != nil {
			result.Numbers["records"] = res.Records
			result.Numbers["span"] = res.Span

			for key, val := range res.Classes {
				result.Numbers[key] = val
			}
		}

	case *Snapshot:
		var model string

		for ptr := range p.Next(nil) {
			model = *(*string)(ptr)
		}

		if err := p.Error(); err != nil {
			return result, err
		}

		result.Texts["model"] = model

	case *Restore:
		modelStr := text["model"]
		var records float64

		for ptr := range p.Next(data.NewValue(modelStr).Next(nil)) {
			records = *(*float64)(ptr)
		}

		if err := p.Error(); err != nil {
			return result, err
		}

		result.Numbers["records"] = records

	case *Export:
		var tree string

		for ptr := range p.Next(nil) {
			tree = *(*string)(ptr)
		}

		if err := p.Error(); err != nil {
			return result, err
		}

		result.Texts["tree"] = tree
	}

	return result, nil
}

func number(result *DriveResult, key string) (float64, error) {
	val, ok := result.Numbers[key]

	if !ok {
		return 0, core.ErrNotHeld
	}

	return val, nil
}

func literal(result *DriveResult, key string) (string, error) {
	val, ok := result.Texts[key]

	if !ok {
		return "", core.ErrNotHeld
	}

	return val, nil
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
