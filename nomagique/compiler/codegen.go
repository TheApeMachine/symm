package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"

	"github.com/theapemachine/symm/nomagique/catalog"
)

/*
Generator parses a JSON graph and compiles it into clean, typed Go code.
Zero maps, zero reflection, zero runtime graph walking on the hot path.
*/
type Generator struct {
	graph   Graph
	catalog map[string]catalog.Schema
}

/*
NewGenerator initializes the compiler code generator for a JSON definition file.
*/
func NewGenerator(jsonPath string) (*Generator, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var graph Graph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, err
	}

	cat, err := loadCatalog()
	if err != nil {
		return nil, err
	}

	return &Generator{
		graph:   graph,
		catalog: cat,
	}, nil
}

/*
Compile generates formatted Go source code and writes it to outputPath.
*/
func (g *Generator) Compile(packageName, funcName, outputPath string) error {
	code, err := g.GenerateSource(packageName, funcName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(outputPath, code, 0644)
}

/*
CompileAll scans a definitions directory and compiles each JSON definition into outputDir.
*/
func CompileAll(defDir, outputDir, packageName string) error {
	entries, err := os.ReadDir(defDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" || entry.Name() == "training.json" {
			continue
		}

		baseName := strings.TrimSuffix(entry.Name(), ".json")
		jsonPath := filepath.Join(defDir, entry.Name())
		gen, err := NewGenerator(jsonPath)
		if err != nil {
			return err
		}

		funcName := ExportName(baseName)
		outPath := filepath.Join(outputDir, strings.ReplaceAll(baseName, ":", "_")+".go")
		if err := gen.Compile(packageName, funcName, outPath); err != nil {
			return err
		}
	}

	return nil
}

/*
GenerateSource produces the Go source byte slice for the graph.
*/
func (g *Generator) GenerateSource(packageName, funcName string) ([]byte, error) {
	inDegree := make(map[string]int)
	adjacency := make(map[string][]string)

	for id := range g.graph.Nodes {
		inDegree[id] = 0
	}

	for id, node := range g.graph.Nodes {
		for _, targets := range node.Connections.Outputs {
			for _, target := range targets {
				adjacency[id] = append(adjacency[id], target.NodeID)
				inDegree[target.NodeID]++
			}
		}
	}

	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue)

	var execOrder []string
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		execOrder = append(execOrder, curr)

		for _, nbr := range adjacency[curr] {
			inDegree[nbr]--
			if inDegree[nbr] == 0 {
				queue = append(queue, nbr)
			}
		}
	}

	if len(execOrder) != len(g.graph.Nodes) {
		return nil, fmt.Errorf("cycle detected in graph %s", g.graph.Name)
	}

	// Determine runnable nodes based on available catalog entries and upstream readiness
	runnable := make(map[string]bool)
	for _, nodeID := range execOrder {
		node := g.graph.Nodes[nodeID]
		if isSource(node, nodeID) {
			runnable[nodeID] = true
			continue
		}
		if isSink(node, nodeID) {
			continue
		}
		if _, ok := g.catalog[node.Type]; !ok {
			if !strings.HasPrefix(node.Type, "pipeline.") {
				continue
			}
		}

		hasInput := false
		for _, targets := range node.Connections.Inputs {
			if len(targets) > 0 && runnable[targets[0].NodeID] {
				hasInput = true
				break
			}
		}
		if hasInput {
			runnable[nodeID] = true
		}
	}

	// Collect required packages and aliases
	pkgImports := make(map[string]string) // alias -> full package path
	pkgImports["nomagique"] = "github.com/theapemachine/symm/nomagique"

	knownPackages := map[string]string{
		"nomagique":   "github.com/theapemachine/symm/nomagique",
		"cognition":   "github.com/theapemachine/symm/nomagique/cognition",
		"cvd":         "github.com/theapemachine/symm/nomagique/statistic/cvd",
		"hawkes":      "github.com/theapemachine/symm/nomagique/statistic/hawkes",
		"execution":   "github.com/theapemachine/symm/nomagique/execution",
		"pipeline":    "github.com/theapemachine/symm/nomagique/compiler",
		"associative": "github.com/theapemachine/symm/nomagique/learning/associative",
		"temporal":    "github.com/theapemachine/symm/nomagique/temporal",
		"transport":   "github.com/theapemachine/symm/nomagique/transport",
	}

	for _, nodeID := range execOrder {
		if !runnable[nodeID] {
			continue
		}
		node := g.graph.Nodes[nodeID]
		if schema, ok := g.catalog[node.Type]; ok {
			alias := schema.Category
			if alias != "" && schema.Package != "" {
				pkgImports[alias] = schema.Package
			}
			for _, in := range schema.Inputs {
				for pkg, path := range knownPackages {
					if strings.Contains(in.Type, pkg+".") {
						pkgImports[pkg] = path
					}
				}
			}
			for _, out := range schema.Outputs {
				for pkg, path := range knownPackages {
					if strings.Contains(out.Type, pkg+".") {
						pkgImports[pkg] = path
					}
				}
			}
		}
	}

	var buf bytes.Buffer
	buf.WriteString("// Code generated by nomagique compiler. DO NOT EDIT.\n\n")
	buf.WriteString("package " + packageName + "\n\n")
	buf.WriteString("import (\n")

	var aliases []string
	for a := range pkgImports {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)

	for _, a := range aliases {
		buf.WriteString(fmt.Sprintf("\t%s %q\n", a, pkgImports[a]))
	}
	buf.WriteString(")\n\n")

	// Constructor function
	if funcName == "" {
		funcName = ExportName(g.graph.Name)
	}

	buf.WriteString(fmt.Sprintf("/*\nNew%s compiles %s into a typed nomagique.Number pipeline.\n*/\n", funcName, g.graph.Name))
	buf.WriteString(fmt.Sprintf("func New%s() nomagique.Number[any] {\n", funcName))

	// 1. Instantiate nodes outside closure
	for _, nodeID := range execOrder {
		if !runnable[nodeID] {
			continue
		}
		node := g.graph.Nodes[nodeID]
		if isSource(node, nodeID) || isSink(node, nodeID) {
			continue
		}

		schema, ok := g.catalog[node.Type]
		if !ok && !strings.HasPrefix(node.Type, "pipeline.") {
			continue
		}

		varInst := "stage_" + sanitizeVar(nodeID)
		var builderCall string
		if strings.HasPrefix(node.Type, "pipeline.") {
			builderCall = "New" + ExportName(strings.TrimPrefix(node.Type, "pipeline.")) + "()"
		} else if node.Type == "data.Extract" {
			builderCall = `data.NewExtract("value")`
		} else {
			builderCall = fmt.Sprintf("%s.%s()", schema.Category, schema.Builder)
		}
		buf.WriteString(fmt.Sprintf("\t%s := %s\n", varInst, builderCall))
	}
	buf.WriteString("\n")

	// 2. Inner execution closure
	buf.WriteString("\treturn nomagique.NewNumber[any](func(in any) any {\n")

	// Track variable names and types producing values
	valVar := make(map[string]string)
	valType := make(map[string]string)
	for _, nodeID := range execOrder {
		node := g.graph.Nodes[nodeID]
		if isSource(node, nodeID) {
			valVar[nodeID] = "in"
			valType[nodeID] = "any"
		}
	}

	for _, nodeID := range execOrder {
		if !runnable[nodeID] {
			continue
		}
		node := g.graph.Nodes[nodeID]
		if isSource(node, nodeID) || isSink(node, nodeID) {
			continue
		}

		schema, ok := g.catalog[node.Type]
		if !ok && !strings.HasPrefix(node.Type, "pipeline.") {
			continue
		}

		var inputArg string
		var upstreamType string
		for _, targets := range node.Connections.Inputs {
			if len(targets) > 0 {
				inputArg = valVar[targets[0].NodeID]
				upstreamType = valType[targets[0].NodeID]
				break
			}
		}

		if inputArg == "" {
			for _, targets := range node.Connections.Inputs {
				if len(targets) > 0 && isSource(g.graph.Nodes[targets[0].NodeID], targets[0].NodeID) {
					inputArg = "in"
					upstreamType = "any"
					break
				}
			}
		}

		if inputArg == "" {
			continue
		}

		stageVar := "stage_" + sanitizeVar(nodeID)
		outVar := "v_" + sanitizeVar(nodeID)
		inType := "any"
		outType := "any"
		if ok {
			if len(schema.Inputs) > 0 {
				inType = schema.Inputs[0].Type
			}
			if len(schema.Outputs) > 0 {
				outType = schema.Outputs[0].Type
			}
		}

		if inputArg == "in" {
			if inType != "any" && inType != "" {
				inVar := "inTyped_" + sanitizeVar(nodeID)
				buf.WriteString(fmt.Sprintf("\t\t%s, ok := in.(%s)\n\t\tif !ok {\n\t\t\treturn nil\n\t\t}\n", inVar, inType))
				inputArg = inVar
			}
		} else if upstreamType == "any" || upstreamType == "" || upstreamType == "interface{}" {
			if inType != "any" && inType != "" {
				inputArg = fmt.Sprintf("%s.(%s)", inputArg, inType)
			}
		}

		buf.WriteString(fmt.Sprintf("\t\t%s := %s(%s)\n", outVar, stageVar, inputArg))
		valVar[nodeID] = outVar
		valType[nodeID] = outType
	}

	var sinkInputVars []string
	for _, nodeID := range execOrder {
		node := g.graph.Nodes[nodeID]
		if isSink(node, nodeID) {
			for _, targets := range node.Connections.Inputs {
				if len(targets) > 0 {
					if v := valVar[targets[0].NodeID]; v != "" {
						sinkInputVars = append(sinkInputVars, v)
						break
					}
				}
			}
		}
	}

	if len(sinkInputVars) > 1 {
		buf.WriteString("\t\treturn []float64{\n")
		for _, v := range sinkInputVars {
			buf.WriteString(fmt.Sprintf("\t\t\tfloat64(%s),\n", v))
		}
		buf.WriteString("\t\t}\n")
	} else if len(sinkInputVars) == 1 {
		buf.WriteString(fmt.Sprintf("\t\treturn %s\n", sinkInputVars[0]))
	} else {
		if len(execOrder) > 0 && valVar[execOrder[len(execOrder)-1]] != "" {
			buf.WriteString(fmt.Sprintf("\t\treturn %s\n", valVar[execOrder[len(execOrder)-1]]))
		} else {
			buf.WriteString("\t\treturn nil\n")
		}
	}

	buf.WriteString("\t})\n")
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return buf.Bytes(), nil // Return raw if formatting has minor issue to inspect
	}

	return formatted, nil
}

func loadCatalog() (map[string]catalog.Schema, error) {
	dir := catalogDir()
	path := filepath.Join(dir, "primitives.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var schemas map[string]catalog.Schema
	if err := json.Unmarshal(data, &schemas); err != nil {
		return nil, err
	}

	return schemas, nil
}

func catalogDir() string {
	candidates := []string{
		"nomagique/catalog",
		"../catalog",
		"../../nomagique/catalog",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c
		}
	}
	_, goFile, _, _ := goruntime.Caller(0)
	return filepath.Join(filepath.Dir(goFile), "..", "catalog")
}

func isSource(node Node, id string) bool {
	return node.Type == "source" || node.Type == "data.Source" || id == "source" || id == "src"
}

func isSink(node Node, id string) bool {
	return strings.HasPrefix(node.Type, "sink") || node.Type == "data.Sink" || strings.HasPrefix(id, "sink")
}

func sanitizeVar(s string) string {
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

func ExportName(s string) string {
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, "-", "_")
	parts := strings.Split(s, "_")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}
