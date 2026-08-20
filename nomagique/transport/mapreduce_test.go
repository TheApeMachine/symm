package transport

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func Setup[T any](
	consumerIDs []string,
	mapFn func(T) T,
	reduceFn func(T, T) T,
) *MapReduce[T] {
	consumers := make([]*Consumer[T], 0, len(consumerIDs))

	for _, consumerID := range consumerIDs {
		consumers = append(consumers, NewConsumer[T](consumerID, func() {}))
	}

	mr := NewMapReduce(consumers, mapFn, reduceFn)
	return mr
}

func TestNewMapReduce(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("Given valid consumer IDs", func() {
			Convey("When creating a MapReduce instance", func() {
				mr := Setup[int]([]string{"A", "B"}, nil, nil)

				Convey("Then it should initialize correctly", func() {
					So(mr, ShouldNotBeNil)
					So(len(mr.consumers), ShouldEqual, 2)
					So(mr.mapFn, ShouldNotBeNil)
					So(mr.reduceFn, ShouldNotBeNil)
				})
			})
		})

		Convey("Given empty consumer IDs", func() {
			Convey("When creating a MapReduce instance", func() {
				mr := Setup[int]([]string{}, nil, nil)

				Convey("Then it should initialize with empty slices", func() {
					So(mr, ShouldNotBeNil)
					So(len(mr.consumers), ShouldEqual, 0)
				})
			})
		})

		Convey("Given nil map and reduce functions", func() {
			Convey("When creating a MapReduce instance", func() {
				mr := Setup[int]([]string{"A"}, nil, nil)

				Convey("Then it should use noop defaults", func() {
					mr.Push(42)
					item, ok := mr.Pop(mr.consumers[0])
					So(ok, ShouldBeTrue)
					So(item, ShouldEqual, 42)
				})
			})
		})

		Convey("Given custom map and reduce functions", func() {
			Convey("When creating a MapReduce instance", func() {
				mapper := func(x int) int { return x * 2 }
				reducer := func(a, b int) int { return a + b }
				mr := Setup([]string{"A"}, mapper, reducer)

				Convey("Then it should apply transformations correctly", func() {
					mr.Push(5)
					item, ok := mr.Pop(mr.consumers[0])
					So(ok, ShouldBeTrue)
					So(item, ShouldEqual, 10)
				})
			})
		})
	})
}

func TestPush(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("Given a MapReduce instance", func() {
			mr := Setup[int]([]string{"A", "B"}, nil, nil)
			consumers := mr.consumers

			Convey("When pushing a single item", func() {
				mr.Push(42)

				Convey("Then all consumers should receive the item", func() {
					for _, consumer := range consumers {
						item, ok := mr.Pop(consumer)
						So(ok, ShouldBeTrue)
						So(item, ShouldEqual, 42)
					}
				})
			})

			Convey("When pushing multiple items", func() {
				mr.Push(1)
				mr.Push(2)
				mr.Push(3)

				Convey("Then items should be available in FIFO order", func() {
					first, ok := mr.Pop(consumers[0])
					So(ok, ShouldBeTrue)
					So(first, ShouldEqual, 1)

					second, ok := mr.Pop(consumers[0])
					So(ok, ShouldBeTrue)
					So(second, ShouldEqual, 2)

					third, ok := mr.Pop(consumers[0])
					So(ok, ShouldBeTrue)
					So(third, ShouldEqual, 3)
				})
			})
		})
	})
}

func TestPop(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("Given a MapReduce instance with a reducer", func() {
			reducer := func(a, b int) int { return a + b }
			mr := Setup([]string{"A"}, nil, reducer)
			consumerA := mr.consumers[0]

			Convey("When popping the first item", func() {
				mr.Push(10)
				item, ok := mr.Pop(consumerA)

				Convey("Then it should return without reduction", func() {
					So(ok, ShouldBeTrue)
					So(item, ShouldEqual, 10)
				})
			})

			Convey("When popping subsequent items", func() {
				mr.Push(10)
				mr.Push(20)
				mr.Push(30)

				first, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(first, ShouldEqual, 10)

				second, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(second, ShouldEqual, 30)

				third, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(third, ShouldEqual, 60)
			})
		})

		Convey("Given multiple consumers", func() {
			reducer := func(a, b int) int { return a + b }
			mr := Setup([]string{"A", "B"}, nil, reducer)
			consumers := mr.consumers
			consumerA := consumers[0]
			consumerB := consumers[1]

			Convey("When one item is pushed and both consumers pop", func() {
				mr.Push(5)

				itemA, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(itemA, ShouldEqual, 5)

				itemB, ok := mr.Pop(consumerB)
				So(ok, ShouldBeTrue)
				So(itemB, ShouldEqual, 5)
			})

			Convey("When consumers have independent reduce chains", func() {
				mr.Push(1)
				mr.Push(2)
				mr.Push(3)

				mr.Pop(consumerA)
				mr.Pop(consumerA)

				itemB, ok := mr.Pop(consumerB)
				So(ok, ShouldBeTrue)
				So(itemB, ShouldEqual, 1)

				itemA, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(itemA, ShouldEqual, 6)
			})
		})

		Convey("Given an unknown consumer ID", func() {
			mr := Setup[int]([]string{"A"}, nil, nil)

			Convey("When popping from an unknown consumer", func() {
				item, ok := mr.Pop(NewConsumer[int]("Z", func() {}))

				Convey("Then it should return false", func() {
					So(ok, ShouldBeFalse)
					So(item, ShouldEqual, 0)
				})
			})
		})

		Convey("Given an empty data queue", func() {
			mr := Setup[int]([]string{"A"}, nil, nil)
			consumerA := mr.consumers[0]

			Convey("When popping from an empty queue", func() {
				item, ok := mr.Pop(consumerA)

				Convey("Then it should return false", func() {
					So(ok, ShouldBeFalse)
					So(item, ShouldEqual, 0)
				})
			})
		})

		Convey("Given a mapper function", func() {
			mapper := func(x int) int { return x * 2 }
			reducer := func(a, b int) int { return a + b }
			mr := Setup([]string{"A"}, mapper, reducer)
			consumerA := mr.consumers[0]

			Convey("When pushing and popping", func() {
				mr.Push(5)
				mr.Push(3)

				first, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(first, ShouldEqual, 10)

				second, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(second, ShouldEqual, 16)
			})
		})
	})
}

func TestDrain(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("Given a MapReduce instance with items", func() {
			mr := Setup[int]([]string{"A"}, nil, nil)
			consumerA := mr.consumers[0]
			mr.Push(1)
			mr.Push(2)
			mr.Push(3)

			Convey("When draining all items", func() {
				var collected []int
				fn := func(item int) bool { return true }

				for item := range mr.Drain(consumerA, fn) {
					collected = append(collected, item)
				}

				Convey("Then all items should be collected in order", func() {
					So(len(collected), ShouldEqual, 3)
					So(collected[0], ShouldEqual, 1)
					So(collected[1], ShouldEqual, 2)
					So(collected[2], ShouldEqual, 3)
				})
			})
		})

		Convey("Given a MapReduce instance", func() {
			mr := Setup[int]([]string{"A"}, nil, nil)
			consumerA := mr.consumers[0]
			mr.Push(1)
			mr.Push(2)

			Convey("When the continuation function returns false immediately", func() {
				var collected []int
				fn := func(item int) bool { return false }

				for item := range mr.Drain(consumerA, fn) {
					collected = append(collected, item)
				}

				Convey("Then no items should be collected", func() {
					So(len(collected), ShouldEqual, 0)
				})
			})
		})

		Convey("Given an empty queue", func() {
			mr := Setup[int]([]string{"A"}, nil, nil)
			consumerA := mr.consumers[0]

			Convey("When draining an empty queue", func() {
				var collected []int
				fn := func(item int) bool { return true }

				for item := range mr.Drain(consumerA, fn) {
					collected = append(collected, item)
				}

				Convey("Then no items should be collected", func() {
					So(len(collected), ShouldEqual, 0)
				})
			})
		})

		Convey("Given a reducer", func() {
			reducer := func(a, b int) int { return a + b }
			mr := Setup([]string{"A"}, nil, reducer)
			consumerA := mr.consumers[0]
			mr.Push(10)
			mr.Push(20)
			mr.Push(30)

			Convey("When draining with reduction", func() {
				var collected []int
				fn := func(item int) bool { return true }

				for item := range mr.Drain(consumerA, fn) {
					collected = append(collected, item)
				}

				Convey("Then reduction should accumulate across items", func() {
					So(len(collected), ShouldEqual, 3)
					So(collected[0], ShouldEqual, 10)
					So(collected[1], ShouldEqual, 30)
					So(collected[2], ShouldEqual, 60)
				})
			})
		})
	})
}

func TestStage(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("Given a MapReduce instance with a reducer", func() {
			reducer := func(a, b int) int { return a + b }
			mr := Setup([]string{"A"}, nil, reducer)
			consumerA := mr.consumers[0]

			Convey("When staging the first item", func() {
				mr.Push(10)
				item, ok := mr.Pop(consumerA)

				Convey("Then it should return without reduction", func() {
					So(ok, ShouldBeTrue)
					So(item, ShouldEqual, 10)
				})
			})

			Convey("When staging subsequent items", func() {
				mr.Push(10)
				mr.Push(20)
				mr.Pop(consumerA)

				item, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(item, ShouldEqual, 30)
			})
		})
	})
}

func TestNonComparableType(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("Given a MapReduce instance with slice elements", func() {
			reducer := func(a, b []int) []int {
				result := make([]int, len(a)+len(b))
				copy(result, a)
				copy(result[len(a):], b)
				return result
			}
			mr := Setup[[]int]([]string{"A"}, nil, reducer)
			consumerA := mr.consumers[0]

			Convey("When pushing and popping slice items", func() {
				mr.Push([]int{1})
				mr.Push([]int{2})

				first, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(first, ShouldResemble, []int{1})

				second, ok := mr.Pop(consumerA)
				So(ok, ShouldBeTrue)
				So(second, ShouldResemble, []int{1, 2})
			})
		})
	})
}

func BenchmarkPush(b *testing.B) {
	mr := Setup[int]([]string{"A"}, nil, nil)

	for i := 0; b.Loop(); i++ {
		mr.Push(i)
	}
}

func BenchmarkPop(b *testing.B) {
	mr := Setup[int]([]string{"A"}, nil, nil)
	mr.Push(1)

	for b.Loop() {
		mr.Pop(mr.consumers[0])
	}
}

func BenchmarkDrain(b *testing.B) {
	mr := Setup[int]([]string{"A"}, nil, nil)

	for i := range 1000 {
		mr.Push(i)
	}

	for b.Loop() {
		fn := func(item int) bool { return true }
		for range mr.Drain(mr.consumers[0], fn) {
		}
	}
}
