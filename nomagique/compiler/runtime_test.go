package compiler_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store/tables"
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
			recompJSON := fmt.Appendf(nil, `{
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
			}`, i, i)

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

// retirementWriter models the durable acknowledgement boundary, including retry.
type retirementWriter struct {
	tables.IcebergTableServer
	failure error
	calls   int
}

func (owner *retirementWriter) Flush(ctx context.Context, call runtime.Durable_flush) error {
	owner.calls++
	return owner.failure
}

func TestRuntimeRecompile(t *testing.T) {
	Convey("Given an active graph holding uncommitted durable state", t, func() {
		owner := &retirementWriter{failure: errors.New("archive unavailable")}
		registry := compiler.NewRegistry()
		registry.Register("test.Writer", compiler.Factory{
			InterfaceID: tables.IcebergTable_TypeID,
			New: func(ctx context.Context, config []byte) (capnp.Client, error) {
				return capnp.Client(tables.IcebergTable_ServerToClient(owner)), nil
			},
		})
		active, err := compiler.CompileJSON([]byte(`{"nodes":{"writer":{"id":"writer","type":"test.Writer"}}}`), registry)
		So(err, ShouldBeNil)
		manager := compiler.NewRuntime(active, registry, nil)
		defer func() { manager.Active().Release() }()

		Convey("When retirement fails, the active owner survives and can retry", func() {
			candidate, err := manager.Recompile(context.Background(), compiler.Graph{})
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "archive unavailable")
			So(candidate, ShouldBeNil)
			So(manager.Active(), ShouldEqual, active)
			So(owner.calls, ShouldEqual, 1)

			owner.failure = nil
			candidate, err = manager.Recompile(context.Background(), compiler.Graph{})
			So(err, ShouldBeNil)
			So(manager.Active(), ShouldEqual, candidate)
			So(manager.Active(), ShouldNotEqual, active)
			So(owner.calls, ShouldEqual, 2)
		})
	})
}

func BenchmarkRuntimeRecompile(b *testing.B) {
	graph, err := compiler.ParseGraph([]byte(`{"nodes":{"add":{"id":"add","type":"arithmetic.Add","inputData":{"a":{"value":2},"b":{"value":3}}}}}`))

	if err != nil {
		b.Fatal(err)
	}

	manager := compiler.NewRuntime(nil, nil, nil)
	defer func() { manager.Active().Release() }()
	b.ReportAllocs()

	for b.Loop() {
		if _, err := manager.Recompile(context.Background(), graph); err != nil {
			b.Fatal(err)
		}
	}
}
