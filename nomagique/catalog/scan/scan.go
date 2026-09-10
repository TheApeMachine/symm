/*
Package scan derives nomagique's primitive catalog from its source.

A nomagique primitive is announced by nothing but its contract: there is no
registry to enumerate and no marker to grep for, so the question "is this a
primitive" is exactly the question "does this type satisfy core.Primitive",
and that is a question only the type checker can answer. Matching on the shape
of a declaration instead gets close and then quietly misses — a type that earns
its Next method by embedding another primitive declares nothing at all.

So the packages are loaded and checked, and each constructor is asked what it
returns and whether that satisfies the contract. Its parameters divide the same
way: those satisfying the contract are streams the graph wires, and the rest
are settings the graph carries as values.

Reading the checked types means the catalog cannot describe a primitive that is
not there, and cannot miss one that is.
*/
package scan

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/catalog"
	"golang.org/x/tools/go/packages"
)

const (
	module           = "github.com/theapemachine/symm/nomagique"
	contract         = module + "/core.Primitive"
	constructorStart = "New"
)

/*
skipped are packages that hold no primitives to offer. `tests` is fixtures,
`catalog` is this description of the others.
*/
var skipped = map[string]struct{}{
	"tests":   {},
	"catalog": {},
	"scan":    {},
}

/*
Tree loads a nomagique source tree and returns every primitive it declares,
keyed by Op.

The directory is where loading starts rather than what is read: the type
checker follows the imports itself, which is what makes an embedded contract
visible.
*/
func Tree(directory string) (map[string]catalog.Schema, error) {
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: directory,
	}, "./...")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"catalog: load "+directory,
			err,
		))
	}

	primitive, err := contractOf(loaded)

	if err != nil {
		return nil, err
	}

	schemas := make(map[string]catalog.Schema)

	for _, loadedPackage := range loaded {
		if _, ok := skipped[loadedPackage.Name]; ok {
			continue
		}

		if len(loadedPackage.Errors) > 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"catalog: "+loadedPackage.PkgPath+": "+loadedPackage.Errors[0].Error(),
				nil,
			))
		}

		collect(loadedPackage, primitive, schemas)
	}

	if len(schemas) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"catalog: no primitives found under "+directory,
			nil,
		))
	}

	return schemas, nil
}

/*
contractOf finds core.Primitive among the loaded packages. Everything is
measured against it, so failing to find it is a failure of the scan rather than
a library with no primitives in it.
*/
func contractOf(loaded []*packages.Package) (*types.Interface, error) {
	for _, loadedPackage := range loaded {
		if loadedPackage.Types == nil || loadedPackage.PkgPath != module+"/core" {
			continue
		}

		declared := loadedPackage.Types.Scope().Lookup("Primitive")

		if declared == nil {
			break
		}

		if declaredInterface, ok := declared.Type().Underlying().(*types.Interface); ok {
			return declaredInterface, nil
		}
	}

	return nil, errnie.Error(errnie.Err(
		errnie.Validation,
		"catalog: "+contract+" not found in the loaded tree",
		nil,
	))
}

/*
collect adds every primitive constructor one package declares.
*/
func collect(
	loadedPackage *packages.Package,
	primitive *types.Interface,
	into map[string]catalog.Schema,
) {
	for _, file := range loadedPackage.Syntax {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)

			if !ok || !named(function) {
				continue
			}

			signature := signatureOf(loadedPackage, function)

			if signature == nil || !builds(signature, primitive) {
				continue
			}

			schema := describe(loadedPackage.Name, function, signature, primitive)
			into[schema.Op] = schema
		}
	}
}

// named reports whether a declaration is an exported `New...` free function.
func named(function *ast.FuncDecl) bool {
	return function.Recv == nil &&
		function.Name.IsExported() &&
		strings.HasPrefix(function.Name.Name, constructorStart) &&
		function.Name.Name != constructorStart
}

// signatureOf recovers the checked signature of a declaration.
func signatureOf(loadedPackage *packages.Package, function *ast.FuncDecl) *types.Signature {
	defined, ok := loadedPackage.TypesInfo.Defs[function.Name].(*types.Func)

	if !ok {
		return nil
	}

	signature, _ := defined.Type().(*types.Signature)

	return signature
}

/*
builds reports whether a constructor produces a primitive.

Only the first result is considered, and a constructor that also returns an
error is still a constructor: what matters is that the thing it hands back can
be composed into a pipeline.
*/
func builds(signature *types.Signature, primitive *types.Interface) bool {
	if signature.Results().Len() == 0 {
		return false
	}

	return satisfies(signature.Results().At(0).Type(), primitive)
}

/*
satisfies reports whether a type is a primitive, by value or by pointer. A
constructor usually hands back a pointer, and it is the pointer that carries
the methods.
*/
func satisfies(candidate types.Type, primitive *types.Interface) bool {
	if types.Implements(candidate, primitive) {
		return true
	}

	if _, alreadyPointer := candidate.Underlying().(*types.Pointer); alreadyPointer {
		return false
	}

	return types.Implements(types.NewPointer(candidate), primitive)
}

/*
describe turns one constructor into a schema. Parameters are read in
declaration order so the editor's ports sit in the order the source does.
*/
func describe(
	category string,
	function *ast.FuncDecl,
	signature *types.Signature,
	primitive *types.Interface,
) catalog.Schema {
	thing := strings.TrimPrefix(function.Name.Name, constructorStart)

	schema := catalog.Schema{
		Kind:        "primitive",
		Category:    category,
		Op:          category + "." + thing,
		Name:        thing,
		Label:       spaced(thing),
		Description: doc(function),
		Package:     category,
		Builder:     function.Name.Name,
		/*
			Every primitive reads the run it is handed and answers with one. That
			is the contract itself rather than anything declared per primitive,
			so these two ports are stated rather than discovered.
		*/
		Inputs: []catalog.Port{{
			Name:        "in",
			Type:        "primitive",
			Description: "The run this primitive reads",
		}},
		Outputs: []catalog.Port{{
			Name:        "out",
			Type:        "primitive",
			Description: "The run this primitive produces",
		}},
	}

	parameters := signature.Params()

	for index := range parameters.Len() {
		parameter := parameters.At(index)
		variadic := signature.Variadic() && index == parameters.Len()-1
		kind := parameter.Type()

		// A variadic parameter is checked as a slice of what it accepts.
		if variadic {
			if sliced, ok := kind.(*types.Slice); ok {
				kind = sliced.Elem()
			}
		}

		name := parameter.Name()

		if name == "" || name == "_" {
			name = lowered(kind.String())
		}

		if !satisfies(kind, primitive) {
			schema.Config = append(schema.Config, catalog.Setting{
				Name: name,
				Type: shortened(kind.String()),
			})

			continue
		}

		schema.Variadic = schema.Variadic || variadic

		description := "A stream this primitive reads"

		if variadic {
			description = "Streams this primitive reads, any number of them"
		}

		schema.Inputs = append(schema.Inputs, catalog.Port{
			Name:        name,
			Type:        "primitive",
			Description: description,
		})
	}

	return schema
}

/*
doc is the constructor's own comment, rewrapped onto one line. The source is
the only place a primitive says what it is for, so nothing is invented when it
says nothing.
*/
func doc(function *ast.FuncDecl) string {
	if function.Doc == nil {
		return ""
	}

	lines := make([]string, 0, len(function.Doc.List))

	for _, comment := range function.Doc.List {
		text := strings.TrimPrefix(comment.Text, "//")
		text = strings.TrimPrefix(text, "/*")
		text = strings.TrimSuffix(text, "*/")

		for _, line := range strings.Split(text, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				lines = append(lines, trimmed)
			}
		}
	}

	return strings.Join(lines, " ")
}

/*
spaced turns a constructor's CamelCase into the words a person reads on a node
header, keeping runs of capitals together so ZScore stays one word.
*/
func spaced(name string) string {
	runes := []rune(name)
	words := make([]rune, 0, len(runes)+4)

	for index, letter := range runes {
		upper := letter >= 'A' && letter <= 'Z'
		previousLower := index > 0 && runes[index-1] >= 'a' && runes[index-1] <= 'z'
		nextLower := index+1 < len(runes) && runes[index+1] >= 'a' && runes[index+1] <= 'z'

		if index > 0 && upper && (previousLower || nextLower) {
			words = append(words, ' ')
		}

		words = append(words, letter)
	}

	return string(words)
}

/*
shortened drops the module path from a type so a setting reads as its type
rather than as an import line.
*/
func shortened(name string) string {
	trimmed := strings.ReplaceAll(name, module+"/", "")

	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		return trimmed[index+1:]
	}

	return trimmed
}

// lowered names an unnamed parameter after its own type.
func lowered(name string) string {
	short := shortened(name)

	if index := strings.LastIndex(short, "."); index >= 0 {
		short = short[index+1:]
	}

	short = strings.TrimLeft(short, "*[]")

	if short == "" {
		return "value"
	}

	return strings.ToLower(short[:1]) + short[1:]
}
