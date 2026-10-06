package cognition

import (
	"bytes"
	"encoding/gob"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
)

func TestSnapshotNext(t *testing.T) {
	Convey("Given an association of ctx with enter", t, func() {
		memory := NewAssociate()
		_, err := drive(memory, map[string]string{
			"context": "ctx",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		reading, snapErr := drive(NewSnapshot(memory), nil, nil)
		So(snapErr, ShouldBeNil)
		model, modelErr := literal(reading, "model")
		So(modelErr, ShouldBeNil)

		decoder := gob.NewDecoder(bytes.NewReader([]byte(model)))
		var format string
		var count int
		So(decoder.Decode(&format), ShouldBeNil)
		So(decoder.Decode(&count), ShouldBeNil)
		So(format, ShouldEqual, "cognition/association/1")

		keys := map[string]struct{}{}

		for range count {
			var key []byte
			var value []byte
			So(decoder.Decode(&key), ShouldBeNil)
			So(decoder.Decode(&value), ShouldBeNil)
			keys[string(key)] = struct{}{}
		}

		_, basin := keys["b/ctx/enter"]
		_, sensory := keys["s/ctx"]
		So(basin, ShouldBeTrue)
		So(sensory, ShouldBeTrue)

		Convey("Restore into an empty trie recalls the class", func() {
			fresh := NewAssociate()
			_, restoreErr := drive(NewRestore(fresh), map[string]string{
				"model": model,
			}, nil)
			So(restoreErr, ShouldBeNil)

			recalled, recallErr := drive(NewRecall(fresh), map[string]string{
				"context": "ctx",
			}, nil)
			So(recallErr, ShouldBeNil)
			winner, winnerErr := literal(recalled, "winner")
			So(winnerErr, ShouldBeNil)
			So(winner, ShouldEqual, "enter")

			census, censusErr := drive(NewCensus(fresh), nil, nil)
			So(censusErr, ShouldBeNil)
			enter, enterErr := number(census, "enter")
			So(enterErr, ShouldBeNil)
			So(enter, ShouldEqual, 1)
		})
	})
}

func TestRestoreNext(t *testing.T) {
	Convey("Given a populated trie", t, func() {
		memory := NewAssociate()
		_, err := drive(memory, map[string]string{
			"context": "ctx",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)

		_, restoreErr := drive(NewRestore(memory), map[string]string{
			"model": "cognition/association/1",
		}, nil)
		So(errors.Is(restoreErr, core.ErrDomain), ShouldBeTrue)
	})

	Convey("Given a snapshot from another format", t, func() {
		_, err := drive(NewRestore(NewAssociate()), map[string]string{
			"model": "cognition/packed-weight/2",
		}, nil)
		So(err, ShouldNotBeNil)
	})
}
