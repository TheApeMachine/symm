package scan_test

import (
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	nmcatalog "github.com/theapemachine/symm/nomagique/runtime/catalog"
	nmscan "github.com/theapemachine/symm/nomagique/runtime/catalog/scan"
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
	scanned map[string]nmcatalog.Schema
	scanErr error
	once    sync.Once
)

func tree(t *testing.T) map[string]nmcatalog.Schema {
	t.Helper()

	once.Do(func() { scanned, scanErr = nmscan.Tree("../../..") })

	So(scanErr, ShouldBeNil)

	return scanned
}

func ports(schema nmcatalog.Schema) []string {
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
			ledger, ok := schemas["learning.TemporalLedger"]

			So(ok, ShouldBeTrue)

			Convey("Then each becomes a port under its own parameter name", func() {
				So(ports(ledger), ShouldResemble, []string{"in", "manifold", "target"})
				So(ledger.Variadic, ShouldBeFalse)
			})

			Convey("Then the constructor's own comment describes it", func() {
				So(ledger.Description, ShouldStartWith, "NewTemporalLedger constructs a temporal ledger")
			})
		})

		Convey("When a constructor takes any number of streams", func() {
			apply, ok := schemas["arithmetic.Apply"]

			So(ok, ShouldBeTrue)

			Convey("Then the port is marked variadic rather than repeated", func() {
				So(ports(apply), ShouldResemble, []string{"in", "operations"})
				So(apply.Variadic, ShouldBeTrue)
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
				So(ports(velocity), ShouldResemble, []string{"in"})
			})
		})

		/*
			An interface that extends the contract is still a stream, and a
			parameter of that type is wired rather than typed in.
		*/
		Convey("When a parameter's type extends the contract", func() {
			pairs, ok := schemas["correlation.Pairs"]

			So(ok, ShouldBeTrue)

			Convey("Then it is a port, not a setting", func() {
				So(ports(pairs), ShouldContain, "estimator")
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
		_, err := nmscan.Tree("../../../../tools")

		Convey("Then the scan says so rather than returning an empty palette", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
