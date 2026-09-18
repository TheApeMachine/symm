package store_test

import (
	"iter"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/geometry"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

type testCell struct {
	*core.PrimitiveError
	out string
}

func newTestCell() *testCell {
	return &testCell{PrimitiveError: core.NewPrimitiveError()}
}

func (cell *testCell) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if in == nil {
			if cell.out != "" {
				yield(unsafe.Pointer(&cell.out))
			}

			return
		}

		for arriving := range in {
			input := (*core.Input[any, []string, any])(arriving)

			if input == nil || len(input.Key) == 0 {
				continue
			}

			cell.out = input.Key[0]

			if !yield(unsafe.Pointer(&cell.out)) {
				return
			}
		}
	}
}

func makeStringPayload(items ...string) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for _, item := range items {
			val := any(item)

			if !yield(unsafe.Pointer(&val)) {
				return
			}
		}
	}
}

func makeMapPayload(mappings ...map[string]any) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for _, mapping := range mappings {
			val := any(mapping)

			if !yield(unsafe.Pointer(&val)) {
				return
			}
		}
	}
}

func makeAnyPayload(items ...any) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for _, item := range items {
			val := item

			if !yield(unsafe.Pointer(&val)) {
				return
			}
		}
	}
}

func TestNext(t *testing.T) {
	Convey("Setup", t, func() {
		grid := store.NewGrid[*geometry.Coordinate]()

		firstCell := transport.NewConn[*geometry.Coordinate](newTestCell())
		sequence.Read[core.Connectable[*geometry.Coordinate]](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				firstCell, core.Identify,
			).Next(sequence.NewValue([][]string{{"test1"}})),
		))

		secondCell := transport.NewConn[*geometry.Coordinate](newTestCell())
		sequence.Read[core.Connectable[*geometry.Coordinate]](grid.Next(
			core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
				secondCell, core.Identify,
			).Next(sequence.NewValue([][]string{{"test2", "test2.1"}})),
		))

		Convey("Given a Pipeline with a Grid", func() {
			pipeline := nomagique.NewNumber(grid)

			Convey("When I Push a Query with a Write Action", func() {
				writeCases := []struct {
					name     string
					payload  iter.Seq[unsafe.Pointer]
					expected []string
				}{
					{
						name:     "String payload delivers to matching interest cells",
						payload:  makeStringPayload("test1", "test2"),
						expected: []string{"test1", "test2"},
					},
					{
						name: "Nested map payload traverses interest path segments",
						payload: makeMapPayload(map[string]any{
							"test1": 1,
							"test2": map[string]any{
								"test2.1": 2.1,
							},
						}),
						expected: []string{"test1", "test2"},
					},
					{
						name:     "Single string payload matches only its designated cell",
						payload:  makeStringPayload("test2"),
						expected: []string{"test2"},
					},
					{
						name:     "Unmatched payload keys are skipped without driving cells",
						payload:  makeMapPayload(map[string]any{"unrelated": 42}),
						expected: []string{},
					},
					{
						name: "Non-map intermediate segment stops traversal cleanly",
						payload: makeMapPayload(map[string]any{
							"test2": "not-a-map",
						}),
						expected: []string{},
					},
					{
						name: "Multiple map updates in a stream drive matching cells",
						payload: makeMapPayload(
							map[string]any{"test1": 10},
							map[string]any{"test2": map[string]any{"test2.1": 20}},
						),
						expected: []string{"test1", "test2"},
					},
					{
						name: "Nil map value at path segment stops traversal cleanly",
						payload: makeMapPayload(map[string]any{
							"test2": map[string]any{
								"test2.1": nil,
							},
						}),
						expected: []string{},
					},
					{
						name: "Missing nested key in map stops traversal cleanly",
						payload: makeMapPayload(map[string]any{
							"test2": map[string]any{
								"wrongKey": 42,
							},
						}),
						expected: []string{},
					},
					{
						name:     "Empty string payload does not match interest segments",
						payload:  makeStringPayload(""),
						expected: []string{},
					},
					{
						name:     "Unsupported integer payload is safely ignored without driving cells",
						payload:  makeAnyPayload(42),
						expected: []string{},
					},
					{
						name:     "Unsupported struct payload is safely ignored without driving cells",
						payload:  makeAnyPayload(struct{ Name string }{Name: "test1"}),
						expected: []string{},
					},
					{
						name:     "Empty payload stream yields no output",
						payload:  makeStringPayload(),
						expected: []string{},
					},
				}

				for _, writeCase := range writeCases {
					currentCase := writeCase

					Convey(currentCase.name, func() {
						query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
							nil, core.Write,
						)
						result := pipeline.Next(query.Next(currentCase.payload))
						var received []string

						for res := range result {
							received = append(received, *(*string)(res))
						}

						if len(currentCase.expected) == 0 {
							So(received, ShouldBeEmpty)
							return
						}

						So(received, ShouldResemble, currentCase.expected)
					})
				}
			})

			Convey("When I Push a Query with an Execute Action", func() {
				executeCases := []struct {
					name     string
					payload  iter.Seq[unsafe.Pointer]
					expected []string
				}{
					{
						name:     "Execute routes string payload to matching cells",
						payload:  makeStringPayload("test1", "test2"),
						expected: []string{"test1", "test2"},
					},
					{
						name: "Execute routes nested map payload to matching cells",
						payload: makeMapPayload(map[string]any{
							"test1": 1,
							"test2": map[string]any{
								"test2.1": 2.1,
							},
						}),
						expected: []string{"test1", "test2"},
					},
					{
						name:     "Execute with single string matches only designated cell",
						payload:  makeStringPayload("test1"),
						expected: []string{"test1"},
					},
					{
						name:     "Execute skips unmatched payload keys without driving cells",
						payload:  makeMapPayload(map[string]any{"other": 99}),
						expected: []string{},
					},
					{
						name: "Execute stops traversal when intermediate segment is not a map",
						payload: makeMapPayload(map[string]any{
							"test2": 123,
						}),
						expected: []string{},
					},
					{
						name: "Execute with multiple map updates in stream drives matching cells",
						payload: makeMapPayload(
							map[string]any{"test1": 1},
							map[string]any{"test2": map[string]any{"test2.1": 2}},
						),
						expected: []string{"test1", "test2"},
					},
					{
						name:     "Execute ignores unsupported payload types safely",
						payload:  makeAnyPayload(100, true),
						expected: []string{},
					},
				}

				for _, executeCase := range executeCases {
					currentCase := executeCase

					Convey(currentCase.name, func() {
						query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
							nil, core.Execute,
						)
						var received []string

						for res := range pipeline.Next(query.Next(currentCase.payload)) {
							received = append(received, *(*string)(res))
						}

						if len(currentCase.expected) == 0 {
							So(received, ShouldBeEmpty)
							return
						}

						So(received, ShouldResemble, currentCase.expected)
					})
				}
			})

			Convey("When I Push a Query with a Read Action", func() {
				readCases := []struct {
					name     string
					prime    func()
					expected []string
				}{
					{
						name: "Reads all populated cells in coordinate order",
						prime: func() {
							primeQuery := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
								nil, core.Write,
							)
							for range pipeline.Next(primeQuery.Next(makeStringPayload("test1", "test2"))) {
							}
						},
						expected: []string{"test1", "test2"},
					},
					{
						name:     "Unprimed cells with empty output yield nothing on Read",
						prime:    nil,
						expected: []string{},
					},
					{
						name: "Partially primed cells yield only populated cell outputs in coordinate order",
						prime: func() {
							primeQuery := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
								nil, core.Write,
							)
							for range pipeline.Next(primeQuery.Next(makeStringPayload("test1"))) {
							}
						},
						expected: []string{"test1"},
					},
					{
						name: "Cells retain latest written value on subsequent Read queries",
						prime: func() {
							firstPrime := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
								nil, core.Write,
							)
							for range pipeline.Next(firstPrime.Next(makeStringPayload("test2"))) {
							}

							secondPrime := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
								nil, core.Write,
							)
							for range pipeline.Next(secondPrime.Next(makeStringPayload("test1"))) {
							}
						},
						expected: []string{"test1", "test2"},
					},
				}

				for _, readCase := range readCases {
					currentCase := readCase

					Convey(currentCase.name, func() {
						if currentCase.prime != nil {
							currentCase.prime()
						}

						readQuery := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
							nil, core.Read,
						)
						var received []string

						for res := range pipeline.Next(readQuery.Next(nil)) {
							received = append(received, *(*string)(res))
						}

						if len(currentCase.expected) == 0 {
							So(received, ShouldBeEmpty)
							return
						}

						So(received, ShouldResemble, currentCase.expected)
					})
				}
			})
		})

		Convey("When I Identify members with different coordinates", func() {
			identifyCases := []struct {
				name      string
				coord     *geometry.Coordinate
				expectedX int
				expectedY int
			}{
				{
					name:      "Auto-generates sequential coordinate when coordinate is nil",
					coord:     nil,
					expectedX: 2,
					expectedY: 0,
				},
				{
					name:      "Preserves explicit coordinate identity",
					coord:     geometry.NewCoordinate(5, 10),
					expectedX: 5,
					expectedY: 10,
				},
				{
					name:      "Preserves negative coordinate identity",
					coord:     geometry.NewCoordinate(-4, -8),
					expectedX: -4,
					expectedY: -8,
				},
				{
					name:      "Preserves custom 2D coordinate with zero X and positive Y",
					coord:     geometry.NewCoordinate(0, 15),
					expectedX: 0,
					expectedY: 15,
				},
			}

			for _, identifyCase := range identifyCases {
				currentCase := identifyCase

				Convey(currentCase.name, func() {
					cell := transport.NewConn[*geometry.Coordinate](newTestCell())

					if currentCase.coord != nil {
						cell.Identify(currentCase.coord)
					}

					query := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
						cell, core.Identify,
					)
					addr := sequence.Read[core.Connectable[*geometry.Coordinate]](
						grid.Next(query.Next(sequence.NewValue([][]string{{"custom"}}))),
					)

					So(addr, ShouldNotBeNil)
					So(cell.Identity().X, ShouldEqual, currentCase.expectedX)
					So(cell.Identity().Y, ShouldEqual, currentCase.expectedY)
				})
			}
		})

		Convey("Given a Pipeline with a String-keyed Grid", func() {
			stringGrid := store.NewGrid[string]()

			cellAlpha := transport.NewConn[string](newTestCell())
			sequence.Read[core.Connectable[string]](stringGrid.Next(
				core.NewQuery[string, core.Connectable[string]](
					cellAlpha, core.Identify,
				).Next(sequence.NewValue([][]string{{"alpha"}})),
			))

			cellBeta := transport.NewConn[string](newTestCell())
			sequence.Read[core.Connectable[string]](stringGrid.Next(
				core.NewQuery[string, core.Connectable[string]](
					cellBeta, core.Identify,
				).Next(sequence.NewValue([][]string{{"beta", "sub"}})),
			))

			stringPipeline := nomagique.NewNumber(stringGrid)

			stringCases := []struct {
				name     string
				action   core.Action
				payload  iter.Seq[unsafe.Pointer]
				expected []string
			}{
				{
					name:     "String payload routes to matching string-keyed cell",
					action:   core.Write,
					payload:  makeStringPayload("alpha"),
					expected: []string{"alpha"},
				},
				{
					name:   "Nested map payload routes to matching string-keyed cell",
					action: core.Write,
					payload: makeMapPayload(map[string]any{
						"beta": map[string]any{"sub": 42},
					}),
					expected: []string{"beta"},
				},
				{
					name:     "Unmatched string payload produces empty results",
					action:   core.Write,
					payload:  makeStringPayload("gamma"),
					expected: []string{},
				},
				{
					name:     "Execute action routes payload to string-keyed cells",
					action:   core.Execute,
					payload:  makeStringPayload("alpha", "beta"),
					expected: []string{"alpha", "beta"},
				},
			}

			for _, stringCase := range stringCases {
				currentCase := stringCase

				Convey(currentCase.name, func() {
					query := core.NewQuery[string, core.Connectable[string]](
						nil, currentCase.action,
					)
					var received []string

					for res := range stringPipeline.Next(query.Next(currentCase.payload)) {
						received = append(received, *(*string)(res))
					}

					if len(currentCase.expected) == 0 {
						So(received, ShouldBeEmpty)
						return
					}

					So(received, ShouldResemble, currentCase.expected)
				})
			}

			stringIdentifyCases := []struct {
				name       string
				identity   string
				expectedID string
			}{
				{
					name:       "Auto-generates sequential string identity when identity is empty",
					identity:   "",
					expectedID: "2",
				},
				{
					name:       "Preserves explicit custom string identity",
					identity:   "custom-node",
					expectedID: "custom-node",
				},
			}

			for _, identifyCase := range stringIdentifyCases {
				currentCase := identifyCase

				Convey(currentCase.name, func() {
					cell := transport.NewConn[string](newTestCell())

					if currentCase.identity != "" {
						cell.Identify(currentCase.identity)
					}

					query := core.NewQuery[string, core.Connectable[string]](
						cell, core.Identify,
					)
					addr := sequence.Read[core.Connectable[string]](
						stringGrid.Next(query.Next(sequence.NewValue([][]string{{"item"}}))),
					)

					So(addr, ShouldNotBeNil)
					So(cell.Identity(), ShouldEqual, currentCase.expectedID)
				})
			}
		})

		Convey("When error conditions occur", func() {
			errorCases := []struct {
				name        string
				action      core.Action
				registerNil bool
				expectError bool
			}{
				{
					name:        "Cell with no registered interests records error on Write",
					action:      core.Write,
					registerNil: true,
					expectError: true,
				},
				{
					name:        "Cell with no registered interests records error on Execute",
					action:      core.Execute,
					registerNil: true,
					expectError: true,
				},
				{
					name:        "Empty grid without cells records no error on Write",
					action:      core.Write,
					registerNil: false,
					expectError: false,
				},
			}

			for _, errorCase := range errorCases {
				currentCase := errorCase

				Convey(currentCase.name, func() {
					testGrid := store.NewGrid[*geometry.Coordinate]()

					if currentCase.registerNil {
						bareCell := transport.NewConn[*geometry.Coordinate](newTestCell())
						sequence.Read[core.Connectable[*geometry.Coordinate]](testGrid.Next(
							core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
								bareCell, core.Identify,
							).Next(nil),
						))
					}

					testQuery := core.NewQuery[*geometry.Coordinate, core.Connectable[*geometry.Coordinate]](
						nil, currentCase.action,
					)
					for range testGrid.Next(testQuery.Next(makeStringPayload("test1"))) {
					}

					if currentCase.expectError {
						So(testGrid.Error(), ShouldNotBeNil)
						return
					}

					So(testGrid.Error(), ShouldBeNil)
				})
			}
		})
	})
}

func TestNewGrid(t *testing.T) {
	Convey("Given NewGrid constructor", t, func() {
		constructorCases := []struct {
			name        string
			memberCount int
		}{
			{
				name:        "Constructs empty grid with no initial members",
				memberCount: 0,
			},
			{
				name:        "Constructs grid with single initial member",
				memberCount: 1,
			},
			{
				name:        "Constructs grid with multiple initial members",
				memberCount: 3,
			},
		}

		for _, constructorCase := range constructorCases {
			currentCase := constructorCase

			Convey(currentCase.name, func() {
				var members []core.Connectable[*geometry.Coordinate]

				for index := 0; index < currentCase.memberCount; index++ {
					members = append(members, transport.NewConn[*geometry.Coordinate](newTestCell()))
				}

				constructed := store.NewGrid(members...)
				So(constructed, ShouldNotBeNil)
				So(constructed.Error(), ShouldBeNil)
			})
		}
	})
}
