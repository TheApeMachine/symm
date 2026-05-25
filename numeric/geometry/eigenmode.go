package geometry

/*
Eigenmode represents a cluster of field participants whose affinity
vectors are mutually coupled above a threshold. The members share
structural resonance — they respond to similar input topology. Energy
is the aggregate surprisal mass of the mode, used to rank dominance.
*/
type Eigenmode struct {
	members []uint64
	energy  float64
}

/*
Members returns the participant IDs in this mode.
*/
func (mode *Eigenmode) Members() []uint64 {
	return append([]uint64(nil), mode.members...)
}

/*
Energy returns the aggregate energy score.
*/
func (mode *Eigenmode) Energy() float64 {
	return mode.energy
}

/*
ModeParticipant carries the minimum signals needed for eigenmode
detection: an origin ID and a scalar energy contribution.
*/
type ModeParticipant struct {
	Origin uint64
	Energy float64
}

/*
eigenSnap stores the last completed eigenmode partition for a community field.
It is swapped in by a background pass so cycleLeaf can read slightly stale
modes without blocking on DetectModes.
*/
type EigenSnap struct {
	modes       []Eigenmode
	dominantIdx int
}

/*
NewEigenSnap wraps a (modes, dominantIdx) pair — typically the direct
return from DetectModes — into a snap so callers can hand the whole
partition around by a single pointer and swap it atomically when a new
one lands.
*/
func NewEigenSnap(modes []Eigenmode, dominantIdx int) *EigenSnap {
	return &EigenSnap{
		modes:       modes,
		dominantIdx: dominantIdx,
	}
}

/*
Modes returns a defensive copy of the eigenmode partition held by this snap.
Callers cannot mutate the snap's internal state through the returned slice.
*/
func (snap *EigenSnap) Modes() []Eigenmode {
	if snap == nil {
		return nil
	}

	out := make([]Eigenmode, len(snap.modes))

	for i := range snap.modes {
		m := &snap.modes[i]
		out[i] = Eigenmode{
			members: m.Members(),
			energy:  m.Energy(),
		}
	}

	return out
}

/*
DominantIdx returns the index of the highest-energy mode, or -1 when
the partition is empty.
*/
func (snap *EigenSnap) DominantIdx() int {
	if snap == nil || len(snap.modes) == 0 {
		return -1
	}

	return snap.dominantIdx
}

/*
Dominant returns the dominant Eigenmode directly, or the zero value
when no modes were detected. The ok flag distinguishes a genuine empty
snap from a legitimate zero-energy dominant mode.
*/
func (snap *EigenSnap) Dominant() (mode Eigenmode, ok bool) {
	if snap == nil {
		return Eigenmode{}, false
	}

	if snap.dominantIdx < 0 || snap.dominantIdx >= len(snap.modes) {
		return Eigenmode{}, false
	}

	return snap.modes[snap.dominantIdx], true
}

/*
PhaseMode is the dominant finite-field phase extracted from a vector.
The lane index acts as the phase angle; amplitude and concentration
describe how collapsed the vector is around that lane.
*/
type PhaseMode struct {
	Index         int
	Amplitude     uint32
	Concentration float64
}

/*
DetectModes partitions participants into eigenmodes by greedy affinity
clustering. Two participants belong to the same mode when couplingFn
returns a value at or above the threshold. Returns the modes and the
index of the dominant (highest energy) mode, or -1 if none.

couplingFn receives two origin IDs and returns the coupling strength
between them. This keeps the geometry package free of domain types.
*/
func DetectModes(
	participants []ModeParticipant,
	couplingThreshold float64,
	couplingFn func(a, b uint64) float64,
) ([]Eigenmode, int) {
	assigned := make(map[uint64]bool, len(participants))
	modes := make([]Eigenmode, 0)

	for _, pA := range participants {
		if assigned[pA.Origin] {
			continue
		}

		mode := Eigenmode{
			members: []uint64{pA.Origin},
			energy:  pA.Energy,
		}

		assigned[pA.Origin] = true

		for _, pB := range participants {
			if assigned[pB.Origin] {
				continue
			}

			if couplingFn(pA.Origin, pB.Origin) >= couplingThreshold {
				mode.members = append(mode.members, pB.Origin)
				mode.energy += pB.Energy
				assigned[pB.Origin] = true
			}
		}

		modes = append(modes, mode)
	}

	if len(modes) == 0 {
		return modes, -1
	}

	dominantIdx := 0
	maxEnergy := modes[0].energy

	for idx := 1; idx < len(modes); idx++ {
		if modes[idx].energy > maxEnergy {
			maxEnergy = modes[idx].energy
			dominantIdx = idx
		}
	}

	return modes, dominantIdx
}

func phaseModeFromDominant(dominantPhase PhaseMode) PhaseMode {
	return PhaseMode{
		Index:         dominantPhase.Index,
		Amplitude:     dominantPhase.Amplitude,
		Concentration: dominantPhase.Concentration,
	}
}
