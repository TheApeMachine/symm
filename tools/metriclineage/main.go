/*
Command metriclineage is a static-analysis tool. It has no runtime relationship
to the trading system: it only reads Go source and writes a JSON report.

It answers one question: for every metric a signal kernel produces (a
data.Binding{Name: "..."} passed to data.NewProjector inside a projector.Project(
symbol, "<source>", ...) call), is that (source, metric) pair referenced by
name anywhere a real decision-path consumer would read it — and if not, it is
flagged dead.

Producer identity: the pair of (a) the literal source string passed as the
second argument to a *data.Projector's Project method, and (b) the literal
Name field of each data.Binding passed to data.NewProjector in the same
package. This is the wire identity a Measurement actually carries (see
nomagique/data/projector.go's Project), not the internal MustIntern series-slot
prefix used for temporal-context bookkeeping, which is a different, private
namespace.

Consumer identity, by kind — "bound" and "catalog" mark declared named
inputs; "learned", "kernel", and "generic" record wider reads as context.

  - bound: one exact source/metric row in types.CategorySchemas.

  - catalog: a relation.Selector{Source:, Metric:, Side:} literal. The
    enclosing declaration names the consumer, and production code derives its
    lookup identity from that same selector. It is an explicit coordinate
    contract rather than a variable-name inference.

  - learned: directionalPredictor.observeMeasurement ranges a concrete Metrics
    map and reads every metric's Raw value. It does not statically identify one
    producer, so it never clears "dead".

  - kernel: a runtime.Register callback receives a specific signal kernel's
    whole Measurement. It is scoped to that kernel but not one named metric.

  - generic: a runtime.Register callback receives a generic Measurement and
    therefore identifies neither a kernel nor a named metric.

A metric with zero bound/catalog references is reported dead, regardless of
how many kernel/generic edges it has — those are recorded for context (so the
report never claims a metric touches nothing when its raw bytes do flow
somewhere), but "the struct passed through a bulk subscription" is not the
same claim as "this metric's value is read by anything." A declaration is only
ever a named input; the report does not claim its downstream calculation is
correct.
*/
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

/*
metricID is the join key between producers and consumers: the literal
(source, metric, side) triple as the wire actually carries it.
*/
type metricID struct {
	Source string
	Metric string
	Side   string
}

func (id metricID) String() string {
	if id.Side != "" {
		return id.Source + "/" + id.Metric + ":" + id.Side
	}
	return id.Source + "/" + id.Metric
}

type producer struct {
	ID       metricID
	Package  string
	File     string
	Line     int
	Unit     string
	Resolved bool
}

type consumerEdge struct {
	ID       metricID
	Kind     string // "learned" | "bound" | "catalog" | "kernel" | "generic"
	Consumer string // human label: package/function or subsystem name
	Package  string
	File     string
	Line     int
}

func splitMetricIdentity(source, metric string) metricID {
	identity := metricID{Source: source, Metric: metric}

	if index := strings.IndexByte(metric, ':'); index >= 0 {
		identity.Metric = metric[:index]
		identity.Side = metric[index+1:]
	}

	return identity
}

type report struct {
	Producers  []producerOut   `json:"producers"`
	Consumers  []consumerOut   `json:"consumers"`
	Unresolved []unresolvedOut `json:"unresolved"`
	Summary    summaryOut      `json:"summary"`
}

type producerOut struct {
	ID        string        `json:"id"`
	Source    string        `json:"source"`
	Metric    string        `json:"metric"`
	Side      string        `json:"side,omitempty"`
	Package   string        `json:"package"`
	File      string        `json:"file"`
	Line      int           `json:"line"`
	Unit      string        `json:"unit,omitempty"`
	Consumers []consumerRef `json:"consumers"`
	// Dead means no named (bound/catalog) declared consumer. Wide learned,
	// kernel and generic reads do not clear dead: none of them resolve to this
	// specific metric, so none prove it is actually used.
	Dead bool `json:"dead"`
	// KernelOnly means the metric has no fine-grained (named) consumer and is
	// only reachable because something bulk-subscribes to its whole kernel's
	// output type (e.g. manifold's *hawkes.Measurement subscription) — every
	// metric that kernel produces looks identically "used" through that edge,
	// so this is a weaker signal than a fine edge and worth distinguishing in
	// the UI rather than folding into a blanket "used."
	KernelOnly bool `json:"kernelOnly"`
}

type consumerRef struct {
	Kind     string `json:"kind"`
	Consumer string `json:"consumer"`
	Package  string `json:"package"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

type consumerOut struct {
	Consumer string   `json:"consumer"`
	Kind     string   `json:"kind"`
	Package  string   `json:"package"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Targets  []string `json:"targets"` // metric ids, or "*" for kernel/generic-wide
}

type unresolvedOut struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Reason  string `json:"reason"`
}

type summaryOut struct {
	TotalProducers int `json:"totalProducers"`
	// DeadProducers is the count with no named bound/catalog declaration. Wide
	// learned, kernel and generic reads do not count as references.
	DeadProducers int `json:"deadProducers"`
	// ReferencedProducers is the count with at least one named bound/catalog
	// declaration.
	ReferencedProducers int `json:"referencedProducers"`
	KernelOnlyProducers int `json:"kernelOnlyProducers"`
	BoundConsumers      int `json:"boundConsumerEdges"`
	CatalogConsumers    int `json:"catalogConsumerEdges"`
	LearnedConsumers    int `json:"learnedConsumerEdges"`
	KernelConsumers     int `json:"kernelConsumerEdges"`
	GenericConsumers    int `json:"genericConsumerEdges"`
	Unresolved          int `json:"unresolved"`
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal(err)
	}

	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	root, err = filepath.Abs(root)
	if err != nil {
		fatal(err)
	}

	out := "metric-lineage.json"
	if len(os.Args) > 2 {
		out = os.Args[2]
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedDeps,
		Dir: root,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		fatal(err)
	}

	var hadErrors bool
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			fmt.Fprintln(os.Stderr, e)
			hadErrors = true
		}
	})
	if hadErrors {
		fatal(fmt.Errorf("package loading failed; lineage report was not written"))
	}

	var (
		producers  []producer
		consumers  []consumerEdge
		unresolved []unresolvedOut
	)

	for _, pkg := range pkgs {
		if pkg.PkgPath == "" || strings.Contains(pkg.PkgPath, "/tools/metriclineage") {
			continue
		}

		for _, file := range pkg.Syntax {
			filename := pkg.Fset.Position(file.Package).Filename
			relFile := filename
			if rel, relErr := filepath.Rel(root, filename); relErr == nil {
				relFile = rel
			}

			p, u := scanProducers(pkg, file, relFile)
			producers = append(producers, p...)
			unresolved = append(unresolved, u...)

			consumers = append(consumers, scanFineConsumers(pkg, file, relFile)...)
			consumers = append(consumers, scanCategoryConsumers(pkg, file, relFile)...)
			consumers = append(consumers, scanKernelConsumers(pkg, file, relFile)...)
			consumers = append(consumers, scanLearnedConsumers(pkg, file, relFile)...)
			consumers = append(consumers, scanAdvisorConsumers(pkg, file, relFile)...)
		}
	}


	rep := buildReport(producers, consumers, unresolved)

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		fatal(err)
	}

	if err := os.WriteFile(out, data, 0o644); err != nil {
		fatal(err)
	}

	fmt.Printf(
		"metriclineage: %d producers (%d dead), %d learned reads, %d bound refs, %d catalog refs, %d kernel edges, %d generic edges, %d unresolved -> %s\n",
		rep.Summary.TotalProducers, rep.Summary.DeadProducers,
		rep.Summary.LearnedConsumers,
		rep.Summary.BoundConsumers, rep.Summary.CatalogConsumers,
		rep.Summary.KernelConsumers, rep.Summary.GenericConsumers,
		rep.Summary.Unresolved, out,
	)
}

/*
scanLearnedConsumers verifies the concrete all-metric predictive read. It only
emits an edge when directionalPredictor.observeMeasurement both ranges a
Metrics selector and reads Raw from the ranged metric inside that method.
*/
func scanLearnedConsumers(pkg *packages.Package, file *ast.File, relFile string) []consumerEdge {
	if !strings.HasSuffix(pkg.PkgPath, "/strategy") {
		return nil
	}

	var out []consumerEdge

	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)

		if !ok || function.Name.Name != "observeMeasurement" || function.Body == nil {
			continue
		}

		rangesMetrics := false
		readsRaw := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			switch expression := node.(type) {
			case *ast.RangeStmt:
				selector, valid := expression.X.(*ast.SelectorExpr)

				if valid && selector.Sel.Name == "Metrics" {
					rangesMetrics = true
				}
			case *ast.SelectorExpr:
				if expression.Sel.Name == "Raw" {
					readsRaw = true
				}
			}

			return true
		})

		if !rangesMetrics || !readsRaw {
			continue
		}

		position := pkg.Fset.Position(function.Pos())
		out = append(out, consumerEdge{
			Kind:     "learned",
			Consumer: "strategy.directionalPredictor semantic metric routing",
			Package:  pkg.PkgPath,
			File:     relFile,
			Line:     position.Line,
		})
	}

	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "metriclineage:", err)
	os.Exit(1)
}

/*
scanProducers finds every projector.Project(label, "source", at, from, frame)
call and every data.NewProjector(bindings...) call in the file, and pairs the
Project call's literal source argument with the enclosing package's Binding
Name literals to build the package's producer set.

Both calls are matched structurally: any selector call whose method is named
"Project" for the source string, and any call named "NewProjector" for the
binding names — this tolerates the receiver var name differing per kernel
(projector, ticker.projector, level3.projector, ...) without needing type
information beyond what go/packages already resolved.
*/
func scanProducers(pkg *packages.Package, file *ast.File, relFile string) ([]producer, []unresolvedOut) {
	var (
		sources    []string
		bindings   []producer
		unresolved []unresolvedOut
	)

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fn.Sel.Name == "Project" && len(call.Args) >= 2 {
				if lit := stringLiteral(call.Args[1]); lit != "" {
					sources = append(sources, lit)
				} else {
					pos := pkg.Fset.Position(call.Pos())
					unresolved = append(unresolved, unresolvedOut{
						Package: pkg.PkgPath, File: relFile, Line: pos.Line,
						Reason: "Project() source argument is not a string literal",
					})
				}
			}
		case *ast.Ident:
			// unqualified NewProjector shouldn't happen (always data.NewProjector),
			// but handle it defensively.
			if fn.Name == "NewProjector" {
				bindings = append(bindings, extractBindings(pkg, call, relFile)...)
			}
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "NewProjector" {
			bindings = append(bindings, extractBindings(pkg, call, relFile)...)
		}

		return true
	})

	if len(sources) == 0 || len(bindings) == 0 {
		return nil, unresolved
	}

	// A package emits under one source name in the overwhelming majority of
	// cases (confirmed by direct reading of every signal/* kernel). If a file
	// contains more than one distinct literal source, attribute every binding
	// in that file to all of them rather than silently guessing — downstream
	// dedup on (source, metric) still produces a correct dead/used verdict as
	// long as at least one attribution is right, and readers can see the
	// ambiguity in the package field.
	uniqueSources := dedupeStrings(sources)

	out := make([]producer, 0, len(bindings)*len(uniqueSources))
	for _, b := range bindings {
		for _, src := range uniqueSources {
			out = append(out, producer{
				ID:       metricID{Source: src, Metric: b.ID.Metric, Side: b.ID.Side},
				Package:  pkg.PkgPath,
				File:     relFile,
				Line:     b.Line,
				Unit:     b.Unit,
				Resolved: true,
			})
		}
	}

	return out, unresolved
}

/*
extractBindings walks one NewProjector(...) call's arguments for
data.Binding{Name: "..."} composite literals and splits "metric:side" into
its two parts, matching the convention confirmed against strategy/defaults.go.
*/
func extractBindings(pkg *packages.Package, call *ast.CallExpr, relFile string) []producer {
	var out []producer

	for _, arg := range call.Args {
		lit, ok := arg.(*ast.CompositeLit)
		if !ok {
			continue
		}

		var name, unit string
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch key.Name {
			case "Name":
				name = stringLiteral(kv.Value)
			case "Unit":
				unit = exprString(kv.Value)
			}
		}

		if name == "" {
			continue
		}

		metric, side := name, ""
		if idx := strings.IndexByte(name, ':'); idx >= 0 {
			metric, side = name[:idx], name[idx+1:]
		}

		pos := pkg.Fset.Position(lit.Pos())
		out = append(out, producer{
			ID:   metricID{Metric: metric, Side: side},
			File: relFile,
			Line: pos.Line,
			Unit: unit,
		})
	}

	return out
}

/*
scanFineConsumers finds explicit relation.Selector declarations and attributes
each coordinate to its enclosing package variable or function.
*/
func scanFineConsumers(pkg *packages.Package, file *ast.File, relFile string) []consumerEdge {
	var out []consumerEdge

	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			consumer := pkg.Name + "." + function.Name.Name
			out = append(out, scanDeclaredSelectors(pkg, function, relFile, consumer)...)
			continue
		}

		general, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}

		for _, specification := range general.Specs {
			values, ok := specification.(*ast.ValueSpec)
			if !ok || len(values.Names) == 0 {
				continue
			}

			consumer := pkg.Name + "." + values.Names[0].Name
			out = append(out, scanDeclaredSelectors(pkg, values, relFile, consumer)...)
		}
	}

	return out
}

func scanDeclaredSelectors(
	pkg *packages.Package,
	declaration ast.Node,
	relFile string,
	consumer string,
) []consumerEdge {
	var out []consumerEdge

	ast.Inspect(declaration, func(node ast.Node) bool {
		literal, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}

		if edge, found := declaredSelector(pkg, literal, relFile, consumer); found {
			out = append(out, edge)
		}

		return true
	})

	return out
}

func declaredSelector(
	pkg *packages.Package,
	literal *ast.CompositeLit,
	relFile string,
	consumer string,
) (consumerEdge, bool) {
	selectorType, ok := pkg.TypesInfo.TypeOf(literal.Type).(*types.Named)

	if !ok || selectorType.Obj().Pkg() == nil ||
		selectorType.Obj().Pkg().Path() != "github.com/theapemachine/symm/nomagique/relation" ||
		selectorType.Obj().Name() != "Selector" {
		return consumerEdge{}, false
	}

	var source, metric, side string

	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		name, ok := field.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch name.Name {
		case "Source":
			source = typedString(pkg, field.Value)
		case "Metric":
			metric = typedString(pkg, field.Value)
		case "Side":
			side = typedString(pkg, field.Value)
		}
	}

	if source == "" || metric == "" {
		return consumerEdge{}, false
	}

	position := pkg.Fset.Position(literal.Pos())

	return consumerEdge{
		ID:       metricID{Source: source, Metric: metric, Side: side},
		Kind:     "catalog",
		Consumer: consumer + " (" + pkg.PkgPath + ")",
		Package:  pkg.PkgPath,
		File:     relFile,
		Line:     position.Line,
	}, true
}

/*
scanCategoryConsumers finds the explicit types.CategorySchema table. Category
schemas are named semantic consumers: each row names
one exact producer identity and the market-state interpretation it contributes
to. Source is a typed string constant rather than a literal, so go/types is
used to resolve its value instead of guessing from the identifier spelling.
*/
func scanCategoryConsumers(
	pkg *packages.Package,
	file *ast.File,
	relFile string,
) []consumerEdge {
	if !strings.HasSuffix(pkg.PkgPath, "/types") {
		return nil
	}

	var out []consumerEdge

	ast.Inspect(file, func(node ast.Node) bool {
		declaration, ok := node.(*ast.ValueSpec)

		if !ok || !namesIdentifier(declaration.Names, "CategorySchemas") {
			return true
		}

		for _, value := range declaration.Values {
			table, ok := value.(*ast.CompositeLit)

			if !ok {
				continue
			}

			for _, element := range table.Elts {
				literal, ok := element.(*ast.CompositeLit)

				if !ok {
					continue
				}

				if edge, found := categoryConsumer(pkg, literal, relFile); found {
					out = append(out, edge)
				}
			}
		}

		return false
	})

	return out
}

func categoryConsumer(
	pkg *packages.Package,
	literal *ast.CompositeLit,
	relFile string,
) (consumerEdge, bool) {
	var source, metric, category string

	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)

		if !ok {
			continue
		}

		name, ok := field.Key.(*ast.Ident)

		if !ok {
			continue
		}

		switch name.Name {
		case "Source":
			source = typedString(pkg, field.Value)
		case "Metric":
			metric = stringLiteral(field.Value)
		case "Category":
			category = expressionName(field.Value)
		}
	}

	if source == "" || metric == "" || category == "" {
		return consumerEdge{}, false
	}

	position := pkg.Fset.Position(literal.Pos())

	return consumerEdge{
		ID:       splitMetricIdentity(source, metric),
		Kind:     "bound",
		Consumer: "category:" + category + " (" + pkg.PkgPath + ")",
		Package:  pkg.PkgPath,
		File:     relFile,
		Line:     position.Line,
	}, true
}

func namesIdentifier(names []*ast.Ident, target string) bool {
	for _, name := range names {
		if name.Name == target {
			return true
		}
	}

	return false
}

func typedString(pkg *packages.Package, expression ast.Expr) string {
	if literal := stringLiteral(expression); literal != "" {
		return literal
	}

	if pkg == nil || pkg.TypesInfo == nil {
		return ""
	}

	value := pkg.TypesInfo.Types[expression].Value

	if value == nil || value.Kind() != constant.String {
		return ""
	}

	return constant.StringVal(value)
}

func expressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}

/*
scanKernelConsumers finds runtime.Register(bus, keyFn, callback) call sites
and classifies the callback by its first parameter's type: a specific
kernel's own Measurement type (e.g. *hawkes.Measurement) is a "kernel"-kind
edge (bulk, undifferentiated consumption of that one kernel's whole output);
the generic *data.Measurement[float64] is a "generic"-kind edge. Any other
parameter type means this Register call consumes an already-derived artifact
(e.g. *types.ResonanceArtifact, *graph.GraphUpdate) — those are not metric
consumers at all and are skipped.
*/
func scanKernelConsumers(pkg *packages.Package, file *ast.File, relFile string) []consumerEdge {
	var out []consumerEdge

	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		name := calleeName(call.Fun)
		if name != "Register" && name != "RegisterSink" {
			return true
		}

		// Register/RegisterSink's call shape is (bus, keyFn, callback) or
		// (bus, callback); the callback (what actually processes the
		// measurement and decides what to do with it) is always the LAST
		// argument, and keyFn (when present) only extracts a routing key, so
		// only the last arg is classified — otherwise a keyFn sharing the
		// callback's parameter type produces a spurious duplicate edge.
		if len(call.Args) == 0 {
			return true
		}

		last := call.Args[len(call.Args)-1]

		var paramType string
		switch fn := last.(type) {
		case *ast.FuncLit:
			if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
				return true
			}
			paramType = exprString(fn.Type.Params.List[0].Type)
		case *ast.SelectorExpr:
			// Resolve a registered method value's signature via type information
			// rather than relying on its syntax.
			if sig, ok := pkg.TypesInfo.TypeOf(fn).(*types.Signature); ok && sig.Params().Len() > 0 {
				paramType = sig.Params().At(0).Type().String()
			}
		default:
			return true
		}

		kind, label := classifyParam(paramType)
		if kind == "" {
			return true
		}

		pos := pkg.Fset.Position(call.Pos())
		out = append(out, consumerEdge{
			ID:       metricID{}, // wildcard: applies to every metric of the kernel/generically
			Kind:     kind,
			Consumer: fmt.Sprintf("%s (%s)", label, pkg.PkgPath),
			Package:  pkg.PkgPath,
			File:     relFile,
			Line:     pos.Line,
		})

		return true
	})

	return out
}

/*
classifyParam maps a runtime.Register callback's first-parameter type string
to a consumer kind. "*data.Measurement[float64]" (the unwrapped generic
stream every kernel's output is merged into, per signal/runner.go) is
"generic". A pointer to a type from a named signal subpackage (e.g.
"*hawkes.Measurement") is "kernel", scoped to that one kernel. Anything else
(an already-derived artifact type) returns "" to mean "not a metric consumer."
*/
func classifyParam(paramType string) (kind string, kernel string) {
	if !strings.HasPrefix(paramType, "*") {
		return "", ""
	}
	inner := strings.TrimPrefix(paramType, "*")

	// go/types formats a generic instantiation as
	// "github.com/theapemachine/symm/nomagique/data.Measurement[float64]";
	// go/ast's own selector formatting gives the short form
	// "data.Measurement[float64]". Reduce both to (lastPkgSegment, typeName).
	base := inner
	if idx := strings.IndexByte(inner, '['); idx >= 0 {
		base = inner[:idx]
	}

	dot := strings.LastIndexByte(base, '.')
	if dot < 0 {
		return "", ""
	}
	pkgPath := base[:dot]
	typeName := base[dot+1:]

	pkgName := pkgPath
	if slash := strings.LastIndexByte(pkgPath, '/'); slash >= 0 {
		pkgName = pkgPath[slash+1:]
	}

	if typeName != "Measurement" {
		return "", ""
	}

	if pkgName == "data" {
		return "generic", "generic measurement stream"
	}

	return "kernel", pkgName
}

/*
scanAdvisorConsumers scans advisor package for feature keys and prediction metric references.
*/
func scanAdvisorConsumers(
	pkg *packages.Package,
	file *ast.File,
	relFile string,
) []consumerEdge {
	if !strings.HasSuffix(pkg.PkgPath, "/logic/advisor") {
		return nil
	}

	var out []consumerEdge

	ast.Inspect(file, func(node ast.Node) bool {
		lit, ok := node.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING {
			metricStr := stringLiteral(lit)
			if metricStr != "" && strings.Contains(metricStr, "/") && !strings.Contains(metricStr, "://") {
				source, metric, _ := strings.Cut(metricStr, "/")
				pos := pkg.Fset.Position(lit.Pos())
				out = append(out, consumerEdge{
					ID:       splitMetricIdentity(source, metric),
					Kind:     "bound",
					Consumer: "advisor (" + relFile + ")",
					Package:  pkg.PkgPath,
					File:     relFile,
					Line:     pos.Line,
				})
			}
		}

		return true
	})


	return out
}


func calleeName(fn ast.Expr) string {
	switch f := fn.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func stringLiteral(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value := constant.MakeFromLiteral(lit.Value, lit.Kind, 0)
	if value.Kind() != constant.String {
		return ""
	}
	return constant.StringVal(value)
}

func exprString(expr ast.Expr) string {
	var sb strings.Builder
	ast.Fprint(&sb, nil, expr, ast.NotNilFilter)
	// ast.Fprint is a debug dump, not source text; use a lighter formatter for
	// the common shapes this tool actually needs (selector, star, ident,
	// index expr for generics like Measurement[float64]).
	return formatType(expr)
}

func formatType(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + formatType(e.X)
	case *ast.SelectorExpr:
		return formatType(e.X) + "." + e.Sel.Name
	case *ast.IndexExpr:
		return formatType(e.X) + "[" + formatType(e.Index) + "]"
	case *ast.IndexListExpr:
		parts := make([]string, len(e.Indices))
		for i, idx := range e.Indices {
			parts[i] = formatType(idx)
		}
		return formatType(e.X) + "[" + strings.Join(parts, ",") + "]"
	default:
		return ""
	}
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func buildReport(producers []producer, consumers []consumerEdge, unresolved []unresolvedOut) report {
	// Dedup producers by ID, keeping the first-seen registration site.
	seenProducer := map[metricID]producerOut{}
	var order []metricID

	for _, p := range producers {
		if _, ok := seenProducer[p.ID]; ok {
			continue
		}
		seenProducer[p.ID] = producerOut{
			ID: p.ID.String(), Source: p.ID.Source, Metric: p.ID.Metric, Side: p.ID.Side,
			Package: p.Package, File: p.File, Line: p.Line, Unit: p.Unit,
			Consumers: []consumerRef{},
			Dead:      true,
		}
		order = append(order, p.ID)
	}

	referencedByID := map[metricID][]consumerRef{}
	var learnedWide, kernelWide, genericWide []consumerRef
	consumerOutMap := map[string]*consumerOut{}
	boundEdgeCount, catalogEdgeCount, learnedEdgeCount := 0, 0, 0

	for _, c := range consumers {
		key := c.Consumer + "|" + c.Kind
		co, ok := consumerOutMap[key]
		if !ok {
			co = &consumerOut{Consumer: c.Consumer, Kind: c.Kind, Package: c.Package, File: c.File, Line: c.Line}
			consumerOutMap[key] = co
		}

		switch c.Kind {
		case "learned":
			ref := consumerRef{Kind: c.Kind, Consumer: c.Consumer, Package: c.Package, File: c.File, Line: c.Line}
			learnedWide = append(learnedWide, ref)
			co.Targets = append(co.Targets, "*")
			learnedEdgeCount++
		case "bound", "catalog":
			ref := consumerRef{Kind: c.Kind, Consumer: c.Consumer, Package: c.Package, File: c.File, Line: c.Line}
			referencedByID[c.ID] = append(referencedByID[c.ID], ref)
			co.Targets = append(co.Targets, c.ID.String())
			if c.Kind == "bound" {
				boundEdgeCount++
			} else {
				catalogEdgeCount++
			}
		case "kernel":
			ref := consumerRef{Kind: c.Kind, Consumer: c.Consumer, Package: c.Package, File: c.File, Line: c.Line}
			kernelWide = append(kernelWide, ref)
			co.Targets = append(co.Targets, "*") // wildcard within its kernel, resolved below
		case "generic":
			ref := consumerRef{Kind: c.Kind, Consumer: c.Consumer, Package: c.Package, File: c.File, Line: c.Line}
			genericWide = append(genericWide, ref)
			co.Targets = append(co.Targets, "*")
		}
	}

	// kernelWide entries are wildcards scoped to one kernel package name
	// (the "kernel" field packed into Consumer's label, recovered here from
	// the classifyParam output captured at scan time via the Consumer string
	// prefix before " (").
	kernelScopes := map[string][]consumerRef{}
	for _, ref := range kernelWide {
		scope := ref.Consumer
		if idx := strings.Index(scope, " ("); idx >= 0 {
			scope = scope[:idx]
		}
		kernelScopes[scope] = append(kernelScopes[scope], ref)
	}

	sort.Slice(order, func(i, j int) bool { return order[i].String() < order[j].String() })

	deadCount := 0
	kernelEdgeCount, genericEdgeCount := 0, 0

	outProducers := make([]producerOut, 0, len(order))
	for _, id := range order {
		po := seenProducer[id]

		refs := append([]consumerRef{}, learnedWide...)
		refs = append(refs, referencedByID[id]...)
		if scoped, ok := kernelScopes[id.Source]; ok {
			refs = append(refs, scoped...)
		}
		refs = append(refs, genericWide...)

		hasReference := len(referencedByID[id]) > 0
		hasKernel := len(kernelScopes[id.Source]) > 0

		po.Consumers = refs
		/*
			A bound/catalog reference is a named declared consumer — the honest
			signal that this metric is actually wired somewhere. The learned edge
			is a single scan-level wildcard (observeMeasurement ranges whatever
			Metrics selector its caller passes, then reads .Raw) that never
			resolves to specific producers, so it cannot prove this individual
			metric is used — treating a global boolean as a per-producer read was
			laundering every metric into "referenced". It therefore stays in the
			consumers list as context only, exactly like kernel/generic bulk
			subscriptions, and never flips Dead to false.
		*/
		po.Dead = !hasReference
		po.KernelOnly = !hasReference && hasKernel
		if po.Dead {
			deadCount++
		}

		outProducers = append(outProducers, po)
	}
	kernelEdgeCount = len(kernelWide)
	genericEdgeCount = len(genericWide)

	outConsumers := make([]consumerOut, 0, len(consumerOutMap))
	for _, co := range consumerOutMap {
		co.Targets = dedupeStrings(co.Targets)
		outConsumers = append(outConsumers, *co)
	}
	sort.Slice(outConsumers, func(i, j int) bool {
		if outConsumers[i].Kind != outConsumers[j].Kind {
			return outConsumers[i].Kind < outConsumers[j].Kind
		}
		return outConsumers[i].Consumer < outConsumers[j].Consumer
	})

	sort.Slice(unresolved, func(i, j int) bool {
		if unresolved[i].Package != unresolved[j].Package {
			return unresolved[i].Package < unresolved[j].Package
		}
		return unresolved[i].Line < unresolved[j].Line
	})

	kernelOnlyCount := 0
	referencedCount := 0
	for _, po := range outProducers {
		if po.KernelOnly {
			kernelOnlyCount++
		}
		if !po.Dead && !po.KernelOnly {
			referencedCount++
		}
	}

	return report{
		Producers:  outProducers,
		Consumers:  outConsumers,
		Unresolved: unresolved,
		Summary: summaryOut{
			TotalProducers:      len(outProducers),
			DeadProducers:       deadCount,
			ReferencedProducers: referencedCount,
			KernelOnlyProducers: kernelOnlyCount,
			BoundConsumers:      boundEdgeCount,
			CatalogConsumers:    catalogEdgeCount,
			LearnedConsumers:    learnedEdgeCount,
			KernelConsumers:     kernelEdgeCount,
			GenericConsumers:    genericEdgeCount,
			Unresolved:          len(unresolved),
		},
	}
}
