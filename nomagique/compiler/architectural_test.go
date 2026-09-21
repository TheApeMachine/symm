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

		Convey("No generated execution glue exists on disk", func() {
			_, errGen := os.Stat("registry_gen.go")
			So(os.IsNotExist(errGen), ShouldBeTrue)

			_, errAssembler := os.Stat("assembler.go")
			So(os.IsNotExist(errAssembler), ShouldBeTrue)
		})

		Convey("CompiledNode never retains concrete server or any fields", func() {
			nodeType := reflect.TypeOf(compiler.CompiledNode{})
			_, hasServer := nodeType.FieldByName("Server")
			So(hasServer, ShouldBeFalse)

			for i := 0; i < nodeType.NumField(); i++ {
				field := nodeType.Field(i)
				So(field.Type.Kind(), ShouldNotEqual, reflect.Interface)
			}
		})

		Convey("Node AST contains zero any fields", func() {
			astType := reflect.TypeOf(compiler.Node{})
			for i := 0; i < astType.NumField(); i++ {
				field := astType.Field(i)
				So(field.Type.Kind(), ShouldNotEqual, reflect.Interface)
			}
		})

		Convey("Compiler package code contains zero any or interface{} struct fields", func() {
			fset := token.NewFileSet()
			pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
				return strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go")
			}, parser.AllErrors)
			So(err, ShouldBeNil)

			var forbiddenTypes []string
			for _, pkg := range pkgs {
				for filename, file := range pkg.Files {
					ast.Inspect(file, func(n ast.Node) bool {
						if ts, ok := n.(*ast.TypeSpec); ok {
							if st, ok := ts.Type.(*ast.StructType); ok && st.Fields != nil {
								for _, f := range st.Fields.List {
									if ident, ok := f.Type.(*ast.Ident); ok && ident.Name == "any" {
										forbiddenTypes = append(forbiddenTypes, fmt.Sprintf("field %v of type any in %s:%s", f.Names, filename, ts.Name.Name))
									}
									if it, ok := f.Type.(*ast.InterfaceType); ok && len(it.Methods.List) == 0 {
										forbiddenTypes = append(forbiddenTypes, fmt.Sprintf("field %v of type interface{} in %s:%s", f.Names, filename, ts.Name.Name))
									}
								}
							}
						}
						return true
					})
				}
			}
			So(forbiddenTypes, ShouldBeEmpty)
		})

		Convey("No legacy any execution layer identifiers in nomagique", func() {
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
