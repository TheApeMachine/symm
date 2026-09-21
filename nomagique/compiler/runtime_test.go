package compiler_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestRuntimeHotRecompile(t *testing.T) {
	Convey("Given a Runtime with active Program", t, func() {
		reg := compiler.NewRegistry()
		reg.Register("arithmetic.Add", compiler.Factory{
			InterfaceID: arithmetic.Add_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(arithmetic.Add_ServerToClient(arithmetic.NewAdd())), nil
			},
		})
		reg.Register("calculus.Atanh", compiler.Factory{
			InterfaceID: calculus.Atanh_TypeID,
			New: func(ctx context.Context, cfg []byte) (capnp.Client, error) {
				return capnp.Client(calculus.Atanh_ServerToClient(calculus.NewAtanh())), nil
			},
		})

		initialJSON := []byte(`{
			"nodes": {
				"n1": {
					"id": "n1",
					"type": "arithmetic.Add",
					"inputData": {
						"a": {"number": 10},
						"b": {"number": 5}
					}
				}
			}
		}`)

		rt, err := compiler.NewRuntimeFromJSON(initialJSON, reg, nil)
		So(err, ShouldBeNil)
		So(rt, ShouldNotBeNil)

		prog1 := rt.Active()
		So(prog1, ShouldNotBeNil)

		ctx := context.Background()
		err = prog1.Execute(ctx, nil)
		So(err, ShouldBeNil)
		val1, err := prog1.Float64Result("n1", "out")
		So(err, ShouldBeNil)
		So(val1, ShouldEqual, 15.0)

		Convey("When recompiling with a candidate graph", func() {
			newJSON := []byte(`{
				"nodes": {
					"n1": {
						"id": "n1",
						"type": "arithmetic.Add",
						"inputData": {
							"a": {"number": 100},
							"b": {"number": 25}
						}
					}
				}
			}`)

			prog2, err := rt.RecompileJSON(ctx, newJSON)
			So(err, ShouldBeNil)
			So(prog2, ShouldNotBeNil)

			current := rt.Active()
			So(current, ShouldNotBeNil)
			So(current, ShouldEqual, prog2)
			So(current, ShouldNotEqual, prog1)

			err = current.Execute(ctx, nil)
			So(err, ShouldBeNil)
			val2, err := current.Float64Result("n1", "out")
			So(err, ShouldBeNil)
			So(val2, ShouldEqual, 125.0)
		})

		Convey("When candidate compilation fails, active program remains untouched", func() {
			badJSON := []byte(`{
				"nodes": {
					"bad_node": {
						"id": "bad_node",
						"type": "nonexistent.Type"
					}
				}
			}`)

			_, err := rt.RecompileJSON(ctx, badJSON)
			So(err, ShouldNotBeNil)

			// Active program is still prog1
			current := rt.Active()
			So(current, ShouldEqual, prog1)

			err = current.Execute(ctx, nil)
			So(err, ShouldBeNil)
			val1, err := current.Float64Result("n1", "out")
			So(err, ShouldBeNil)
			So(val1, ShouldEqual, 15.0)
		})
	})
}

func TestRuntimeConcurrentQuiescentBarrier(t *testing.T) {
	Convey("Given a Runtime executing concurrently while hot-recompiling", t, func() {
		reg := compiler.DefaultRegistry()

		initialJSON := []byte(`{
			"nodes": {
				"n1": {
					"id": "n1",
					"type": "arithmetic.Add",
					"inputData": {
						"a": {"number": 1},
						"b": {"number": 1}
					}
				}
			}
		}`)

		rt, err := compiler.NewRuntimeFromJSON(initialJSON, reg, nil)
		So(err, ShouldBeNil)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var wg sync.WaitGroup
		// Concurrent executors
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-ctx.Done():
						return
					default:
						_ = rt.Execute(ctx, nil)
					}
				}
			}()
		}

		// Recompile repeatedly across barrier
		for i := 0; i < 20; i++ {
			recompJSON := []byte(fmt.Sprintf(`{
				"nodes": {
					"n1": {
						"id": "n1",
						"type": "arithmetic.Add",
						"inputData": {
							"a": {"number": %d},
							"b": {"number": %d}
						}
					}
				}
			}`, i, i))

			prog, err := rt.RecompileJSON(ctx, recompJSON)
			So(err, ShouldBeNil)
			So(prog != nil, ShouldBeTrue)
		}

		cancel()
		wg.Wait()

		// Final execute on active program succeeds
		err = rt.Execute(context.Background(), nil)
		So(err, ShouldBeNil)
	})
}
