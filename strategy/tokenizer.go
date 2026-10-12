package strategy

import (
	"fmt"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
Tokenizer turns the grid's brightness-sorted regions into tokens and tokens
into token paths. Its three parameters are what the trainer optimizes:

  - Regions: how many of the brightest regions make up a token (Top-K).
  - Threshold: the brightness the weakest of those regions must reach before
    a token may move the path; dimmer frames leave the path as it is.
  - Depth: how many tokens a path holds (the precursor length).

Production and the trainer both tokenize through these methods, so a stored
path is exactly the path the live market produces.
*/
type Tokenizer struct {
	Depth     int     `json:"depth"`
	Regions   int     `json:"regions"`
	Threshold float64 `json:"threshold"`
}

/*
tokenizerKey is the S3 key the trainer publishes the chosen Tokenizer under.
*/
const tokenizerKey = "tokenizer.json"

/*
Token names the frame's Top-K regions in brightness order, e.g. "R03-R07",
and returns the brightness of the weakest of them: the token is only as
strong as its dimmest member.
*/
func (tokenizer *Tokenizer) Token(regions []*store.Region) (string, float64) {
	keys := make([]string, tokenizer.Regions)

	for idx, region := range regions[:tokenizer.Regions] {
		keys[idx] = fmt.Sprintf("R%02d", region.Key)
	}

	return strings.Join(keys, "-"), regions[tokenizer.Regions-1].Brightness
}

/*
Advance moves the path to token when the token is strong enough and differs
from the path's last token, keeping the last Depth tokens. It returns a new
slice, never one sharing memory with path, and whether the path moved.
*/
func (tokenizer *Tokenizer) Advance(path []string, token string, strength float64) ([]string, bool) {
	if strength < tokenizer.Threshold {
		return path, false
	}

	if len(path) > 0 && path[len(path)-1] == token {
		return path, false
	}

	kept := path[max(len(path)-tokenizer.Depth+1, 0):]
	next := make([]string, 0, len(kept)+1)
	return append(append(next, kept...), token), true
}

/*
validate rejects parameters no path can be built from.
*/
func (tokenizer *Tokenizer) validate() error {
	if tokenizer.Depth < 1 || tokenizer.Regions < 1 || tokenizer.Regions > store.RegionCount {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"[tokenizer] depth %d must be at least 1 and regions %d within [1, %d]",
				tokenizer.Depth, tokenizer.Regions, store.RegionCount,
			),
			nil,
		))
	}

	return nil
}
