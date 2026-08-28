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

Consumer identity, by kind — only "fine" determines whether a metric is
"used." A metric is used if and only if something references it BY NAME:
  - fine: a literal (source, metric) pair passed to
    logic/advisor.NewMetricBinding / NewControlBinding, or appearing as a
    relation.Selector{Source:, Metric:} literal in strategy/defaults.go's
    defaultMarketCatalog (the causal-influence graph's declared catalog).
    This is the ONLY kind that marks a metric as used.
  - kernel: a runtime.Register call whose callback parameter type names a
    specific kernel's own Measurement type (e.g. *hawkes.Measurement) rather
    than the generic *data.Measurement[float64] — the consumer receives the
    WHOLE struct that kernel produces, undifferentiated. This proves nothing
    about whether any single metric inside it is actually read — it is
    recorded on the producer as context only (kernelOnly: true when it is the
    metric's only lead), never as evidence of use.
  - generic: a runtime.Register call whose callback parameter type is the
    generic *data.Measurement[float64] with no further per-metric filtering
    inside this tool's static reach (e.g. category.Solver, or a UI wire tap in
    cmd/boot.go) — every metric flows through here, so it is even weaker
    evidence than a kernel edge and likewise never marks a metric as used.

A metric with zero fine consumer edges is reported dead, regardless of how
many kernel/generic edges it has — those are recorded for context (so the
report never claims a metric touches nothing when its raw bytes do flow
somewhere), but "the struct passed through a bulk subscription" is not the
same claim as "this metric's value is read by anything."
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
	Kind     string // "fine" | "kernel" | "generic"
	Consumer string // human label: package/function or subsystem name
	Package  string
	File     string
	Line     int
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
	// Dead means no fine-grained AND no kernel-level consumer references this
	// metric by name/kernel at all — nothing in the decision path ever looks
	// at it specifically.
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
	TotalProducers      int `json:"totalProducers"`
	DeadProducers       int `json:"deadProducers"`
	KernelOnlyProducers int `json:"kernelOnlyProducers"`
	FineConsumers       int `json:"fineConsumerEdges"`
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
		fmt.Fprintln(os.Stderr, "metriclineage: continuing despite package load errors")
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
			consumers = append(consumers, scanKernelConsumers(pkg, file, relFile)...)
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
		"metriclineage: %d producers (%d dead), %d fine edges, %d kernel edges, %d generic edges, %d unresolved -> %s\n",
		rep.Summary.TotalProducers, rep.Summary.DeadProducers,
		rep.Summary.FineConsumers, rep.Summary.KernelConsumers, rep.Summary.GenericConsumers,
		rep.Summary.Unresolved, out,
	)
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
scanFineConsumers finds the two literal-string consumption shapes confirmed by
direct reading: logic/advisor.NewMetricBinding/NewControlBinding(source,
metric, prefix) calls, and relation.Selector{Source:, Metric:, Side:} literals
inside strategy/defaults.go's defaultMarketCatalog (the causal-influence
graph's declared candidate variable set — schema.AddMarketVariable only wires
what's listed there, via the variable(source, metric, side) closure).
*/
func scanFineConsumers(pkg *packages.Package, file *ast.File, relFile string) []consumerEdge {
	var out []consumerEdge

	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			name := calleeName(n.Fun)
			if name != "NewMetricBinding" && name != "NewControlBinding" {
				return true
			}
			if len(n.Args) < 2 {
				return true
			}
			source := stringLiteral(n.Args[0])
			metric := stringLiteral(n.Args[1])
			if source == "" || metric == "" {
				return true
			}
			pos := pkg.Fset.Position(n.Pos())
			out = append(out, consumerEdge{
				ID:       metricID{Source: source, Metric: metric},
				Kind:     "fine",
				Consumer: describeAdvisorConsumer(pkg, relFile),
				Package:  pkg.PkgPath,
				File:     relFile,
				Line:     pos.Line,
			})

		case *ast.CompositeLit:
			sel, ok := n.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Selector" {
				return true
			}
			var source, metric, side string
			for _, elt := range n.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				switch key.Name {
				case "Source":
					source = stringLiteral(kv.Value)
				case "Metric":
					metric = stringLiteral(kv.Value)
				case "Side":
					side = stringLiteral(kv.Value)
				}
			}
			if source == "" || metric == "" {
				return true
			}
			pos := pkg.Fset.Position(n.Pos())
			out = append(out, consumerEdge{
				ID:       metricID{Source: source, Metric: metric, Side: side},
				Kind:     "fine",
				Consumer: "graph.Solver (causal-influence catalog)",
				Package:  pkg.PkgPath,
				File:     relFile,
				Line:     pos.Line,
			})
		}
		return true
	})

	return out
}

/*
describeAdvisorConsumer labels which advisor a NewMetricBinding call belongs
to, from the file's own name (liquidity.go/historical.go) rather than a deep
type walk — sufficient since advisor bindings are always declared in the
advisor's own dedicated file per logic/advisor's existing convention.
*/
func describeAdvisorConsumer(pkg *packages.Package, relFile string) string {
	base := filepath.Base(relFile)
	base = strings.TrimSuffix(base, ".go")
	return fmt.Sprintf("advisor:%s (%s)", base, pkg.PkgPath)
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
			// A method value like liquidityAdvisor.Step / historicalAdvisor.Step:
			// resolve its signature via type info rather than syntax.
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

	fineByID := map[metricID][]consumerRef{}
	var kernelWide, genericWide []consumerRef
	consumerOutMap := map[string]*consumerOut{}

	for _, c := range consumers {
		key := c.Consumer + "|" + c.Kind
		co, ok := consumerOutMap[key]
		if !ok {
			co = &consumerOut{Consumer: c.Consumer, Kind: c.Kind, Package: c.Package, File: c.File, Line: c.Line}
			consumerOutMap[key] = co
		}

		switch c.Kind {
		case "fine":
			ref := consumerRef{Kind: c.Kind, Consumer: c.Consumer, Package: c.Package, File: c.File, Line: c.Line}
			fineByID[c.ID] = append(fineByID[c.ID], ref)
			co.Targets = append(co.Targets, c.ID.String())
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
	fineEdgeCount, kernelEdgeCount, genericEdgeCount := 0, 0, 0

	outProducers := make([]producerOut, 0, len(order))
	for _, id := range order {
		po := seenProducer[id]

		refs := append([]consumerRef{}, fineByID[id]...)
		if scoped, ok := kernelScopes[id.Source]; ok {
			refs = append(refs, scoped...)
		}
		refs = append(refs, genericWide...)

		hasFine := len(fineByID[id]) > 0
		hasKernel := len(kernelScopes[id.Source]) > 0

		po.Consumers = refs
		// A metric is used only if something references it BY NAME (a
		// literal (source, metric) lookup — an advisor binding, or a schema
		// catalog entry). A bulk/type-level subscription (kernel edge) proves
		// nothing about whether THIS metric's value is read by anything —
		// the manifold subscribing to *hawkes.Measurement receives the whole
		// struct regardless of which of its 50+ fields anyone downstream
		// actually looks at, so it cannot answer "is this metric used." Such
		// edges stay attached to Consumers as context (kernelOnly), but never
		// flip Dead to false.
		po.Dead = !hasFine
		po.KernelOnly = !hasFine && hasKernel
		if po.Dead {
			deadCount++
		}

		fineEdgeCount += len(fineByID[id])
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
	for _, po := range outProducers {
		if po.KernelOnly {
			kernelOnlyCount++
		}
	}

	return report{
		Producers:  outProducers,
		Consumers:  outConsumers,
		Unresolved: unresolved,
		Summary: summaryOut{
			TotalProducers:      len(outProducers),
			DeadProducers:       deadCount,
			KernelOnlyProducers: kernelOnlyCount,
			FineConsumers:       fineEdgeCount,
			KernelConsumers:     kernelEdgeCount,
			GenericConsumers:    genericEdgeCount,
			Unresolved:          len(unresolved),
		},
	}
}
