package scan_test

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/catalog"
	"github.com/theapemachine/symm/nomagique/catalog/scan"
)

/*
The scan is exercised against nomagique itself rather than a fixture tree. What
it has to get right is this library's actual idioms — a contract earned by
embedding, an interface that extends the contract, a helper that is not a
primitive at all — and a fixture would only restate the idioms the scan already
assumes.
*/
var (
	// Type-checking the tree costs seconds, and a Convey block re-runs its
	// setup once per leaf. The scan is the fixture, so it is resolved once.
	scanned map[string]catalog.Schema
	scanErr error
	once    sync.Once
)

func tree(t *testing.T) map[string]catalog.Schema {
	t.Helper()

	once.Do(func() { scanned, scanErr = scan.Tree("../..") })

	So(scanErr, ShouldBeNil)

	return scanned
}

func ports(schema catalog.Schema) []string {
	names := make([]string, 0, len(schema.Inputs))

	for _, port := range schema.Inputs {
		names = append(names, port.Name)
	}

	return names
}

func TestTree(t *testing.T) {
	Convey("Given nomagique's source", t, func() {
		schemas := tree(t)

		Convey("Then every primitive reads a run and produces one", func() {
			for op, schema := range schemas {
				So(op, ShouldEqual, schema.Op)
				So(schema.Inputs, ShouldNotBeEmpty)
				So(schema.Inputs[0].Name, ShouldEqual, "in")
				So(schema.Outputs, ShouldHaveLength, 1)
				So(schema.Outputs[0].Name, ShouldEqual, "out")
			}
		})

		/*
			A constructor's primitive parameters are the sub-pipelines a drawn
			graph wires into it, and they are the whole reason the editor needs
			this catalog rather than a list of names.
		*/
		Convey("When a constructor takes streams", func() {
			bound, ok := schemas["equation.Bound"]

			So(ok, ShouldBeTrue)

			Convey("Then each becomes a port under its own parameter name", func() {
				So(ports(bound), ShouldResemble, []string{"in", "value", "lower", "upper"})
				So(bound.Variadic, ShouldBeFalse)
			})

			Convey("Then the constructor's own comment describes it", func() {
				So(bound.Description, ShouldStartWith, "NewBound captures the value")
			})
		})

		Convey("When a constructor takes any number of streams", func() {
			all, ok := schemas["equation.All"]

			So(ok, ShouldBeTrue)

			Convey("Then the port is marked variadic rather than repeated", func() {
				So(ports(all), ShouldResemble, []string{"in", "predicates"})
				So(all.Variadic, ShouldBeTrue)
			})
		})

		/*
			Velocity earns the contract by embedding another primitive and
			declares no Next of its own. Matching on the shape of the source
			misses it; only the checked type finds it.
		*/
		Convey("When a type satisfies the contract by embedding", func() {
			velocity, ok := schemas["temporal.Velocity"]

			So(ok, ShouldBeTrue)

			Convey("Then it is catalogued like any other primitive", func() {
				So(ports(velocity), ShouldResemble, []string{"in", "source", "clock"})
			})
		})

		/*
			An interface that extends the contract is still a stream, and a
			parameter of that type is wired rather than typed in.
		*/
		Convey("When a parameter's type extends the contract", func() {
			profile, ok := schemas["equation.LagProfile"]

			So(ok, ShouldBeTrue)

			Convey("Then it is a port, not a setting", func() {
				So(ports(profile), ShouldContain, "estimator")
				So(profile.Config, ShouldBeEmpty)
			})
		})

		Convey("When a constructor builds something that is not a primitive", func() {
			Convey("Then it is not offered", func() {
				_, sink := schemas["runtime.Sink"]
				_, decoder := schemas["core.Decoder"]

				So(sink, ShouldBeFalse)
				So(decoder, ShouldBeFalse)
			})
		})

		Convey("When a constructor takes a plain value", func() {
			settings := 0

			for _, schema := range schemas {
				settings += len(schema.Config)
			}

			Convey("Then it is carried as a setting rather than a port", func() {
				So(settings, ShouldBeGreaterThan, 0)
			})
		})
	})

	Convey("Given a directory that holds no primitives", t, func() {
		_, err := scan.Tree("../../../tools")

		Convey("Then the scan says so rather than returning an empty palette", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
