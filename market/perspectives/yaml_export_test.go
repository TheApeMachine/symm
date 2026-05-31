package perspectives

import (
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuiltinDocumentBuildsStrategies(t *testing.T) {
	Convey("Given the builtin playbook export", t, func() {
		document := BuiltinDocument()

		strategies, err := BuildStrategies(document)

		Convey("It should build every Go playbook", func() {
			So(err, ShouldBeNil)
			So(len(strategies), ShouldEqual, 5)
		})
	})
}

func TestBuiltinDocumentMatchesGoPumpEntry(t *testing.T) {
	Convey("Given builtin pump measurements", t, func() {
		builtin := NewPumpPerspective()
		document := BuiltinDocument()
		strategies, err := BuildStrategies(document)
		So(err, ShouldBeNil)

		var yamlPump *strategy

		for _, candidate := range strategies {
			strategy := candidate.(*strategy)

			if strategy.Name() == PlaybookPump {
				yamlPump = strategy
			}
		}

		So(yamlPump, ShouldNotBeNil)

		measurements := []Measurement{
			measurement(CategoryCoiledCompression, 2),
		}

		Convey("It should match the Go constructor entry verdict", func() {
			builtinAction := builtin.Decide(measurements, nil)
			yamlAction := yamlPump.Decide(measurements, nil)

			So(builtinAction, ShouldNotBeNil)
			So(yamlAction, ShouldNotBeNil)
			So(*yamlAction, ShouldEqual, *builtinAction)
			So(*yamlAction, ShouldEqual, ActionEnter)
		})
	})
}

func TestSaveBuiltinReferenceDocument(t *testing.T) {
	Convey("Given the builtin export", t, func() {
		path := filepath.Join("..", "..", "config", "perspectives.yaml")

		Convey("It should write the reference YAML", func() {
			So(SaveDocumentFile(path, BuiltinDocument()), ShouldBeNil)
		})
	})
}
