package compiler_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/compiler"
)

func TestArchitecturalInvariants(t *testing.T) {
	Convey("Architectural Invariants Verification", t, func() {
		Convey("Compiler contains no Interests method on Builder", func() {
			builderType := reflect.TypeOf(&compiler.Builder{})
			_, hasInterests := builderType.MethodByName("Interests")
			So(hasInterests, ShouldBeFalse)
		})

		Convey("Compiler registry contains no pipeline.* pseudo-primitives", func() {
			reg := compiler.DefaultRegistry()
			for _, pseudo := range []string{"pipeline.Signals", "pipeline.Logic", "pipeline.Execution"} {
				_, err := reg.Resolve(pseudo)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "unknown primitive type")
			}
		})

		Convey("Test G: No any execution layer in nomagique", func() {
			fset := token.NewFileSet()
			var forbiddenIdentifiers []string
			err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
					file, parseErr := parser.ParseFile(fset, path, nil, parser.AllErrors)
					if parseErr != nil {
						return parseErr
					}

					ast.Inspect(file, func(n ast.Node) bool {
						if ident, ok := n.(*ast.Ident); ok {
							if ident.Name == "WriteAny" || ident.Name == "SetDownstreamAny" || ident.Name == "StreamNode" {
								forbiddenIdentifiers = append(forbiddenIdentifiers, fmt.Sprintf("%s in %s", ident.Name, path))
							}
						}
						return true
					})
				}

				return nil
			})

			So(err, ShouldBeNil)
			So(forbiddenIdentifiers, ShouldBeEmpty)
		})
	})
}
