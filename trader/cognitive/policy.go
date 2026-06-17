package cognitive

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"

	"github.com/theapemachine/symm/logic"
)

/*
Profile holds execution parameters learned from prior fills in a regime.
*/
type Profile struct {
	SlippageBps  float64 `json:"slippage_bps"`
	SizeFraction float64 `json:"size_fraction"`
}

/*
Staging is speculative execution state pre-warmed from beam-search lookahead.
*/
type Staging struct {
	Scope          string
	Sequence       []byte
	Profile        []byte
	LookaheadPaths []string
	LookaheadScore float64
	PreparedAt     int64
}

/*
ApplyAction adjusts action sizing from analog profiles and contrast evidence.
*/
func (memory *Memory) ApplyAction(action *logic.Action, reading *Reading) {
	if memory == nil || action == nil || reading == nil {
		return
	}

	profileRaw, found := memory.LookupProfile(reading.Sequence)

	if found {
		applyProfile(action, profileRaw)
	}

	if reading.ContrastEvidence > reading.contrastFloor() && action.Fraction > 0 {
		action.Fraction *= 1.0 + reading.ContrastEvidence

		if action.EntryConfidence > 0 {
			action.EntryConfidence = math.Min(
				1.0,
				action.EntryConfidence*(1.0+reading.ContrastEvidence),
			)
		}
	}

	if reading.Ambiguous && action.Fraction > 0 {
		action.Fraction *= reading.ambiguousDampening()
	}
}

/*
PreWarmStaging builds execution staging from the latest sealed reading.
*/
func (memory *Memory) PreWarmStaging(reading *Reading) Staging {
	if memory == nil || reading == nil {
		return Staging{}
	}

	profile, _ := memory.LookupProfile(reading.Sequence)

	return Staging{
		Scope:          reading.Scope,
		Sequence:       append([]byte(nil), reading.Sequence...),
		Profile:        append([]byte(nil), profile...),
		LookaheadPaths: append([]string(nil), reading.LookaheadPaths...),
		LookaheadScore: reading.LookaheadScore,
	}
}

/*
RecordOutcome commits episodic memory and stores a reinforced execution profile.
*/
func (memory *Memory) RecordOutcome(
	sequence []byte,
	profile []byte,
	eventAt int64,
) {
	if memory == nil || memory.tree == nil || len(sequence) == 0 {
		return
	}

	if eventAt <= 0 {
		eventAt = 1
	}

	_, _ = memory.tree.CommitToEpisodicBuffer(uint64(eventAt), sequence)
	memory.tree.TrainSensorySequence(sequence)

	if len(profile) > 0 {
		_, _ = memory.StoreProfile(sequence, profile)
	}
}

/*
ProfileFromExecution encodes fill telemetry into a storable profile blob.
*/
func ProfileFromExecution(slippageBps float64, sizeFraction float64) []byte {
	if slippageBps <= 0 && sizeFraction <= 0 {
		return nil
	}

	profile := Profile{
		SlippageBps:  slippageBps,
		SizeFraction: sizeFraction,
	}

	raw, err := json.Marshal(profile)

	if err != nil {
		return nil
	}

	return raw
}

func applyProfile(action *logic.Action, profileRaw []byte) {
	profile := parseProfile(profileRaw)

	if profile.SizeFraction > 0 && action.Fraction > 0 {
		action.Fraction *= profile.SizeFraction
	}

	if profile.SlippageBps > 0 {
		action.Offset = profile.SlippageBps / 10000.0
	}
}

func parseProfile(profileRaw []byte) Profile {
	if len(profileRaw) == 0 {
		return Profile{}
	}

	var profile Profile

	if json.Unmarshal(profileRaw, &profile) == nil {
		return profile
	}

	parsed := Profile{}
	segments := bytes.Split(profileRaw, []byte("&"))

	for _, segment := range segments {
		parts := bytes.SplitN(segment, []byte("="), 2)

		if len(parts) != 2 {
			continue
		}

		value, err := strconv.ParseFloat(string(parts[1]), 64)

		if err != nil {
			continue
		}

		switch string(parts[0]) {
		case "slippage_bps", "slippage":
			parsed.SlippageBps = value
		case "size_fraction", "fraction":
			parsed.SizeFraction = value
		}
	}

	return parsed
}

func (reading *Reading) contrastFloor() float64 {
	if reading == nil || reading.ClassConfidence <= 0 {
		return math.MaxFloat64
	}

	return 1.0 / (reading.ClassConfidence + 1.0)
}

func (reading *Reading) ambiguousDampening() float64 {
	if reading == nil || reading.EntropyThreshold <= 0 {
		return 1.0
	}

	if reading.EntropyBits <= 0 {
		return 1.0
	}

	ratio := reading.EntropyBits / reading.EntropyThreshold

	if ratio <= 1.0 {
		return 1.0
	}

	return 1.0 / ratio
}

func symbolFromSequence(sequence []byte) string {
	lastSeparator := bytes.LastIndexByte(sequence, '_')

	if lastSeparator < 0 || lastSeparator >= len(sequence)-1 {
		return ""
	}

	return string(sequence[lastSeparator+1:])
}
