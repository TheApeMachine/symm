package compiler

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/std/capnp/schema"
	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/network/http"
	"github.com/theapemachine/symm/nomagique/runtime"
)

var (
	defaultDefRepoMu sync.RWMutex
	defaultDefRepo   DefinitionRepository
)

/*
SetDefaultDefinitionRepository configures the repository used by the workbench runner.
*/
func SetDefaultDefinitionRepository(repo DefinitionRepository) {
	defaultDefRepoMu.Lock()
	defaultDefRepo = repo
	defaultDefRepoMu.Unlock()
}

func getDefaultDefinitionRepository() DefinitionRepository {
	defaultDefRepoMu.RLock()
	defer defaultDefRepoMu.RUnlock()

	return defaultDefRepo
}

/*
Diagnostic identifies an editor coordinate and reason for compilation failure.
*/
type Diagnostic struct {
	NodeID   string `json:"nodeId,omitempty"`
	NodeType string `json:"nodeType,omitempty"`
	PortName string `json:"portName,omitempty"`
	EdgeFrom string `json:"edgeFrom,omitempty"`
	EdgeTo   string `json:"edgeTo,omitempty"`
	Kind     string `json:"kind"`
	Message  string `json:"message"`
}

type CompileResponse struct {
	OK          bool         `json:"ok"`
	NodeCount   int          `json:"nodeCount,omitempty"`
	RouteCount  int          `json:"routeCount,omitempty"`
	Error       string       `json:"error,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type RunResponse struct {
	OK          bool                          `json:"ok"`
	Results     map[string]map[string]any     `json:"results,omitempty"`
	Statuses    map[string]string             `json:"statuses,omitempty"`
	Logs        map[string][]runtime.LogEntry `json:"logs,omitempty"`
	Error       string                        `json:"error,omitempty"`
	Diagnostics []Diagnostic                  `json:"diagnostics,omitempty"`
}

type WorkbenchRunnerImpl struct {
	reg *Registry
}

func NewWorkbenchRunner() *WorkbenchRunnerImpl {
	return &WorkbenchRunnerImpl{
		reg: DefaultRegistry(),
	}
}

func (w *WorkbenchRunnerImpl) IsPrimitiveUsable(op string) bool {
	return w.reg.Has(op) || op == "data.Source" || op == "data.Sink"
}

func (w *WorkbenchRunnerImpl) Primitives() ([]byte, error) {
	return GeneratedPrimitivesJSON, nil
}

func (w *WorkbenchRunnerImpl) ListDefinitions() ([]string, error) {
	return DefaultRepository().List()
}

func (w *WorkbenchRunnerImpl) GetDefinition(id string) ([]byte, error) {
	return DefaultRepository().Get(id)
}

func (w *WorkbenchRunnerImpl) SaveDefinition(id string, rawJSON []byte) error {
	return DefaultRepository().Save(id, rawJSON)
}

func parseGraphPayload(rawJSON []byte, repo DefinitionRepository) (Graph, string, error) {
	var payload struct {
		ID    string          `json:"id"`
		Graph *Graph          `json:"graph"`
		Nodes map[string]Node `json:"nodes"`
	}

	if err := sonic.Unmarshal(rawJSON, &payload); err == nil {
		if payload.Graph != nil && len(payload.Graph.Nodes) > 0 {
			return *payload.Graph, payload.ID, nil
		}
		if len(payload.Nodes) > 0 {
			return Graph{Nodes: payload.Nodes}, payload.ID, nil
		}
		if payload.ID != "" && repo != nil {
			g, err := repo.Load(payload.ID)
			return g, payload.ID, err
		}
	}

	// Try unmarshaling directly as Graph
	var direct Graph
	if err := sonic.Unmarshal(rawJSON, &direct); err == nil && len(direct.Nodes) > 0 {
		return direct, "", nil
	}

	return Graph{}, "", fmt.Errorf("workbench: empty or invalid graph payload")
}

func (w *WorkbenchRunnerImpl) Compile(rawJSON []byte) (any, error) {
	repo := getDefaultDefinitionRepository()
	graph, _, err := parseGraphPayload(rawJSON, repo)
	if err != nil {
		return CompileResponse{
			OK:          false,
			Error:       err.Error(),
			Diagnostics: []Diagnostic{{Kind: "parse_error", Message: err.Error()}},
		}, err
	}

	prog, err := CompileWithPrevious(graph, w.reg, nil, repo)
	if err != nil {
		diags := extractDiagnostics(err)
		return CompileResponse{
			OK:          false,
			Error:       err.Error(),
			Diagnostics: diags,
		}, err
	}
	defer prog.Release()

	return CompileResponse{
		OK:         true,
		NodeCount:  len(prog.Nodes),
		RouteCount: len(prog.Routes),
	}, nil
}

func (w *WorkbenchRunnerImpl) Run(ctx context.Context, rawJSON []byte) (any, error) {
	repo := getDefaultDefinitionRepository()
	graph, _, err := parseGraphPayload(rawJSON, repo)
	if err != nil {
		return RunResponse{
			OK:          false,
			Error:       err.Error(),
			Diagnostics: []Diagnostic{{Kind: "parse_error", Message: err.Error()}},
		}, err
	}

	prog, err := CompileWithPrevious(graph, w.reg, nil, repo)
	if err != nil {
		diags := extractDiagnostics(err)
		return RunResponse{
			OK:          false,
			Error:       err.Error(),
			Diagnostics: diags,
		}, err
	}
	defer prog.Release()

	collectedLogs := make(map[string][]runtime.LogEntry)
	var logMu sync.Mutex

	runtime.SetGlobalLogHook(func(sys *runtime.System, entry runtime.LogEntry) {
		logMu.Lock()
		defer logMu.Unlock()
		collectedLogs[sys.Name()] = append(collectedLogs[sys.Name()], entry)

		for nodeID, node := range graph.Nodes {
			if nodeMatchesSystem(node.Type, sys.Name()) {
				collectedLogs[nodeID] = append(collectedLogs[nodeID], entry)
			}
		}
	})
	defer runtime.SetGlobalLogHook(nil)

	if err := prog.Execute(ctx, nil); err != nil {
		statuses := make(map[string]string)
		for _, node := range prog.Nodes {
			statuses[node.ID] = "error"
		}

		return RunResponse{
			OK:       false,
			Error:    err.Error(),
			Statuses: statuses,
			Logs:     collectedLogs,
		}, err
	}

	results := make(map[string]map[string]any)
	statuses := make(map[string]string)

	for _, node := range prog.Nodes {
		statuses[node.ID] = "ready"
		st, ok := prog.Result(node.ID)
		if !ok {
			continue
		}

		nodeRes := make(map[string]any)
		for outName, field := range node.Outputs {
			val := extractFieldValue(st, field)
			nodeRes[outName] = val
		}

		if len(nodeRes) > 0 {
			results[node.ID] = nodeRes
		}
	}

	return RunResponse{
		OK:       true,
		Results:  results,
		Statuses: statuses,
		Logs:     collectedLogs,
	}, nil
}

func extractFieldValue(st capnp.Struct, field CompiledField) any {
	switch field.Which {
	case schema.Type_Which_float64:
		return math.Float64frombits(st.Uint64(capnp.DataOffset(field.Offset * 8)))
	case schema.Type_Which_float32:
		return math.Float32frombits(st.Uint32(capnp.DataOffset(field.Offset * 4)))
	case schema.Type_Which_int64:
		return int64(st.Uint64(capnp.DataOffset(field.Offset * 8)))
	case schema.Type_Which_uint64:
		return st.Uint64(capnp.DataOffset(field.Offset * 8))
	case schema.Type_Which_int32:
		return int32(st.Uint32(capnp.DataOffset(field.Offset * 4)))
	case schema.Type_Which_uint32:
		return st.Uint32(capnp.DataOffset(field.Offset * 4))
	case schema.Type_Which_int16:
		return int16(st.Uint16(capnp.DataOffset(field.Offset * 2)))
	case schema.Type_Which_uint16:
		return st.Uint16(capnp.DataOffset(field.Offset * 2))
	case schema.Type_Which_int8:
		return int8(st.Uint8(capnp.DataOffset(field.Offset)))
	case schema.Type_Which_uint8:
		return st.Uint8(capnp.DataOffset(field.Offset))
	case schema.Type_Which_bool:
		return st.Bit(capnp.BitOffset(field.Offset))
	case schema.Type_Which_text:
		p, err := st.Ptr(uint16(field.Offset))
		if err != nil {
			return ""
		}
		return p.Text()
	case schema.Type_Which_data:
		p, err := st.Ptr(uint16(field.Offset))
		if err != nil || !p.IsValid() {
			return nil
		}
		return p.Data()
	case schema.Type_Which_structType:
		p, err := st.Ptr(uint16(field.Offset))
		if err != nil || !p.IsValid() {
			return nil
		}
		return "<struct>"
	case schema.Type_Which_list:
		p, err := st.Ptr(uint16(field.Offset))
		if err != nil || !p.IsValid() {
			return nil
		}
		return fmt.Sprintf("<list:%d>", p.List().Len())
	case schema.Type_Which_enum:
		return st.Uint16(capnp.DataOffset(field.Offset * 2))
	case schema.Type_Which_void:
		return "void"
	default:
		return fmt.Sprintf("<%v>", field.Which)
	}
}

var (
	typeMismatchRe = regexp.MustCompile(`type mismatch on edge (\S+)\.(\S+) \(([^)]+)\) -> (\S+)\.(\S+) \(([^)]+)\)`)
	noInputPortRe  = regexp.MustCompile(`node "([^"]+)" \(([^)]+)\) has no input port "([^"]+)"`)
	noOutputPortRe = regexp.MustCompile(`node "([^"]+)" \(([^)]+)\) has no output port "([^"]+)"`)
	unknownPrimRe  = regexp.MustCompile(`unknown primitive type "([^"]+)"(?: for node "([^"]+)")?`)
	missingDefRe   = regexp.MustCompile(`failed to load definition "([^"]+)"(?: for node "([^"]+)")?`)
)

func extractDiagnostics(err error) []Diagnostic {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	if matches := typeMismatchRe.FindStringSubmatch(errStr); len(matches) > 6 {
		return []Diagnostic{{
			NodeID:   matches[1],
			PortName: matches[2],
			EdgeFrom: matches[1] + "." + matches[2],
			EdgeTo:   matches[4] + "." + matches[5],
			Kind:     "incompatible_edge",
			Message:  fmt.Sprintf("Cannot connect %s (%s) to %s (%s)", matches[2], matches[3], matches[5], matches[6]),
		}}
	}

	if matches := noInputPortRe.FindStringSubmatch(errStr); len(matches) > 3 {
		return []Diagnostic{{
			NodeID:   matches[1],
			NodeType: matches[2],
			PortName: matches[3],
			Kind:     "missing_input",
			Message:  fmt.Sprintf("Node %s (%s) does not have input port %s", matches[1], matches[2], matches[3]),
		}}
	}

	if matches := noOutputPortRe.FindStringSubmatch(errStr); len(matches) > 3 {
		return []Diagnostic{{
			NodeID:   matches[1],
			NodeType: matches[2],
			PortName: matches[3],
			Kind:     "missing_output",
			Message:  fmt.Sprintf("Node %s (%s) does not have output port %s", matches[1], matches[2], matches[3]),
		}}
	}

	if matches := unknownPrimRe.FindStringSubmatch(errStr); len(matches) > 1 {
		nodeID := ""
		if len(matches) > 2 {
			nodeID = matches[2]
		}
		return []Diagnostic{{
			NodeID:   nodeID,
			NodeType: matches[1],
			Kind:     "unknown_primitive",
			Message:  fmt.Sprintf("Unknown primitive type %s", matches[1]),
		}}
	}

	if strings.Contains(errStr, "cycle detected") {
		return []Diagnostic{{
			Kind:    "cycle",
			Message: "Cycle detected in graph execution order",
		}}
	}

	if matches := missingDefRe.FindStringSubmatch(errStr); len(matches) > 1 {
		nodeID := ""
		if len(matches) > 2 {
			nodeID = matches[2]
		}
		return []Diagnostic{{
			NodeID:  nodeID,
			Kind:    "missing_definition",
			Message: fmt.Sprintf("Definition not found: %s", matches[1]),
		}}
	}

	return []Diagnostic{{
		Kind:    "compile_error",
		Message: errStr,
	}}
}

func nodeMatchesSystem(nodeType, sysName string) bool {
	nt := strings.ToLower(strings.TrimSpace(nodeType))
	sn := strings.ToLower(strings.TrimSpace(sysName))
	if nt == sn {
		return true
	}

	ntParts := strings.Split(nt, ".")
	snParts := strings.Split(sn, ".")
	if len(ntParts) >= 2 && len(snParts) >= 2 && ntParts[0] == snParts[0] {
		if strings.Contains(ntParts[1], snParts[1]) || strings.Contains(snParts[1], ntParts[1]) {
			return true
		}
	}

	return strings.Contains(nt, sn) || strings.Contains(sn, nt)
}

func init() {
	http.RegisterWorkbenchRunner(NewWorkbenchRunner())
}
