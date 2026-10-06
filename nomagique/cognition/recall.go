package cognition

import (
	"bytes"
	"encoding/binary"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/probability"
)

/*
Recall reads the class associated with a context. An exact basin hit wins.
Failing that, the longest stored prefix is tried, then the longest stored
suffix. Equal leading masses abstain. Contrast is the log ratio of the
leading share to the next share. Surprisal is the information of the
sensory count against the observation clock, and is published only when
that sensory record exists.
*/
type Recall struct {
	*core.PrimitiveError
	memory *Associate
}

func NewRecall(memory *Associate) *Recall {
	return &Recall{
		PrimitiveError: core.NewPrimitiveError(),
		memory:         memory,
	}
}

func (op *Recall) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil || op.memory == nil {
				op.Error(core.ErrShape)
				return
			}

			adapter := *(**data.Adapter)(arriving)

			if adapter == nil {
				op.Error(core.ErrShape)
				return
			}

			root := op.memory.root.Load()

			if root == nil {
				op.Error(core.ErrShape)
				return
			}

			var text data.Map[string]

			for pointer := range adapter.Next(data.NewValue(data.NewLiteral("context"))) {
				text = *(*data.Map[string])(pointer)
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			context, contextOK := text.Values["context"]

			if !contextOK {
				op.Error(core.ErrNotHeld)
				return
			}

			if context == "" {
				op.Error(core.ErrDomain)
				return
			}

			contextBytes := []byte(context)
			queries := [][]byte{contextBytes}
			var prefixes [][]byte
			var suffixes [][]byte
			framed := len(contextBytes) >= 12
			var frameAt []int
			offset := 0

			for framed && offset < len(contextBytes) {
				if offset+4 > len(contextBytes) {
					framed = false
					break
				}

				count := int(binary.BigEndian.Uint32(contextBytes[offset : offset+4]))

				if count == 0 || offset+4+count*8 > len(contextBytes) {
					framed = false
					break
				}

				frameAt = append(frameAt, offset)
				offset += 4 + count*8
			}

			if framed && (offset != len(contextBytes) || len(frameAt) < 2) {
				framed = false
			}

			if framed {
				for step := 1; step < len(frameAt); step++ {
					prefixes = append(prefixes, contextBytes[:frameAt[len(frameAt)-step]])
					suffixes = append(suffixes, contextBytes[frameAt[step]:])
				}
			}

			aligned := !framed && len(contextBytes) > 8 && len(contextBytes)%8 == 0

			if aligned {
				tokens := len(contextBytes) / 8

				for step := 1; step < tokens; step++ {
					prefixes = append(prefixes, contextBytes[:len(contextBytes)-step*8])
					suffixes = append(suffixes, contextBytes[step*8:])
				}
			}

			delim := byte(0)
			delimited := false

			if !framed && !aligned && bytes.Contains(contextBytes, []byte{0}) {
				delimited = true
			}

			if !framed && !aligned && !delimited && bytes.Contains(contextBytes, []byte{'/'}) {
				delim = '/'
				delimited = true
			}

			if !framed && !aligned && !delimited && bytes.Contains(contextBytes, []byte{'_'}) {
				delim = '_'
				delimited = true
			}

			if delimited {
				var cuts []int

				for index, item := range contextBytes {
					if item == delim {
						cuts = append(cuts, index)
					}
				}

				for index := len(cuts) - 1; index >= 0; index-- {
					cut := cuts[index]

					if cut > 0 {
						prefixes = append(prefixes, contextBytes[:cut])
					}
				}

				for _, cut := range cuts {
					if cut+1 < len(contextBytes) {
						suffixes = append(suffixes, contextBytes[cut+1:])
					}
				}
			}

			if !framed && !aligned && !delimited && len(contextBytes) > 1 {
				half := len(contextBytes) / 2
				prefixes = append(prefixes, contextBytes[:half])
				suffixes = append(suffixes, contextBytes[half:])
			}

			queries = append(queries, prefixes...)
			queries = append(queries, suffixes...)

			var names []string
			var masses []float64
			var supports []float64

			for _, query := range queries {
				before := len(names)
				prefix := make([]byte, 2+len(query)+1)
				prefix[0] = 'b'
				prefix[1] = '/'
				copy(prefix[2:], query)
				prefix[len(prefix)-1] = '/'
				iterator := root.Root().Iterator()
				iterator.SeekPrefix(prefix)

				for key, value, found := iterator.Next(); found; key, value, found = iterator.Next() {
					if !bytes.HasPrefix(key, prefix) {
						break
					}

					if len(key) < 4 || key[0] != 'b' || key[1] != '/' || len(value) != 24 {
						continue
					}

					rest := key[2:]
					slash := bytes.LastIndexByte(rest, '/')

					if slash <= 0 || slash == len(rest)-1 {
						continue
					}

					className := string(rest[slash+1:])
					count := binary.LittleEndian.Uint64(value[0:8])
					probability := math.Float64frombits(binary.LittleEndian.Uint64(value[8:16]))
					mass := float64(count) * probability
					placed := false

					for index := range names {
						if names[index] != className {
							continue
						}

						if mass > masses[index] {
							masses[index] = mass
							supports[index] = float64(count)
						}

						placed = true
						break
					}

					if !placed {
						names = append(names, className)
						masses = append(masses, mass)
						supports = append(supports, float64(count))
					}
				}

				if len(names) > before {
					break
				}
			}

			total := 0.0

			for _, mass := range masses {
				total += mass
			}

			winner := ""
			runner := ""
			confidence := 0.0
			contrast := 0.0
			support := 0.0
			ambiguity := 0.0
			winnerIndex := -1
			runnerIndex := -1
			bridgeState := data.NewState(data.NewMap())
			bridge := data.NewAdapter(nil, bridgeState)
			issued := data.NewOutputMap()

			if total > 0 {
				argmax := probability.NewArgmax()

				for _, mass := range masses {
					issued.Values["value"] = mass

					for range bridge.Next(data.NewValue(issued)) {
					}

					if err := bridge.Error(); err != nil {
						op.Error(err)
						return
					}

					for range argmax.Next(data.NewValue(bridge)) {
					}

					if err := argmax.Error(); err != nil {
						op.Error(err)
						return
					}
				}

				var chosen data.Map[float64]

				for pointer := range bridge.Next(data.NewValue(data.NewMap(
					"winner_index", "winner_index",
					"winner_value", "winner_value",
				))) {
					chosen = *(*data.Map[float64])(pointer)
				}

				if err := bridge.Error(); err != nil {
					op.Error(err)
					return
				}

				winnerIndex = int(chosen.Values["winner_index"])

				if winnerIndex < 0 || winnerIndex >= len(names) {
					op.Error(core.ErrShape)
					return
				}

				winner = names[winnerIndex]
				support = supports[winnerIndex]
				runnerMass := -1.0

				for index, mass := range masses {
					if index == winnerIndex {
						continue
					}

					if mass > runnerMass {
						runnerMass = mass
						runnerIndex = index
					}
				}

				normalize := probability.NewNormalize()
				issued.Values["value"] = masses[winnerIndex]
				issued.Values["total"] = total

				for range bridge.Next(data.NewValue(issued)) {
				}

				if err := bridge.Error(); err != nil {
					op.Error(err)
					return
				}

				for range normalize.Next(data.NewValue(bridge)) {
				}

				if err := normalize.Error(); err != nil {
					op.Error(err)
					return
				}

				var share data.Map[float64]

				for pointer := range bridge.Next(data.NewValue(data.NewMap("normalized", "normalized"))) {
					share = *(*data.Map[float64])(pointer)
				}

				if err := bridge.Error(); err != nil {
					op.Error(err)
					return
				}

				confidence = share.Values["normalized"]
			}

			if total > 0 && runnerIndex >= 0 {
				runner = names[runnerIndex]
				issued.Values["value"] = masses[runnerIndex]
				issued.Values["total"] = total
				normalize := probability.NewNormalize()

				for range bridge.Next(data.NewValue(issued)) {
				}

				if err := bridge.Error(); err != nil {
					op.Error(err)
					return
				}

				for range normalize.Next(data.NewValue(bridge)) {
				}

				if err := normalize.Error(); err != nil {
					op.Error(err)
					return
				}

				var share data.Map[float64]

				for pointer := range bridge.Next(data.NewValue(data.NewMap("normalized", "normalized"))) {
					share = *(*data.Map[float64])(pointer)
				}

				if err := bridge.Error(); err != nil {
					op.Error(err)
					return
				}

				runnerShare := share.Values["normalized"]

				if confidence > 0 && runnerShare > 0 {
					contrast = math.Log2(confidence / runnerShare)
				}

				if masses[runnerIndex] == masses[winnerIndex] {
					winner = ""
					runner = ""
				}
			}

			if total > 0 {
				entropy := probability.NewAmbiguity()

				for _, mass := range masses {
					issued.Values["value"] = mass

					for range bridge.Next(data.NewValue(issued)) {
					}

					if err := bridge.Error(); err != nil {
						op.Error(err)
						return
					}

					for range entropy.Next(data.NewValue(bridge)) {
					}

					if err := entropy.Error(); err != nil {
						op.Error(err)
						return
					}
				}

				var ambiguityValues data.Map[float64]

				for pointer := range bridge.Next(data.NewValue(data.NewMap("ambiguity", "ambiguity"))) {
					ambiguityValues = *(*data.Map[float64])(pointer)
				}

				if err := bridge.Error(); err != nil {
					op.Error(err)
					return
				}

				ambiguity = ambiguityValues.Values["ambiguity"]
			}

			surprisalHeld := false
			surprisal := 0.0
			clock := uint64(0)

			if raw, held := root.Get([]byte{0}); held && len(raw) == 8 {
				clock = binary.BigEndian.Uint64(raw)
			}

			sensory := make([]byte, 2+len(contextBytes))
			sensory[0] = 's'
			sensory[1] = '/'
			copy(sensory[2:], contextBytes)
			raw, sensoryHeld := root.Get(sensory)

			if sensoryHeld && len(raw) == 24 && clock > 0 {
				count := binary.LittleEndian.Uint64(raw[0:8])

				if count > 0 && count <= clock {
					surprisal = -math.Log2(float64(count) / float64(clock))
					surprisalHeld = true
				}
			}

			published := data.NewOutputMap()
			published.Values["confidence"] = confidence
			published.Values["contrast"] = contrast
			published.Values["support"] = support
			published.Values["ambiguity"] = ambiguity

			if surprisalHeld {
				published.Values["surprisal"] = surprisal
			}

			for range adapter.Next(data.NewValue(published)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			label := data.NewTextMap()
			label.Values["winner"] = winner
			label.Values["runner_up"] = runner

			for range adapter.Next(data.NewValue(label)) {
			}

			if err := adapter.Error(); err != nil {
				op.Error(err)
				return
			}

			if !yield(arriving) {
				return
			}
		}
	}
}
