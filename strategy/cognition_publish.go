package strategy

import (
	"encoding/json"
	"io"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/ui"
)

type CognitionFrame struct {
	Epoch    uint64              `json:"epoch,string"`
	Paths    int                 `json:"paths"`
	Trading  bool                `json:"trading"`
	Keys     []string            `json:"keys"`
	Branches []ui.TrieBranchJSON `json:"branches"`
	Problem  *CognitionProblem   `json:"problem"`
}

type CognitionProblem struct {
	Level  string `json:"level"`
	Reason string `json:"reason"`
	Fix    string `json:"fix"`
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
