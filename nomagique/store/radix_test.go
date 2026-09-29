package store_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
)

func TestRadixNext(t *testing.T) {
	Convey("Radix retains exact writes and publishes immutable tree snapshots", t, func() {
		op := store.NewRadix()
		writes := []map[string][]byte{
			{"selector": []byte("b/enter\x00abc"), "data": []byte("one")},
			{"selector": []byte("b/exit\x00abc"), "data": []byte("two")},
			{"selector": []byte("b/")},
		}
		input := func(yield func(unsafe.Pointer) bool) {
			for _, write := range writes {
				if !yield(unsafe.Pointer(&write)) {
					return
				}
			}
		}
		output := tests.CollectSeq[*iradix.Tree[[]byte]](op.Next(input))
		So(len(output), ShouldEqual, 3)
		So(output[0].Len(), ShouldEqual, 1)
		So(output[1].Len(), ShouldEqual, 2)
		So(output[2].Len(), ShouldEqual, 2)
		So(op.Error(), ShouldBeNil)
	})
}

func TestRadixGet(t *testing.T) {
	Convey("Only explicitly stored contexts return an association", t, func() {
		op := store.NewRadix()
		op.Insert([]byte{1, 2, 3}, []byte("enter"))
		value, found := op.Get([]byte{1, 2, 3})
		So(found, ShouldBeTrue)
		So(string(value), ShouldEqual, "enter")

		for _, unseen := range [][]byte{{2, 3}, {9, 2, 3}, {1, 2}, {1, 2, 3, 4}} {
			value, found := op.Get(unseen)
			So(found, ShouldBeFalse)
			So(value, ShouldBeNil)
		}

		So(op.Tree().Len(), ShouldEqual, 1)
	})
}

func TestRadixInsert(t *testing.T) {
	Convey("The store owns its bytes and reads cannot mutate the model", t, func() {
		op := store.NewRadix()
		key, value := []byte{1, 2, 3}, []byte("enter")
		op.Insert(key, value)
		key[0], value[0] = 9, 'x'
		read, found := op.Get([]byte{1, 2, 3})
		So(found, ShouldBeTrue)
		So(string(read), ShouldEqual, "enter")
		read[0] = 'x'
		read, found = op.Get([]byte{1, 2, 3})
		So(found, ShouldBeTrue)
		So(string(read), ShouldEqual, "enter")
	})
}

func TestRadixMarshalJSON(t *testing.T) {
	Convey("Checkpoint round trips every possible byte without UTF-8 key collisions", t, func() {
		op := store.NewRadix()

		for index := 0; index < 256; index++ {
			op.Insert([]byte{byte(index), 0, 255}, []byte(fmt.Sprint(index)))
		}

		payload, err := json.Marshal(op)
		So(err, ShouldBeNil)
		So(bytes.Contains(payload, []byte(`\ufffd`)), ShouldBeFalse)
		restored := store.NewRadix()
		So(json.Unmarshal(payload, restored), ShouldBeNil)
		So(restored.Tree().Len(), ShouldEqual, 256)

		for index := 0; index < 256; index++ {
			value, found := restored.Get([]byte{byte(index), 0, 255})
			So(found, ShouldBeTrue)
			So(string(value), ShouldEqual, fmt.Sprint(index))
		}

		again, err := json.Marshal(restored)
		So(err, ShouldBeNil)
		So(bytes.Equal(payload, again), ShouldBeTrue)
	})
}

func TestRadixUnmarshalJSON(t *testing.T) {
	Convey("Invalid or lossy checkpoints leave the current model intact", t, func() {
		op := store.NewRadix()
		op.Insert([]byte("kept"), []byte("wait"))

		for _, payload := range []string{
			`{"\ufffd":"d2FpdA=="}`,
			`{"format":"symm-radix/1","entries":null}`,
			`{"format":"symm-radix/1","entries":[{"key":"%%%","value":""}]}`,
			`{"format":"symm-radix/1","entries":[{"key":"AQ==","value":""},{"key":"AQ==","value":""}]}`,
		} {
			So(json.Unmarshal([]byte(payload), op), ShouldNotBeNil)
			value, found := op.Get([]byte("kept"))
			So(found, ShouldBeTrue)
			So(string(value), ShouldEqual, "wait")
			So(op.Tree().Len(), ShouldEqual, 1)
		}
	})
}

func TestRadixConcurrent(t *testing.T) {
	Convey("Writes, exact reads, and checkpoint snapshots can run concurrently", t, func() {
		op := store.NewRadix()
		var workers sync.WaitGroup
		failures := make(chan error, 4)

		for worker := 0; worker < 4; worker++ {
			workers.Add(1)
			go func(worker int) {
				defer workers.Done()

				for index := 0; index < 64; index++ {
					key := []byte{byte(worker), byte(index), 128}
					op.Insert(key, key)
					value, found := op.Get(key)

					if !found || !bytes.Equal(value, key) {
						failures <- fmt.Errorf("lost exact key %x", key)
						return
					}

					if _, err := json.Marshal(op); err != nil {
						failures <- err
						return
					}
				}
			}(worker)
		}

		workers.Wait()
		close(failures)

		for err := range failures {
			So(err, ShouldBeNil)
		}

		So(op.Tree().Len(), ShouldEqual, 256)
	})
}
