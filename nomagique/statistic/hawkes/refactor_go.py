import os

servers = {
    "assemble": {
        "struct": "AssembleServer",
        "method": "Assemble",
        "imports": ["context", "fmt", "github.com/bytedance/sonic"],
        "downstream_type": "[2]float64",
        "code": """	var in map[string]any
	if payloadPtr.IsValid() {
		data := payloadPtr.Data()
		if len(data) > 0 {
			_ = sonic.Unmarshal(data, &in)
		}
	}
	
	ts := 0.0
	sideStr := ""
	
	if in != nil {
		if t, ok := in["timestamp"].(float64); ok {
			ts = t
		} else if t, ok := in["timestamp"].(int64); ok {
			ts = float64(t)
		} else if t, ok := in["time"].(float64); ok {
			ts = t
		} else if t, ok := in["time"].(int64); ok {
			ts = float64(t)
		}
		
		if sv, ok := in["side"].(string); ok {
			sideStr = sv
		} else if sideAny, exists := in["side"]; exists {
			sideStr = fmt.Sprint(sideAny)
		}
	}
	
	mark := 1.0
	if sideStr == "sell" || sideStr == "s" {
		mark = -1.0
	}
	
	result := [2]float64{ts, mark}
	return s.Downstream(ctx, result)"""
    },
    "process": {
        "struct": "ProcessServer",
        "method": "Process",
        "imports": ["context", "time", "github.com/bytedance/sonic", "github.com/theapemachine/symm/nomagique/core"],
        "downstream_type": "*Reading",
        "extra": """type ProcessServer struct {
	state *path
	Downstream func(context.Context, *Reading) error
}

func NewProcessServer() *ProcessServer {
	return &ProcessServer{
		state: &path{samples: make([]sample, 0)},
	}
}""",
        "code": """	var event [2]float64
	if payloadPtr.IsValid() {
		data := payloadPtr.Data()
		if len(data) > 0 {
			_ = sonic.Unmarshal(data, &event)
		}
	}

	atNano := int64(event[0])
	markRaw := event[1]

	if atNano <= 0 {
		return nil
	}

	at := time.Unix(0, atNano)

	if s.state.hasLast && at.Before(s.state.lastAt) {
		return nil
	}

	mark := -core.Unit
	if markRaw > 0 {
		mark = core.Unit
	}

	buyArrivals, sellArrivals := s.state.sides()
	countBuy := float64(len(buyArrivals))
	countSell := float64(len(sellArrivals))

	if mark > 0 {
		countBuy++
	}
	if mark <= 0 {
		countSell++
	}

	count := countBuy + countSell
	reading := Reading{
		EventCount:   count,
		BuyCount:     countBuy,
		SellCount:    countSell,
		BuyFraction:  countBuy / count,
		SellFraction: countSell / count,
	}

	from := at
	if len(s.state.samples) > 0 {
		from = s.state.origin()
	}

	atSec := float64(atNano) * 1e-9
	fromSec := float64(from.UnixNano()) * 1e-9
	span := atSec - fromSec

	if span > 0 {
		reading.HasRates = true
		reading.BuyRate = countBuy / span
		reading.SellRate = countSell / span
		reading.ArrivalRate = count / span
	}

	if s.state.modelReady {
		reading.HasFit = true
		evaluateReading(&reading, s.state, buyArrivals, sellArrivals, atSec)
	}

	s.state.lastAt = at
	s.state.hasLast = true
	s.state.remember(at, atSec, mark)
	s.state.refit(atSec)

	return s.Downstream(ctx, &reading)"""
    }
}

views = [
    ("event_count", "EventCount", "reading.EventCount"),
    ("buy_count", "BuyCount", "reading.BuyCount"),
    ("sell_count", "SellCount", "reading.SellCount"),
    ("buy_fraction", "BuyFraction", "reading.BuyFraction"),
    ("sell_fraction", "SellFraction", "reading.SellFraction"),
    ("arrival_rate", "ArrivalRate", "reading.ArrivalRate"),
    ("buy_rate", "BuyRate", "reading.BuyRate"),
    ("sell_rate", "SellRate", "reading.SellRate"),
    ("conditional_intensity", "ConditionalIntensity", "reading.Lambda"),
    ("buy_intensity", "BuyIntensity", "reading.LambdaBuy"),
    ("sell_intensity", "SellIntensity", "reading.LambdaSell"),
    ("spectral_radius", "SpectralRadius", "reading.SpectralRadius"),
]

for filename, structName, resultExpr in views:
    servers[filename] = {
        "struct": f"{structName}Server",
        "method": structName,
        "imports": ["context"],
        "downstream_type": "float64",
        "extra": f"""type {structName}Server struct {{
	Downstream func(context.Context, float64) error
}}

func New{structName}Server() *{structName}Server {{
	return &{structName}Server{{}}
}}""",
        "code": f"""	reading, err := extractReading(payloadPtr)
	if err != nil || reading == nil {{
		return nil
	}}
	result := {resultExpr}
	return s.Downstream(ctx, result)"""
    }

for filename, spec in servers.items():
    if "extra" not in spec:
        spec["extra"] = f"""type {spec["struct"]} struct {{
	Downstream func(context.Context, {spec["downstream_type"]}) error
}}

func New{spec["struct"]}() *{spec["struct"]} {{
	return &{spec["struct"]}{{}}
}}"""
        
    imports_str = "\n".join([f'\t"{i}"' for i in spec["imports"]])
    arg_call = "View" if filename in [v[0] for v in views] else spec["method"]
    
    content = f"""package hawkes

import (
{imports_str}
)

{spec["extra"]}

func (s *{spec["struct"]}) Write(ctx context.Context, call {spec["method"]}_write) error {{
	args, err := call.Args().{arg_call}()
	if err != nil {{
		return err
	}}
	
	payloadPtr, err := args.Payload()
	if err != nil {{
		return err
	}}

	if s.Downstream == nil {{
		return nil
	}}
	
{spec["code"]}
}}

func (s *{spec["struct"]}) Done(ctx context.Context, call {spec["method"]}_done) error {{
	return nil
}}
"""
    with open(f"/Users/theapemachine/go/src/github.com/theapemachine/symm/nomagique/statistic/hawkes/{filename}.go", "w") as f:
        f.write(content)
