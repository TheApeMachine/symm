package strategy

import (
	"encoding/json"
	"io"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/ui"
)

type CognitionGateFrame struct {
	TickEpoch    string  `json:"tickEpoch"`
	Enabled      bool    `json:"enabled"`
	SurpriseZ    float64 `json:"surpriseZ"`
	Warmup       string  `json:"warmup"`
	LearningRate float64 `json:"learningRate"`
}

type CognitionFrame struct {
	Epoch    uint64              `json:"epoch,string"`
	Paths    int                 `json:"paths"`
	Trading  bool                `json:"trading"`
	Gate     *CognitionGateFrame `json:"gate"`
	Keys     []string            `json:"keys"`
	Branches []ui.TrieBranchJSON `json:"branches"`
	Problem  *CognitionProblem   `json:"problem"`
}

type CognitionProblem struct {
	Level  string `json:"level"`
	Reason string `json:"reason"`
	Fix    string `json:"fix"`
}

/*
CognitionMatchFrame is one symbol's live token path against the stored tree.
*/
type CognitionMatchFrame struct {
	Symbol string   `json:"symbol"`
	Tick   uint64   `json:"tick"`
	Tokens []string `json:"tokens"`
	Stored bool     `json:"stored"`
	Match  string   `json:"match"`
	Depth  int      `json:"depth"`
	Action string   `json:"action"`
}

func PublishCognitionTree(w io.Writer, training *Training, trading bool) error {
	if w == nil || training == nil {
		return errnie.Err(errnie.Validation, "[cognition] publishing needs a writer and training", nil)
	}

	export := training.CognitionTree()
	keys := export.Keys

	if keys == nil {
		keys = []string{}
	}

	branches := export.Branches

	if branches == nil {
		branches = []ui.TrieBranchJSON{}
	}

	frame := CognitionFrame{
		Epoch:    data.Epoch.Load(),
		Paths:    len(keys),
		Trading:  trading && len(keys) > 0,
		Keys:     keys,
		Branches: branches,
	}

	if len(keys) == 0 {
		frame.Problem = &CognitionProblem{
			Level:  "warning",
			Reason: "no trained policy is loaded",
			Fix:    "run detect and train on a collected tick epoch",
		}
		frame.Trading = false
	}

	buf, err := json.Marshal(frame)

	if err != nil {
		return errnie.Err(errnie.Internal, "[cognition] tree frame", err)
	}

	out := data.NewValue(data.CognitionOrigin, data.CognitionTreeRole, data.PolicyScope)
	out.FormatPrefix()
	out.AppendJSON(buf, data.LayerCognition)

	if _, err := io.Copy(w, out); err != nil {
		return errnie.Err(errnie.IO, "[cognition] tree frame to the hub", err)
	}

	return nil
}

/*
publishCognitionMatch writes one symbol's live path. The hub keeps the latest
frame per symbol and replays it when a dashboard connects.
*/
func (training *Training) publishCognitionMatch(tick *data.Value, symbol string, tokens []string, matched string, depth int, action string) error {
	if training == nil || training.hub == nil || symbol == "" || tick == nil {
		return nil
	}

	if tokens == nil {
		tokens = []string{}
	}

	buf, err := json.Marshal(CognitionMatchFrame{
		Symbol: symbol,
		Tick:   tick.Tick,
		Tokens: tokens,
		Stored: action != "",
		Match:  matched,
		Depth:  depth,
		Action: action,
	})

	if err != nil {
		return errnie.Err(errnie.Internal, "[cognition] match frame of "+symbol, err)
	}

	out := data.NewValue(data.CognitionOrigin, data.CognitionMatchRole, symbol)
	out.Epoch, out.Tick, out.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp
	out.FormatPrefix()
	out.AppendJSON(buf, data.LayerCognition)

	if err := training.writeFrame("cognition/match/"+symbol, out); err != nil {
		return errnie.Err(errnie.IO, "[cognition] match frame of "+symbol, err)
	}

	return nil
}
