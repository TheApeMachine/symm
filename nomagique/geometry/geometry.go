package geometry

import (
	"context"
	"fmt"
	"math"
	"math/cmplx"
	"sort"
	"sync"
	"time"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
PhaseDial is a high-dimensional complex vector of rotational phase gradients.
It is the internal Go representation of WirePhaseDial.
*/
type PhaseDial []complex128

func wireToPhaseDial(wire WirePhaseDial) (PhaseDial, error) {
	if !wire.IsValid() {
		return nil, fmt.Errorf("invalid wire phase dial")
	}
	
	comp, err := wire.Components()
	if err != nil {
		return nil, err
	}
	
	if comp.Len() % 2 != 0 {
		return nil, fmt.Errorf("wire phase dial components must be even (real/imag pairs)")
	}
	
	dial := make(PhaseDial, comp.Len()/2)
	for i := 0; i < comp.Len(); i += 2 {
		dial[i/2] = complex(comp.At(i), comp.At(i+1))
	}
	
	return dial, nil
}

func phaseDialToWire(dial PhaseDial, list capnp.Float64List) {
	for i, c := range dial {
		list.Set(i*2, real(c))
		list.Set(i*2+1, imag(c))
	}
}

func dialNorm(dial PhaseDial) float64 {
	var total float64
	for _, value := range dial {
		re, im := real(value), imag(value)
		total += re*re + im*im
	}
	return math.Sqrt(total)
}

func normalizeDial(dial PhaseDial) PhaseDial {
	var sumSq float64
	for _, value := range dial {
		re, im := real(value), imag(value)
		sumSq += re*re + im*im
	}

	if sumSq == 0 {
		return dial
	}

	inv := core.Unit / math.Sqrt(sumSq)
	for index := range dial {
		dial[index] = complex(real(dial[index])*inv, imag(dial[index])*inv)
	}

	return dial
}

func copyAndNormalize(dial PhaseDial) PhaseDial {
	out := make(PhaseDial, len(dial))
	copy(out, dial)
	return normalizeDial(out)
}

func dialOverlap(dial, other PhaseDial) complex128 {
	if len(dial) != len(other) || len(dial) == 0 {
		return 0
	}

	var dot complex128
	var normA, normB float64

	for index := range dial {
		dot += cmplx.Conj(dial[index]) * other[index]
		reA, imA := real(dial[index]), imag(dial[index])
		reB, imB := real(other[index]), imag(other[index])
		normA += reA*reA + imA*imA
		normB += reB*reB + imB*imB
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / complex(math.Sqrt(normA)*math.Sqrt(normB), 0)
}

func validateDial(dial PhaseDial) error {
	if len(dial) == 0 || dialNorm(dial) == 0 {
		return fmt.Errorf("phase dial must contain nonzero amplitude")
	}

	for _, component := range dial {
		if math.IsNaN(real(component)) || math.IsNaN(imag(component)) ||
			math.IsInf(real(component), 0) || math.IsInf(imag(component), 0) {
			return fmt.Errorf("phase dial must contain finite components")
		}
	}

	return nil
}

// PhasePathServer implements PhasePath_Server natively.
type PhasePathServer struct{}

func NewPhasePathServer() *PhasePathServer {
	return &PhasePathServer{}
}

func (s *PhasePathServer) Execute(ctx context.Context, call PhasePath_execute) error {
	samples := call.Args().Samples()
	
	res, err := call.AllocResults()
	if err != nil {
		return err
	}
	
	reading, err := res.NewReading()
	if err != nil {
		return err
	}

	if samples <= 0 {
		return nil
	}

	angles, err := reading.NewAngles(samples)
	if err != nil {
		return err
	}

	for index := 0; index < int(samples); index++ {
		angles.Set(index, 2*math.Pi*float64(index)/float64(samples))
	}

	return nil
}

// NormalizeServer implements Normalize_Server natively.
type NormalizeServer struct{}

func NewNormalizeServer() *NormalizeServer {
	return &NormalizeServer{}
}

func (s *NormalizeServer) Execute(ctx context.Context, call Normalize_execute) error {
	wireIn, err := call.Args().Dial()
	if err != nil {
		return err
	}
	
	dial, err := wireToPhaseDial(wireIn)
	if err != nil {
		return err
	}
	
	normalized := normalizeDial(dial)
	
	res, err := call.AllocResults()
	if err != nil {
		return err
	}
	
	wireOut, err := res.NewDial()
	if err != nil {
		return err
	}
	
	outList, err := wireOut.NewComponents(int32(len(normalized) * 2))
	if err != nil {
		return err
	}
	
	phaseDialToWire(normalized, outList)
	
	return nil
}

// OverlapServer implements Overlap_Server natively.
type OverlapServer struct{}

func NewOverlapServer() *OverlapServer {
	return &OverlapServer{}
}

func (s *OverlapServer) Execute(ctx context.Context, call Overlap_execute) error {
	pair, err := call.Args().Pair()
	if err != nil {
		return err
	}
	
	wireProbe, err := pair.Probe()
	if err != nil {
		return err
	}
	
	wireEntry, err := pair.Entry()
	if err != nil {
		return err
	}
	
	probe, err := wireToPhaseDial(wireProbe)
	if err != nil {
		return err
	}
	
	entry, err := wireToPhaseDial(wireEntry)
	if err != nil {
		return err
	}
	
	overlap := dialOverlap(probe, entry)
	
	res, err := call.AllocResults()
	if err != nil {
		return err
	}
	
	res.SetReal(real(overlap))
	res.SetImag(imag(overlap))
	
	return nil
}

// CorpusEntry is the internal state snapshot.
type CorpusEntry struct {
	Dial    PhaseDial
	Outcome []byte // Cap'n Proto serialized AnyPointer
	At      time.Time
}

// CorpusMatch is a single ranked result from a corpus similarity scan.
type CorpusMatch struct {
	Outcome    []byte
	At         time.Time
	Similarity float64
}

// CorpusServer implements Corpus_Server natively.
type CorpusServer struct {
	mu         sync.RWMutex
	entries    []CorpusEntry
	dimensions int
	next       int
	capSize    int
}

func NewCorpusServer(maxSize int) *CorpusServer {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &CorpusServer{
		entries: make([]CorpusEntry, 0, maxSize),
		capSize: maxSize,
	}
}

func (s *CorpusServer) Execute(ctx context.Context, call Corpus_execute) error {
	command, err := call.Args().Command()
	if err != nil {
		return err
	}
	
	res, err := call.AllocResults()
	if err != nil {
		return err
	}
	
	switch command.Which() {
	case WireCorpusCommand_Which_insert:
		insertCmd, err := command.Insert()
		if err != nil {
			return err
		}
		
		wireDial, err := insertCmd.Dial()
		if err != nil {
			return err
		}
		
		dial, err := wireToPhaseDial(wireDial)
		if err != nil {
			return err
		}
		
		if err := validateDial(dial); err != nil {
			return err
		}
		dial = copyAndNormalize(dial)
		
		outcomePtr, err := insertCmd.Outcome()
		if err != nil {
			return err
		}
		
		var outcomeBytes []byte
		if outcomePtr.IsValid() {
			msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
			if err == nil {
				if msg.SetRoot(outcomePtr) == nil {
					outcomeBytes, _ = seg.Message().Marshal()
				}
			}
		}
		
		resultOut, err := res.NewResult()
		if err != nil {
			return err
		}
		
		s.mu.Lock()
		if s.dimensions == 0 {
			s.dimensions = len(dial)
		}
		
		if len(dial) == s.dimensions {
			entry := CorpusEntry{
				Dial:    dial,
				Outcome: outcomeBytes,
				At:      time.Unix(0, insertCmd.At()),
			}
			if len(s.entries) < s.capSize {
				s.entries = append(s.entries, entry)
			} else {
				s.entries[s.next] = entry
				s.next = (s.next + 1) % s.capSize
			}
			resultOut.SetInserted(true)
		}
		s.mu.Unlock()
		
	case WireCorpusCommand_Which_query:
		queryCmd, err := command.Query()
		if err != nil {
			return err
		}
		
		wireDial, err := queryCmd.Dial()
		if err != nil {
			return err
		}
		
		dial, err := wireToPhaseDial(wireDial)
		if err != nil {
			return err
		}
		
		if err := validateDial(dial); err != nil {
			return err
		}
		
		anglesList, err := queryCmd.Angles()
		if err != nil || queryCmd.TopK() <= 0 || anglesList.Len() == 0 {
			return nil
		}
		
		var angles []float64
		for i := 0; i < anglesList.Len(); i++ {
			ang := anglesList.At(i)
			if math.IsNaN(ang) || math.IsInf(ang, 0) {
				return nil
			}
			angles = append(angles, ang)
		}
		
		excludedList, err := queryCmd.ExcludeTimes()
		excluded := make(map[int64]bool)
		if err == nil {
			for i := 0; i < excludedList.Len(); i++ {
				excluded[excludedList.At(i)] = true
			}
		}
		
		s.mu.RLock()
		if s.dimensions != 0 && len(dial) != s.dimensions {
			s.mu.RUnlock()
			return nil
		}
		
		evalEntries := make([]CorpusEntry, 0, len(s.entries))
		overlaps := make([]complex128, 0, len(s.entries))
		for _, entry := range s.entries {
			if excluded[entry.At.UnixNano()] {
				continue
			}
			evalEntries = append(evalEntries, entry)
			overlaps = append(overlaps, dialOverlap(dial, entry.Dial))
		}
		s.mu.RUnlock()
		
		resultOut, err := res.NewResult()
		if err != nil {
			return err
		}
		
		scanList, err := resultOut.NewScan(int32(len(angles)))
		if err != nil {
			return err
		}
		
		matches := make([]CorpusMatch, len(evalEntries))
		
		for angleIndex, angle := range angles {
			rotation := cmplx.Rect(1, -angle)
			for entryIndex, entry := range evalEntries {
				matches[entryIndex] = CorpusMatch{
					Outcome:    entry.Outcome,
					At:         entry.At,
					Similarity: real(overlaps[entryIndex] * rotation),
				}
			}
			rankMatches(matches)
			limit := min(int(queryCmd.TopK()), len(matches))
			
			matchList := scanList.At(angleIndex)
			resMatches, err := matchList.NewMatches(int32(limit))
			if err == nil {
					for i := 0; i < limit; i++ {
						m := matches[i]
						resMatch := resMatches.At(i)
						resMatch.SetSimilarity(m.Similarity)
						resMatch.SetAt(m.At.UnixNano())
						
						if len(m.Outcome) > 0 {
							msg, err := capnp.Unmarshal(m.Outcome)
							if err == nil {
								root, err := msg.Root()
								if err == nil {
									resMatch.SetOutcome(root)
								}
							}
						}
					}
				}
		}
	case WireCorpusCommand_Which_count:
		s.mu.RLock()
		sz := len(s.entries)
		s.mu.RUnlock()
		resultOut, err := res.NewResult()
		if err == nil {
			resultOut.SetSize(int32(sz))
		}
	}
	
	return nil
}

func rankMatches(matches []CorpusMatch) {
	sort.Slice(matches, func(left, right int) bool {
		if matches[left].Similarity != matches[right].Similarity {
			return matches[left].Similarity > matches[right].Similarity
		}
		return matches[left].At.Before(matches[right].At)
	})
}
