package cognition

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
)

func TestTrainNext(t *testing.T) {
	learn := func(memory *Associate, context string) {
		_, err := drive(NewTrain(memory), map[string]string{
			"context": context,
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)
	}

	winner := func(memory *Associate, context string) string {
		reading, err := drive(NewRecall(memory), map[string]string{
			"context": context,
		}, nil)
		So(err, ShouldBeNil)
		class, readErr := literal(reading, "winner")
		So(readErr, ShouldBeNil)
		return class
	}

	Convey("Given a slash-separated context", t, func() {
		memory := NewAssociate()
		learn(memory, "aa/bb/cc")

		Convey("A stored suffix recalls the class", func() {
			So(winner(memory, "cc"), ShouldEqual, "enter")
			So(winner(memory, "bb/cc"), ShouldEqual, "enter")
		})
	})

	Convey("Given an aligned context that also contains a slash", t, func() {
		memory := NewAssociate()
		learn(memory, "abcdefgh/ijklmno")
		reading, err := drive(NewSnapshot(memory), nil, nil)
		So(err, ShouldBeNil)
		model, modelErr := literal(reading, "model")
		So(modelErr, ShouldBeNil)

		decoder := gob.NewDecoder(bytes.NewReader([]byte(model)))
		var format string
		var count int
		So(decoder.Decode(&format), ShouldBeNil)
		So(decoder.Decode(&count), ShouldBeNil)
		keys := map[string]struct{}{}

		for range count {
			var key []byte
			var value []byte
			So(decoder.Decode(&key), ShouldBeNil)
			So(decoder.Decode(&value), ShouldBeNil)
			keys[string(key)] = struct{}{}
		}

		_, aligned := keys["b//ijklmno/enter"]
		_, separated := keys["b/ijklmno/enter"]
		So(aligned, ShouldBeTrue)
		So(separated, ShouldBeFalse)
		So(winner(memory, "/ijklmno"), ShouldEqual, "enter")
	})

	Convey("Given two length-framed timesteps", t, func() {
		first := make([]byte, 12)
		second := make([]byte, 12)
		binary.BigEndian.PutUint32(first[:4], 1)
		binary.BigEndian.PutUint32(second[:4], 1)
		copy(first[4:], []byte("aaaaaaa1"))
		copy(second[4:], []byte("bbbbbbb2"))
		memory := NewAssociate()
		learn(memory, string(append(first, second...)))

		Convey("The later frame recalls the class", func() {
			So(winner(memory, string(second)), ShouldEqual, "enter")
		})
	})

	Convey("Given an empty context", t, func() {
		_, err := drive(NewTrain(NewAssociate()), map[string]string{
			"context": "",
			"class":   "enter",
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})

		So(errors.Is(err, core.ErrDomain), ShouldBeTrue)
	})
}
