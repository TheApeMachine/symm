package compiler

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/theapemachine/errnie"
	"golang.org/x/tools/go/packages"
)

const (
	scanModule           = "github.com/theapemachine/symm/nomagique"
	scanConstructorStart = "New"
)

var scanSkipped = map[string]struct{}{
	"tests":    {},
	"catalog":  {},
	"scan":     {},
	"compiler": {},
	"cmd":      {},
}

/*
Port is one wired connection of a primitive: a stream it reads, or the stream it produces.
*/
type Port struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	RawType     string `json:"rawType,omitempty"`
	Description string `json:"description"`
}

/*
Param is one constructor parameter of a primitive.
*/
type Param struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Variadic bool   `json:"variadic,omitempty"`
}

/*
Schema is the catalog entry for a single primitive behavior.
*/
type Schema struct {
	Kind              string   `json:"kind"`
	Category          string   `json:"category"`
	Op                string   `json:"op"`
	Name              string   `json:"name"`
	Label             string   `json:"label"`
	Description       string   `json:"description"`
	Package           string   `json:"package"`
	Builder           string   `json:"builder"`
	Inputs            []Port   `json:"inputs"`
	Outputs           []Port   `json:"outputs"`
	ParamCount        int      `json:"paramCount,omitempty"`
	TypeParamCount    int      `json:"typeParamCount,omitempty"`
	ConstructorParams []Param  `json:"constructorParams,omitempty"`
	Stateful          bool     `json:"stateful,omitempty"`
	ReturnsError      bool     `json:"returnsError,omitempty"`
	InjectedDeps      []string `json:"injectedDeps,omitempty"`
	CapnpWrite        bool     `json:"capnpWrite,omitempty"`
	HasDownstream     bool     `json:"hasDownstream,omitempty"`
	DownstreamKind    string   `json:"downstreamKind,omitempty"`
	HasServer         bool     `json:"hasServer,omitempty"`
	HasImpl           bool     `json:"hasImpl,omitempty"`
}

/*
ScanTree discovers all Cap'n Proto primitive schemas under directory.
*/
func ScanTree(directory string) (map[string]Schema, error) {
	errnie.Debug(fmt.Sprintf("[compiler.ScanTree] scanning directory %s with packages.Load...", directory))
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedFiles,
		Dir: directory,
	}, "./...")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"compiler: load "+directory,
			err,
		))
	}

	schemas := make(map[string]Schema)

	for _, loadedPackage := range loaded {
		if _, ok := scanSkipped[loadedPackage.Name]; ok {
			continue
		}

		if len(loadedPackage.Errors) > 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"compiler: "+loadedPackage.PkgPath+": "+loadedPackage.Errors[0].Error(),
				nil,
			))
		}

		collectPackageSchemas(loadedPackage, schemas)
	}

	if len(schemas) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"compiler: no primitives found under "+directory,
			nil,
		))
	}

	errnie.Debug(fmt.Sprintf("[compiler.ScanTree] successfully collected %d primitive schemas", len(schemas)))
	return schemas, nil
}

type primitiveTypeInfo struct {
	Name           string
	T              string
	U              string
	HasDownstream  bool
	DownstreamKind string
}

func collectPackageSchemas(
	loadedPackage *packages.Package,
	into map[string]Schema,
) {
	serverTypes := make(map[string]primitiveTypeInfo)
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

				if strings.HasSuffix(typeSpec.Name.Name, "Server") || strings.HasSuffix(typeSpec.Name.Name, "Impl") {
					outType := "Float64"
					inType := "Float64"
					hasDownstream := false
					downstreamKind := ""

					if structType, ok := typeSpec.Type.(*ast.StructType); ok && structType.Fields != nil {
						for _, field := range structType.Fields.List {
							for _, fieldName := range field.Names {
								if fieldName.Name == "Downstream" {
									hasDownstream = true
									switch ft := field.Type.(type) {
									case *ast.SelectorExpr:
										if ft.Sel != nil {
											selName := ft.Sel.Name
											downstreamKind = selName
											if strings.HasPrefix(selName, "Data") {
												outType = "Data"
											}
											if strings.HasPrefix(selName, "Bool") {
												outType = "Bool"
											}
											if strings.HasPrefix(selName, "Int64") {
												outType = "Int64"
											}
											if strings.HasPrefix(selName, "Text") {
												outType = "Text"
											}
											if strings.HasPrefix(selName, "Float64") {
												outType = "Float64"
											}
										}
									case *ast.FuncType:
										outType = "Data"
										downstreamKind = "func"
										if ft.Params != nil && len(ft.Params.List) > 1 {
											firstValParam := ft.Params.List[1]
											if pt, ok := firstValParam.Type.(*ast.Ident); ok {
												switch pt.Name {
												case "float64":
													outType = "Float64"
												case "bool":
													outType = "Bool"
												case "int64", "int", "uint64":
													outType = "Int64"
												case "string":
													outType = "Text"
												}
											}
										}
									}
								}
							}
						}
					}

					serverTypes[typeSpec.Name.Name] = primitiveTypeInfo{
						Name:           typeSpec.Name.Name,
						T:              inType,
						U:              outType,
						HasDownstream:  hasDownstream,
						DownstreamKind: downstreamKind,
					}
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
				if !ok || ident.Name != "types" || selExpr.Sel.Name != "Value" {
					continue
				}

				if len(indexExpr.Indices) == 2 {
					tType := loadedPackage.TypesInfo.TypeOf(indexExpr.Indices[0])
					uType := loadedPackage.TypesInfo.TypeOf(indexExpr.Indices[1])

					if tType != nil && uType != nil {
						serverTypes[typeSpec.Name.Name] = primitiveTypeInfo{
							Name: typeSpec.Name.Name,
							T:    simplifyTypeName(tType.String()),
							U:    simplifyTypeName(uType.String()),
						}
					}
				}
			}
		}
	}

	serverMethods := make(map[string]map[string]bool)
	for _, file := range loadedPackage.Syntax {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv != nil && len(function.Recv.List) > 0 {
				recvType := function.Recv.List[0].Type
				if star, ok := recvType.(*ast.StarExpr); ok {
					recvType = star.X
				}
				if ident, ok := recvType.(*ast.Ident); ok {
					if serverMethods[ident.Name] == nil {
						serverMethods[ident.Name] = make(map[string]bool)
					}
					serverMethods[ident.Name][function.Name.Name] = true
				}
			}
		}
	}

	for _, file := range loadedPackage.Syntax {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || !isNamedConstructor(function) {
				continue
			}

			signature := signatureOfFunc(loadedPackage, function)
			if signature == nil || signature.Results().Len() == 0 {
				continue
			}

			firstResult := signature.Results().At(0).Type()
			if ptr, ok := firstResult.(*types.Pointer); ok {
				firstResult = ptr.Elem()
			}
			namedType, ok := firstResult.(*types.Named)
			if !ok {
				continue
			}

			retTypeName := namedType.Obj().Name()

			if primType, found := serverTypes[retTypeName]; found {
				params := make([]Param, signature.Params().Len())
				injected := make([]string, 0)
				for i := 0; i < signature.Params().Len(); i++ {
					paramVal := signature.Params().At(i)
					pType := simplifyTypeName(paramVal.Type().String())
					isVar := signature.Variadic() && i == signature.Params().Len()-1
					params[i] = Param{
						Name:     paramVal.Name(),
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

				thing := function.Name.Name
				if after, ok := strings.CutPrefix(thing, scanConstructorStart); ok {
					thing = after
				}
				hasServer := false
				if _, ok := serverTypes[thing+"Server"]; ok {
					hasServer = true
				}
				hasImpl := false
				if _, ok := serverTypes[thing+"Impl"]; ok {
					hasImpl = true
				}

				hasWriteMethod := serverMethods[thing+"Server"]["Write"] || serverMethods[thing+"Impl"]["Write"]
				hasDoneMethod := serverMethods[thing+"Server"]["Done"] || serverMethods[thing+"Impl"]["Done"]

				schema := describePrimitive(
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
					primType.HasDownstream,
					primType.DownstreamKind,
					hasServer,
					hasImpl,
					hasWriteMethod,
					hasDoneMethod,
				)
				into[schema.Op] = schema
			}
		}
	}
}

func isNamedConstructor(function *ast.FuncDecl) bool {
	return function.Recv == nil &&
		function.Name.IsExported() &&
		(strings.HasPrefix(function.Name.Name, scanConstructorStart) || function.Name.Name != scanConstructorStart)
}

func signatureOfFunc(loadedPackage *packages.Package, function *ast.FuncDecl) *types.Signature {
	defined, ok := loadedPackage.TypesInfo.Defs[function.Name].(*types.Func)
	if !ok {
		return nil
	}

	signature, _ := defined.Type().(*types.Signature)
	return signature
}

func simplifyTypeName(t string) string {
	t = strings.ReplaceAll(t, scanModule+"/", "")
	if idx := strings.LastIndex(t, "/"); idx != -1 {
		dotIdx := strings.Index(t[idx:], ".")
		if dotIdx != -1 {
			t = t[:0] + t[idx+1:]
		}
	}
	return t
}

func describePrimitive(
	category string,
	pkgPath string,
	pkgDir string,
	function *ast.FuncDecl,
	inType string,
	outType string,
	paramCount int,
	typeParamCount int,
	params []Param,
	stateful bool,
	returnsError bool,
	injected []string,
	hasDownstream bool,
	downstreamKind string,
	hasServer bool,
	hasImpl bool,
	hasWriteMethod bool,
	hasDoneMethod bool,
) Schema {
	thing := function.Name.Name
	if after, ok := strings.CutPrefix(thing, scanConstructorStart); ok {
		thing = after
	}

	var dir string
	if len(function.Name.Name) > 0 && len(pkgDir) > 0 {
		dir = pkgDir
	}

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

		if capnpPath == "" {
			for _, f := range files {
				if strings.HasSuffix(f.Name(), ".capnp") {
					content, err := os.ReadFile(filepath.Join(dir, f.Name()))
					if err == nil {
						cStr := string(content)
						if strings.Contains(cStr, "interface "+thing+" ") ||
							strings.Contains(cStr, "interface "+thing+"{") ||
							strings.Contains(cStr, "interface "+thing+"(") ||
							strings.Contains(cStr, "interface "+thing+"\n") ||
							strings.Contains(cStr, "interface "+thing+"\r") {
							capnpPath = filepath.Join(dir, f.Name())
							break
						}
					}
				}
			}
		}
	}

	var inputs []Port
	capnpWrite := false
	var outputs []Port

	if capnpPath != "" {
		content, err := os.ReadFile(capnpPath)
		if err == nil {
			reMethod := regexp.MustCompile(`(?s)(?:write|poke)\s+@\d+\s*\((.*?)\)\s*->\s*([^;]+);?`)
			matches := reMethod.FindStringSubmatch(string(content))
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
					portType := inType
					if strings.Contains(part, ":") {
						portType = strings.TrimSpace(strings.Split(part, ":")[1])
					}
					rawType := portType
					if strings.HasPrefix(portType, "Wire") || portType == "AnyPointer" || strings.HasPrefix(portType, "List") {
						portType = "Data"
					}
					if strings.HasPrefix(portType, "UInt") || strings.HasPrefix(portType, "Int") {
						portType = "Int64"
					}
					if strings.HasPrefix(portType, "Float") {
						portType = "Float64"
					}
					if portType == "Text" {
						portType = "Text"
					}
					if portType == "Bool" {
						portType = "Bool"
					}
					if portType == "Status" || strings.HasSuffix(portType, ".Status") {
						portType = "Status"
					}
					if isCapabilityType(portType) {
						portType = "Capability"
					}

					inputs = append(inputs, Port{
						Name:        name,
						Type:        portType,
						RawType:     rawType,
						Description: "The " + name + " stream this primitive reads",
					})
				}

				if len(matches) > 2 {
					retStr := strings.TrimSpace(matches[2])
					if strings.Contains(retStr, "List") || strings.Contains(retStr, "Cell") || strings.Contains(retStr, "Data") || strings.Contains(retStr, "AnyPointer") {
						outType = "Data"
					}
				}
			}

			reDone := regexp.MustCompile(`(?s)done\s+@\d+\s*\([^)]*\)\s*->\s*\(([^)]+)\)`)
			matchesDone := reDone.FindStringSubmatch(string(content))
			if len(matchesDone) > 1 {
				returnsStr := matchesDone[1]
				parts := strings.Split(returnsStr, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					name := strings.Split(part, ":")[0]
					name = strings.TrimSpace(name)
					portType := outType
					if strings.Contains(part, ":") {
						portType = strings.TrimSpace(strings.Split(part, ":")[1])
					}
					rawType := portType
					if strings.HasPrefix(portType, "Wire") || portType == "AnyPointer" || strings.HasPrefix(portType, "List") {
						portType = "Data"
					}
					if strings.HasPrefix(portType, "UInt") || strings.HasPrefix(portType, "Int") {
						portType = "Int64"
					}
					if strings.HasPrefix(portType, "Float") {
						portType = "Float64"
					}
					if portType == "Text" {
						portType = "Text"
					}
					if portType == "Bool" {
						portType = "Bool"
					}
					if portType == "Status" || strings.HasSuffix(portType, ".Status") {
						portType = "Status"
					}
					if isCapabilityType(portType) {
						portType = "Capability"
					}

					outputs = append(outputs, Port{
						Name:        name,
						Type:        portType,
						RawType:     rawType,
						Description: "The " + name + " this primitive produces",
					})
				}
			}

			reExtends := regexp.MustCompile(
				`interface\s+` + regexp.QuoteMeta(thing) + `\s+extends\s*\(\s*([A-Za-z0-9_.]+)\s*\)`,
			)

			if extended := reExtends.FindStringSubmatch(string(content)); len(extended) > 1 {
				outputs = append(outputs, Port{
					Name:        "self",
					Type:        "Capability",
					RawType:     extended[1],
					Description: "This primitive as a " + extended[1] + ", to wire into a capability port",
				})
			}

			reWrite := regexp.MustCompile(`(?s)write\s+@\d+\s*\((.*?)\)\s*->\s*stream`)
			if reWrite.MatchString(string(content)) && hasWriteMethod && hasDoneMethod {
				capnpWrite = true
			}
		}
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

		inputs = append(inputs, Port{
			Name:        p.Name,
			Type:        portType,
			Description: fmt.Sprintf("Port for %s", p.Name),
		})
	}

	schema := Schema{
		Kind:              "primitive",
		Category:          category,
		Op:                category + "." + thing,
		Name:              thing,
		Label:             spacedName(thing),
		Description:       docComment(function),
		Package:           pkgPath,
		Builder:           function.Name.Name,
		ParamCount:        paramCount,
		TypeParamCount:    typeParamCount,
		ConstructorParams: params,
		Stateful:          stateful,
		ReturnsError:      returnsError,
		InjectedDeps:      injected,
		CapnpWrite:        capnpWrite,
		HasDownstream:     hasDownstream,
		DownstreamKind:    downstreamKind,
		HasServer:         hasServer,
		HasImpl:           hasImpl,
		Inputs:            inputs,
		Outputs:           outputs,
	}

	return schema
}

func docComment(function *ast.FuncDecl) string {
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

func spacedName(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune(' ')
		}
		result.WriteRune(r)
	}
	return result.String()
}

/*
isCapabilityType reports whether a port carries a live reference to another
node rather than a value. Cap'n Proto interfaces are first-class, so a port
typed as one is wired to a node that the holder calls back as a function.

Every value type the schemas use is named here, so anything left over is an
interface: a new value type must be added rather than silently becoming a
capability.
*/
func isCapabilityType(portType string) bool {
	switch portType {
	case "Data", "Text", "Bool", "Int64", "Float64", "Status", "Void", "":
		return false
	}

	return true
}
