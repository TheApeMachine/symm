package model

import (
	"bytes"
	"encoding/gob"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestModelClone(t *testing.T) {
	Convey("A new agent inherits evidence without sharing mutable priors or pending work", t, func() {
		learned := New[string, string]()
		So(learned.Observe("task", []uint64{1, 2}, "work", -2, 1), ShouldBeNil)
		identity, err := learned.Issue("task", []uint64{1, 2}, "work", 1)
		So(err, ShouldBeNil)
		So(learned.Feedback(identity, 4), ShouldBeNil)
		cloned := learned.Clone()
		reading := cloned.Recall("task", []uint64{1, 2}, "work")
		So(reading.Mean, ShouldEqual, -2)
		So(reading.Pending, ShouldEqual, 0)
		So(cloned.Sequence, ShouldEqual, learned.Sequence)
		So(cloned.Feedback(identity, 1), ShouldNotBeNil)

		So(cloned.Observe("task", []uint64{1, 2}, "work", 6, 1), ShouldBeNil)
		So(cloned.Recall("task", []uint64{1, 2}, "work").Mean, ShouldEqual, 2)
		So(learned.Recall("task", []uint64{1, 2}, "work").Mean, ShouldEqual, -2)
		So(learned.Recall("task", []uint64{1, 2}, "work").Pending, ShouldEqual, 1)
	})
	Convey("Empty checkpoint branches are omitted without discarding provisional or zero outcomes", t, func() {
		learned := New[string, string]()
		identity, err := learned.Issue("symbol", []uint64{1, 2}, "enter", 1)
		So(err, ShouldBeNil)
		_, err = learned.Issue("symbol", []uint64{1, 3}, "enter", 1)
		So(err, ShouldBeNil)
		So(learned.Feedback(identity, -1), ShouldBeNil)
		So(learned.Observe("symbol", []uint64{4}, "wait", 0, 1), ShouldBeNil)

		cloned := learned.Clone()
		So(cloned.Contexts["symbol"].Children[1].Children[3], ShouldBeNil)
		So(cloned.Recall("symbol", []uint64{1, 2}, "enter").Mean, ShouldEqual, -1)
		So(cloned.Recall("symbol", []uint64{4}, "wait").Samples, ShouldEqual, 1)
		So(learned.Contexts["symbol"].Children[1].Children[3].Priors["enter"].pending, ShouldEqual, 1)
	})
}

func BenchmarkModelClone(b *testing.B) {
	learned := New[int, int]()

	// Multiple tasks and actions share prefixes but own separate evidence.
	for key := range 64 {
		for action := range 4 {
			if err := learned.Observe(key, []uint64{1, 2, 3}, action, -1, 1); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportAllocs()

	for b.Loop() {
		learned.Clone()
	}
}

/*
TestModelCloneConcurrent reproduces the live crash: the observation path
trains the trie while replay workers clone it and a checkpoint encodes it.
Before the trie owned its own access this aborted the process with "concurrent
map iteration and map write".
*/
func TestModelCloneConcurrent(t *testing.T) {
	Convey("Given one policy trained, cloned and encoded at the same time", t, func() {
		policy := New[string, string]()

		for index := range 64 {
			So(policy.Observe("BTC/USD", []uint64{uint64(index), uint64(index + 1)}, "enter", 1, 1), ShouldBeNil)
		}

		var workers sync.WaitGroup

		Convey("None of the three access paths corrupts the trie", func() {
			for worker := range 4 {
				workers.Go(func() {
					for index := range 256 {
						token := uint64(worker*1000 + index)

						if err := policy.Observe("BTC/USD", []uint64{token, token + 1}, "enter", 1, 1); err != nil {
							t.Error(err)

							return
						}
					}
				})
			}

			for range 4 {
				workers.Go(func() {
					for range 64 {
						if clone := policy.Clone(); clone == nil {
							t.Error("clone returned nothing")

							return
						}
					}
				})
			}

			workers.Go(func() {
				for range 64 {
					var data bytes.Buffer

					if err := policy.Encode(gob.NewEncoder(&data)); err != nil {
						t.Error(err)

						return
					}
				}
			})

			workers.Go(func() {
				for range 256 {
					policy.Recall("BTC/USD", []uint64{1, 2}, "enter")
				}
			})

			workers.Wait()

			So(len(policy.Contexts), ShouldBeGreaterThan, 0)
		})
	})
}
