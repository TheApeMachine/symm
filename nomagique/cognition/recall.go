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
Recall reads the class associated with a context.
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

			query := (*RecallQuery)(arriving)

			if query == nil {
				op.Error(core.ErrShape)
				return
			}

			root := op.memory.root.Load()

			if root == nil {
				op.Error(core.ErrShape)
				return
			}

			context := query.Context
			stance := query.Stance

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
			var strengths []float64

			for _, candidateQuery := range queries {
				before := len(names)
				prefix := make([]byte, 2+len(candidateQuery)+1)
				prefix[0] = 'b'
				prefix[1] = '/'
				copy(prefix[2:], candidateQuery)
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

					if className == "wait" || (stance != "" && className != stance) {
						continue
					}

					count := binary.LittleEndian.Uint64(value[0:8])
					probabilityVal := math.Float64frombits(binary.LittleEndian.Uint64(value[8:16]))
					mass := float64(count) * probabilityVal
					placed := false

					for index := range names {
						if names[index] != className {
							continue
						}

						if mass > masses[index] {
							masses[index] = mass
							supports[index] = float64(count)
							strengths[index] = probabilityVal
						}

						placed = true
						break
					}

					if !placed {
						names = append(names, className)
						masses = append(masses, mass)
						supports = append(supports, float64(count))
						strengths = append(strengths, probabilityVal)
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

			if total > 0 {
				argmax := probability.NewArgmax()
				var argmaxVals [2]float64
				argmaxIdx := 0

				for pointer := range argmax.Next(data.NewValue(masses...).Next(nil)) {
					if argmaxIdx < 2 {
						argmaxVals[argmaxIdx] = *(*float64)(pointer)
						argmaxIdx++
					}
				}

				if err := argmax.Error(); err != nil {
					op.Error(err)
					return
				}

				winnerIndex = int(argmaxVals[0])

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

				for pointer := range normalize.Next(data.NewValue(masses[winnerIndex], total).Next(nil)) {
					confidence = *(*float64)(pointer)
				}

				if err := normalize.Error(); err != nil {
					op.Error(err)
					return
				}

				if stance != "" && strengths[winnerIndex] <= core.Unit/2 {
					winner = ""
				}
			}

			if total > 0 && runnerIndex >= 0 {
				runner = names[runnerIndex]
				normalize := probability.NewNormalize()
				runnerShare := 0.0

				for pointer := range normalize.Next(data.NewValue(masses[runnerIndex], total).Next(nil)) {
					runnerShare = *(*float64)(pointer)
				}

				if err := normalize.Error(); err != nil {
					op.Error(err)
					return
				}

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

				for pointer := range entropy.Next(data.NewValue(masses...).Next(nil)) {
					ambiguity = *(*float64)(pointer)
				}

				if err := entropy.Error(); err != nil {
					op.Error(err)
					return
				}
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

			result := &RecallResult{
				Winner:       winner,
				RunnerUp:     runner,
				Confidence:   confidence,
				Contrast:     contrast,
				Support:      support,
				Ambiguity:    ambiguity,
				Surprisal:    surprisal,
				HasSurprisal: surprisalHeld,
			}

			for value := range data.NewValue(unsafe.Pointer(result)).Next(nil) {
				if !yield(value) {
					return
				}
			}
		}
	}
}
