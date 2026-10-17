package strategy

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
tapeSample is one trade on the tape. The plot draws every sample, so the
frame is the whole tape rather than a window around one excursion.
*/
type tapeSample struct {
	Symbol string  `json:"symbol"`
	Tick   int64   `json:"tick"`
	Seq    int64   `json:"seq"`
	Price  float64 `json:"price"`
}

/*
tapeDecision is one branch the detector took. Flush also carries the four
marks it wrote, so a collapsed window is visible on the tape.
*/
type tapeDecision struct {
	Symbol string  `json:"symbol"`
	Tick   int64   `json:"tick"`
	Seq    int64   `json:"seq"`
	Price  float64 `json:"price"`
	Action string  `json:"action"`
	Detail string  `json:"detail"`
	ATick  int64   `json:"a_tick,omitempty"`
	BTick  int64   `json:"b_tick,omitempty"`
	CTick  int64   `json:"c_tick,omitempty"`
	DTick  int64   `json:"d_tick,omitempty"`
	AIdx   int64   `json:"a_idx,omitempty"`
	BIdx   int64   `json:"b_idx,omitempty"`
	CIdx   int64   `json:"c_idx,omitempty"`
	DIdx   int64   `json:"d_idx,omitempty"`
}

type decisionTrace struct {
	samples   []tapeSample
	decisions []tapeDecision
}

/*
EnableTrace records every trade and every detector branch for the replay.
*/
func (detector *Detector) EnableTrace() {
	if detector == nil {
		return
	}

	detector.trace = &decisionTrace{}
}

func (detector *Detector) sample(pt tradePoint) {
	if detector == nil || detector.trace == nil {
		return
	}

	detector.trace.samples = append(detector.trace.samples, tapeSample{
		Symbol: detector.label,
		Tick:   pt.tick,
		Seq:    pt.seqIdx,
		Price:  pt.priceF,
	})
}

func (detector *Detector) note(action string, pt tradePoint, detail string) {
	if detector == nil || detector.trace == nil {
		return
	}

	detector.trace.decisions = append(detector.trace.decisions, tapeDecision{
		Symbol: detector.label,
		Tick:   pt.tick,
		Seq:    pt.seqIdx,
		Price:  pt.priceF,
		Action: action,
		Detail: detail,
	})
}

func (detector *Detector) noteMarks(
	class string,
	aTick, bTick, cTick, dTick int64,
	aIdx, bIdx, cIdx, dIdx int64,
	price *decimal.Decimal,
) {
	if detector == nil || detector.trace == nil {
		return
	}

	px := 0.0

	if price != nil {
		px = price.Float64()
	}

	detector.trace.decisions = append(detector.trace.decisions, tapeDecision{
		Symbol: detector.label,
		Tick:   cTick,
		Seq:    cIdx,
		Price:  px,
		Action: "flush",
		Detail: class,
		ATick:  aTick,
		BTick:  bTick,
		CTick:  cTick,
		DTick:  dTick,
		AIdx:   aIdx,
		BIdx:   bIdx,
		CIdx:   cIdx,
		DIdx:   dIdx,
	})
}

/*
WriteTrace writes the tape and the decisions as one JSON document.
*/
func (detector *Detector) WriteTrace(path string) error {
	if detector == nil || detector.trace == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[detector] trace was not enabled", nil,
		))
	}

	body := struct {
		Samples   []tapeSample   `json:"samples"`
		Decisions []tapeDecision `json:"decisions"`
	}{
		Samples:   detector.trace.samples,
		Decisions: detector.trace.decisions,
	}

	if body.Samples == nil {
		body.Samples = []tapeSample{}
	}

	if body.Decisions == nil {
		body.Decisions = []tapeDecision{}
	}

	raw, err := json.Marshal(body)

	if err != nil {
		return errnie.Error(err)
	}

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return errnie.Error(err)
	}

	textPath := strings.TrimSuffix(path, ".json") + "_decisions.txt"
	lines := make([]string, 0, len(body.Decisions)+1)
	lines = append(lines, fmt.Sprintf("trades %d", len(body.Samples)))

	for _, decision := range body.Decisions {
		line := fmt.Sprintf(
			"%s %s tick=%d seq=%d price=%.8g %s",
			decision.Symbol, decision.Action, decision.Tick, decision.Seq, decision.Price, decision.Detail,
		)

		if decision.Action == "flush" {
			line += fmt.Sprintf(
				" marks a=%d b=%d c=%d d=%d aIdx=%d bIdx=%d cIdx=%d dIdx=%d",
				decision.ATick, decision.BTick, decision.CTick, decision.DTick,
				decision.AIdx, decision.BIdx, decision.CIdx, decision.DIdx,
			)
		}

		lines = append(lines, line)
	}

	if err := os.WriteFile(textPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return errnie.Error(err)
	}

	return nil
}
