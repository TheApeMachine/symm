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
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/types"
	"sort"
	"strings"

	"github.com/theapemachine/errnie"
	nmcatalog "github.com/theapemachine/symm/nomagique/runtime/catalog"
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
func Tree(directory string) (map[string]nmcatalog.Schema, error) {
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

	schemas := make(map[string]nmcatalog.Schema)

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
	into map[string]nmcatalog.Schema,
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
) nmcatalog.Schema {
	thing := strings.TrimPrefix(function.Name.Name, constructorStart)

	schema := nmcatalog.Schema{
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
		Inputs: []nmcatalog.Port{{
			Name:        "in",
			Type:        "primitive",
			Description: "The run this primitive reads",
		}},
		Outputs: []nmcatalog.Port{{
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
			schema.Config = append(schema.Config, nmcatalog.Setting{
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

		schema.Inputs = append(schema.Inputs, nmcatalog.Port{
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

type constructorEntry struct {
	op           string
	pkgPath      string
	pkgAlias     string
	funcName     string
	typeArgs     string
	returnsError bool
	params       []paramEntry
}

type paramEntry struct {
	name        string
	rawName     string
	typeStr     string
	isPrimitive bool
	isVariadic  bool
}

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

func sanitizeParamName(name string) string {
	if goKeywords[name] || name == "primitives" || name == "values" {
		return "param_" + name
	}

	return name
}

func packageAlias(pkgPath string) string {
	rel := strings.TrimPrefix(pkgPath, "github.com/theapemachine/symm/")
	rel = strings.ReplaceAll(rel, "/", "_")
	rel = strings.ReplaceAll(rel, "-", "_")
	rel = strings.ReplaceAll(rel, ".", "_")
	return "pkg_" + rel
}

func determineTypeArgs(funcName string) (string, bool) {
	switch funcName {
	case "NewRetained":
		return "[float64]", true
	case "NewMapping", "NewObservation", "NewSympathy", "NewConn":
		return "[*pkg_nomagique_geometry.Coordinate]", true
	case "NewIO":
		return "[any]", true
	default:
		return "", false
	}
}

func isSupportedParam(param *types.Var, isPrimitive bool, isVariadic bool) (string, bool) {
	kind := param.Type()

	if isVariadic {
		if sliced, ok := kind.(*types.Slice); ok {
			kind = sliced.Elem()
		}
	}

	if isPrimitive {
		if sliced, ok := kind.(*types.Slice); ok {
			kind = sliced.Elem()
		}

		if kind.String() == contract {
			return "core.Primitive", true
		}

		return "", false
	}

	basic, ok := kind.Underlying().(*types.Basic)

	if ok {
		switch basic.Kind() {
		case types.String:
			return "string", true
		case types.Int:
			return "int", true
		case types.Int64:
			return "int64", true
		case types.Int32:
			return "int32", true
		case types.Float64:
			return "float64", true
		case types.Float32:
			return "float32", true
		case types.Bool:
			return "bool", true
		}
	}

	return "", false
}

/*
GenerateRegistry inspects all constructors satisfying core.Primitive and emits
Go code declaring catalog.PrimitiveRegistry.
*/
func GenerateRegistry(directory string) ([]byte, error) {
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

	var entries []constructorEntry
	packagesUsed := make(map[string]string)

	for _, loadedPackage := range loaded {
		if _, ok := skipped[loadedPackage.Name]; ok {
			continue
		}

		if len(loadedPackage.Errors) > 0 {
			continue
		}

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
				alias := packageAlias(loadedPackage.PkgPath)

				typeArgs := ""

				if function.Type.TypeParams != nil && len(function.Type.TypeParams.List) > 0 {
					tArgs, ok := determineTypeArgs(function.Name.Name)

					if !ok {
						continue
					}

					typeArgs = tArgs
					packagesUsed["pkg_nomagique_geometry"] = "github.com/theapemachine/symm/nomagique/geometry"
				}

				params := make([]paramEntry, 0)
				supported := true

				for index := range signature.Params().Len() {
					param := signature.Params().At(index)
					isVar := signature.Variadic() && index == signature.Params().Len()-1
					kind := param.Type()

					if isVar {
						if sliced, ok := kind.(*types.Slice); ok {
							kind = sliced.Elem()
						}
					}

					pName := param.Name()

					if pName == "" || pName == "_" {
						pName = lowered(kind.String())
					}

					cleanName := sanitizeParamName(pName)
					isPrim := satisfies(kind, primitive)
					typeStr, ok := isSupportedParam(param, isPrim, isVar)

					if !ok {
						supported = false
						break
					}

					params = append(params, paramEntry{
						name:        cleanName,
						rawName:     pName,
						typeStr:     typeStr,
						isPrimitive: isPrim,
						isVariadic:  isVar,
					})
				}

				if !supported {
					continue
				}

				packagesUsed[alias] = loadedPackage.PkgPath
				entries = append(entries, constructorEntry{
					op:           schema.Op,
					pkgPath:      loadedPackage.PkgPath,
					pkgAlias:     alias,
					funcName:     function.Name.Name,
					typeArgs:     typeArgs,
					returnsError: signature.Results().Len() > 1,
					params:       params,
				})
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].op < entries[j].op
	})

	var buf bytes.Buffer
	buf.WriteString("// Code generated by tools/nomagiquecatalog. DO NOT EDIT.\n\n")
	buf.WriteString("package catalog\n\n")
	buf.WriteString("import (\n")
	buf.WriteString("\t\"fmt\"\n")
	buf.WriteString("\t\"github.com/theapemachine/symm/nomagique/core\"\n")

	var sortedAliases []string

	for a := range packagesUsed {
		sortedAliases = append(sortedAliases, a)
	}

	sort.Strings(sortedAliases)

	for _, a := range sortedAliases {
		buf.WriteString(fmt.Sprintf("\t%s %q\n", a, packagesUsed[a]))
	}

	buf.WriteString(")\n\n")
	buf.WriteString("var PrimitiveRegistry = map[string]Builder{\n")

	for _, entry := range entries {
		buf.WriteString(fmt.Sprintf("\t%q: func(values map[string]any, primitives map[string]core.Primitive) (core.Primitive, error) {\n", entry.op))

		callArgs := make([]string, 0, len(entry.params))

		for _, p := range entry.params {
			if p.isPrimitive {
				if p.isVariadic {
					buf.WriteString(fmt.Sprintf("\t\tvar %s []core.Primitive\n", p.name))
					buf.WriteString(fmt.Sprintf("\t\tif p, ok := primitives[%q]; ok {\n\t\t\t%s = append(%s, p)\n\t\t}\n", p.rawName, p.name, p.name))
					buf.WriteString("\t\tfor i := 0; ; i++ {\n")
					buf.WriteString(fmt.Sprintf("\t\t\tp, ok := primitives[fmt.Sprintf(\"%%s_%%d\", %q, i)]\n", p.rawName))
					buf.WriteString("\t\t\tif !ok {\n\t\t\t\tbreak\n\t\t\t}\n")
					buf.WriteString(fmt.Sprintf("\t\t\t%s = append(%s, p)\n", p.name, p.name))
					buf.WriteString("\t\t}\n")
					buf.WriteString(fmt.Sprintf("\t\tif len(%s) == 0 {\n", p.name))
					buf.WriteString(fmt.Sprintf("\t\t\tfor _, p := range primitives {\n\t\t\t\t%s = append(%s, p)\n\t\t\t}\n\t\t}\n", p.name, p.name))
					callArgs = append(callArgs, p.name+"...")
				}

				if !p.isVariadic {
					buf.WriteString(fmt.Sprintf("\t\tvar %s core.Primitive\n", p.name))
					buf.WriteString(fmt.Sprintf("\t\tif p, ok := primitives[%q]; ok {\n\t\t\t%s = p\n\t\t}\n", p.rawName, p.name))
					callArgs = append(callArgs, p.name)
				}
			}

			if !p.isPrimitive {
				if p.isVariadic {
					switch p.typeStr {
					case "string":
						buf.WriteString(fmt.Sprintf("\t\tvar %s []string\n", p.name))
						buf.WriteString(fmt.Sprintf("\t\tif v, ok := values[%q]; ok {\n", p.rawName))
						buf.WriteString("\t\t\tswitch val := v.(type) {\n")
						buf.WriteString(fmt.Sprintf("\t\t\tcase []string:\n\t\t\t\t%s = val\n", p.name))
						buf.WriteString(fmt.Sprintf("\t\t\tcase []any:\n\t\t\t\tfor _, item := range val {\n\t\t\t\t\tif s, ok := item.(string); ok {\n\t\t\t\t\t\t%s = append(%s, s)\n\t\t\t\t\t}\n\t\t\t\t}\n", p.name, p.name))
						buf.WriteString(fmt.Sprintf("\t\t\tcase string:\n\t\t\t\tif val != \"\" {\n\t\t\t\t\t%s = []string{val}\n\t\t\t\t}\n", p.name))
						buf.WriteString("\t\t\t}\n\t\t}\n")
						callArgs = append(callArgs, p.name+"...")
					default:
						buf.WriteString(fmt.Sprintf("\t\tvar %s []%s\n", p.name, p.typeStr))
						callArgs = append(callArgs, p.name+"...")
					}
				}

				if !p.isVariadic {
					switch p.typeStr {
					case "string":
						buf.WriteString(fmt.Sprintf("\t\tvar %s string\n", p.name))
						buf.WriteString(fmt.Sprintf("\t\tif v, ok := values[%q]; ok {\n\t\t\tif s, ok := v.(string); ok {\n\t\t\t\t%s = s\n\t\t\t}\n\t\t}\n", p.rawName, p.name))
					case "int", "int64", "int32":
						buf.WriteString(fmt.Sprintf("\t\tvar %s %s\n", p.name, p.typeStr))
						buf.WriteString(fmt.Sprintf("\t\tif v, ok := values[%q]; ok {\n\t\t\tswitch n := v.(type) {\n\t\t\tcase float64:\n\t\t\t\t%s = %s(n)\n\t\t\tcase int:\n\t\t\t\t%s = %s(n)\n\t\t\tcase int64:\n\t\t\t\t%s = %s(n)\n\t\t\t}\n\t\t}\n", p.rawName, p.name, p.typeStr, p.name, p.typeStr, p.name, p.typeStr))
					case "float64", "float32":
						buf.WriteString(fmt.Sprintf("\t\tvar %s %s\n", p.name, p.typeStr))
						buf.WriteString(fmt.Sprintf("\t\tif v, ok := values[%q]; ok {\n\t\t\tswitch n := v.(type) {\n\t\t\tcase float64:\n\t\t\t\t%s = %s(n)\n\t\t\tcase int:\n\t\t\t\t%s = %s(n)\n\t\t\tcase int64:\n\t\t\t\t%s = %s(n)\n\t\t\t}\n\t\t}\n", p.rawName, p.name, p.typeStr, p.name, p.typeStr, p.name, p.typeStr))
					case "bool":
						buf.WriteString(fmt.Sprintf("\t\tvar %s bool\n", p.name))
						buf.WriteString(fmt.Sprintf("\t\tif v, ok := values[%q]; ok {\n\t\t\tif b, ok := v.(bool); ok {\n\t\t\t\t%s = b\n\t\t\t}\n\t\t}\n", p.rawName, p.name))
					default:
						buf.WriteString(fmt.Sprintf("\t\tvar %s %s\n", p.name, p.typeStr))
					}

					callArgs = append(callArgs, p.name)
				}
			}
		}

		callStr := fmt.Sprintf("%s.%s%s(%s)", entry.pkgAlias, entry.funcName, entry.typeArgs, strings.Join(callArgs, ", "))

		if entry.returnsError {
			buf.WriteString(fmt.Sprintf("\t\tres, err := %s\n", callStr))
			buf.WriteString("\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
			buf.WriteString("\t\treturn res, nil\n")
		}

		if !entry.returnsError {
			buf.WriteString(fmt.Sprintf("\t\treturn %s, nil\n", callStr))
		}

		buf.WriteString("\t},\n")
	}

	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"catalog: format registry source: "+err.Error(),
			err,
		))
	}

	return formatted, nil
}

