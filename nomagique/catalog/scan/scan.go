package scan

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/catalog"
	"golang.org/x/tools/go/packages"
)

const (
	module           = "github.com/theapemachine/symm/nomagique"
	constructorStart = "New"
)

var skipped = map[string]struct{}{
	"tests":    {},
	"catalog":  {},
	"scan":     {},
	"compiler": {},
}

func Tree(directory string) (map[string]catalog.Schema, error) {
	errnie.Debug(fmt.Sprintf("[scan.Tree] scanning directory %s with packages.Load...", directory))
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedFiles,
		Dir: directory,
	}, "./...")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"catalog: load "+directory,
			err,
		))
	}

	errnie.Debug(fmt.Sprintf("[scan.Tree] loaded %d packages, extracting schemas...", len(loaded)))
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

		collect(loadedPackage, schemas)
	}

	if len(schemas) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"catalog: no primitives found under "+directory,
			nil,
		))
	}

	errnie.Debug(fmt.Sprintf("[scan.Tree] successfully collected %d primitive schemas", len(schemas)))
	return schemas, nil
}

type primitiveType struct {
	Name string
	T    string
	U    string
}

func collect(
	loadedPackage *packages.Package,
	into map[string]catalog.Schema,
) {
	// Collect Server structs
	serverTypes := make(map[string]primitiveType)
	for _, file := range loadedPackage.Syntax {
		for _, declaration := range file.Decls {
			genDecl, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok || !typeSpec.Name.IsExported() {
					continue
				}

				indexExpr, ok := typeSpec.Type.(*ast.IndexListExpr)
				if !ok {
					continue
				}

				selExpr, ok := indexExpr.X.(*ast.SelectorExpr)
				if !ok {
					continue
				}

				ident, ok := selExpr.X.(*ast.Ident)
				if !ok || ident.Name != "types" || (selExpr.Sel.Name != "StreamNode" && selExpr.Sel.Name != "Value") {
					continue
				}

				if len(indexExpr.Indices) == 2 {
					tType := loadedPackage.TypesInfo.TypeOf(indexExpr.Indices[0])
					uType := loadedPackage.TypesInfo.TypeOf(indexExpr.Indices[1])

					if tType != nil && uType != nil {
						serverTypes[typeSpec.Name.Name] = primitiveType{
							Name: typeSpec.Name.Name,
							T:    simplifyType(tType.String()),
							U:    simplifyType(uType.String()),
						}
					}
				}
			}
		}
	}

	for _, file := range loadedPackage.Syntax {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)

			if !ok || !named(function) {
				continue
			}

			signature := signatureOf(loadedPackage, function)
			if signature == nil || signature.Results().Len() == 0 {
				continue
			}

			firstResult := signature.Results().At(0).Type()
			namedType, ok := firstResult.(*types.Named)
			if !ok {
				continue
			}

			retTypeName := namedType.Obj().Name()

			if primType, found := serverTypes[retTypeName]; found {
				params := make([]catalog.Param, signature.Params().Len())
				injected := make([]string, 0)
				for i := 0; i < signature.Params().Len(); i++ {
					p := signature.Params().At(i)
					pType := simplifyType(p.Type().String())
					isVar := signature.Variadic() && i == signature.Params().Len()-1
					params[i] = catalog.Param{
						Name:     p.Name(),
						Type:     pType,
						Variadic: isVar,
					}
					if strings.Contains(pType, "context.Context") || strings.Contains(pType, "Config") {
						injected = append(injected, pType)
					}
				}

				stateful := function.Body != nil && len(function.Body.List) > 1

				returnsError := false
				if signature.Results() != nil && signature.Results().Len() == 2 {
					if signature.Results().At(1).Type().String() == "error" {
						returnsError = true
					}
				}

				var dir string
				if len(loadedPackage.GoFiles) > 0 {
					dir = filepath.Dir(loadedPackage.GoFiles[0])
				}

				schema := describe(
					loadedPackage.Name,
					loadedPackage.PkgPath,
					dir,
					function,
					primType.T,
					primType.U,
					signature.Params().Len(),
					signature.TypeParams().Len(),
					params,
					stateful,
					returnsError,
					injected,
				)
				into[schema.Op] = schema
			}
		}
	}
}

func named(function *ast.FuncDecl) bool {
	return function.Recv == nil &&
		function.Name.IsExported() &&
		(strings.HasPrefix(function.Name.Name, constructorStart) || function.Name.Name != constructorStart)
}

func signatureOf(loadedPackage *packages.Package, function *ast.FuncDecl) *types.Signature {
	defined, ok := loadedPackage.TypesInfo.Defs[function.Name].(*types.Func)
	if !ok {
		return nil
	}

	signature, _ := defined.Type().(*types.Signature)
	return signature
}

func simplifyType(t string) string {
	t = strings.ReplaceAll(t, module+"/", "")
	if idx := strings.LastIndex(t, "/"); idx != -1 {
		dotIdx := strings.Index(t[idx:], ".")
		if dotIdx != -1 {
			t = t[:0] + t[idx+1:]
		}
	}
	return t
}

func describe(
	category string,
	pkgPath string,
	pkgDir string,
	function *ast.FuncDecl,
	inType string,
	outType string,
	paramCount int,
	typeParamCount int,
	params []catalog.Param,
	stateful bool,
	returnsError bool,
	injected []string,
) catalog.Schema {
	thing := function.Name.Name
	if after, ok := strings.CutPrefix(thing, constructorStart); ok {
		thing = after
	}

	var dir string
	if len(function.Name.Name) > 0 && len(pkgDir) > 0 {
		dir = pkgDir
	}

	// Find the matching .capnp file
	var capnpPath string
	if files, err := os.ReadDir(dir); err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".capnp") {
				stripped := strings.ReplaceAll(strings.ToLower(f.Name()), "_", "")
				if stripped == strings.ToLower(thing)+".capnp" {
					capnpPath = filepath.Join(dir, f.Name())
					break
				}
			}
		}
	}

	var inputs []catalog.Port
	if capnpPath != "" {
		content, err := os.ReadFile(capnpPath)
		if err == nil {
			re := regexp.MustCompile(`write\s+@\d+\s*\((.*?)\)\s*->`)
			matches := re.FindStringSubmatch(string(content))
			if len(matches) > 1 {
				paramsStr := matches[1]
				parts := strings.Split(paramsStr, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					name := strings.Split(part, ":")[0]
					name = strings.TrimSpace(name)

					inputs = append(inputs, catalog.Port{
						Name:        name,
						Type:        inType,
						Description: "The " + name + " stream this primitive reads",
					})
				}
			}
		}
	}

	if len(inputs) == 0 {
		inputs = []catalog.Port{{
			Name:        "in",
			Type:        inType,
			Description: "The run this primitive reads",
		}}
	}

	for _, p := range params {
		if strings.Contains(p.Type, "context.Context") || strings.Contains(p.Type, "Config") {
			continue
		}

		portType := p.Type
		if p.Variadic && strings.HasPrefix(portType, "[]") {
			portType = portType[2:]
		}

		if strings.HasPrefix(portType, "types.String") || strings.Contains(portType, "Value[any, string]") {
			portType = "string"
		}

		if strings.HasPrefix(portType, "types.Integer") || strings.Contains(portType, "Value[any, int]") {
			portType = "int"
		}

		if strings.HasPrefix(portType, "types.Float") || strings.Contains(portType, "Value[any, float64]") {
			portType = "float64"
		}

		if strings.HasPrefix(portType, "types.Boolean") || strings.Contains(portType, "Value[any, bool]") {
			portType = "bool"
		}

		if strings.HasPrefix(portType, "types.Bytes") || strings.Contains(portType, "Value[any, []byte]") {
			portType = "[]byte"
		}

		if strings.HasPrefix(portType, "types.Map") || strings.Contains(portType, "Value[any, map[string]any]") {
			portType = "map[string]any"
		}

		if strings.HasPrefix(portType, "types.Any") || strings.Contains(portType, "Value[any, any]") {
			portType = "any"
		}

		inputs = append(inputs, catalog.Port{
			Name:        p.Name,
			Type:        portType,
			Description: fmt.Sprintf("Port for %s", p.Name),
		})
	}

	schema := catalog.Schema{
		Kind:              "primitive",
		Category:          category,
		Op:                category + "." + thing,
		Name:              thing,
		Label:             spaced(thing),
		Description:       doc(function),
		Package:           pkgPath,
		Builder:           function.Name.Name,
		ParamCount:        paramCount,
		TypeParamCount:    typeParamCount,
		ConstructorParams: params,
		Stateful:          stateful,
		ReturnsError:      returnsError,
		InjectedDeps:      injected,
		Inputs:            inputs,
		Outputs: []catalog.Port{{
			Name:        "out",
			Type:        outType,
			Description: "The run this primitive produces",
		}},
	}

	return schema
}

func doc(function *ast.FuncDecl) string {
	if function.Doc == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range function.Doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
		b.WriteString(text)
		b.WriteString(" ")
	}
	return strings.TrimSpace(b.String())
}

func spaced(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune(' ')
		}
		result.WriteRune(r)
	}
	return result.String()
}
