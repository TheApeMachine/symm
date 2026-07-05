package dmt

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const snapshotName = "cognitive-tree.json"

var (
	errEmptySequence = errors.New("dmt: empty sequence")
)

/*
Tree stores typed cognitive prefix and basin weights.
*/
type Tree struct {
	mu          sync.RWMutex
	sensory     map[string]CognitiveState
	basins      map[string]map[string]CognitiveState
	persistPath string
	err         error
}

type snapshot struct {
	Sensory map[string]CognitiveState            `json:"sensory"`
	Basins  map[string]map[string]CognitiveState `json:"basins"`
}

/*
NewTree creates a local cognitive tree, optionally backed by a JSON snapshot.
*/
func NewTree(persistDir string) *Tree {
	tree := &Tree{
		sensory: make(map[string]CognitiveState),
		basins:  make(map[string]map[string]CognitiveState),
	}

	if strings.TrimSpace(persistDir) == "" {
		return tree
	}

	tree.persistPath = filepath.Join(persistDir, snapshotName)
	tree.load()

	return tree
}

/*
Error returns the last persistence error observed by the tree.
*/
func (tree *Tree) Error() error {
	if tree == nil {
		return nil
	}

	tree.mu.RLock()
	defer tree.mu.RUnlock()

	return tree.err
}

/*
GetSensoryWeight reads learned state for an exact sensory prefix.
*/
func (tree *Tree) GetSensoryWeight(sequence []byte) CognitiveState {
	if tree == nil {
		return CognitiveState{}
	}

	tree.mu.RLock()
	defer tree.mu.RUnlock()

	return tree.sensory[string(sequence)]
}

/*
TrainSensorySequence reinforces every prefix in a sensory sequence.
*/
func (tree *Tree) TrainSensorySequence(sequence []byte) {
	if tree == nil || len(sequence) == 0 {
		return
	}

	tokens := splitSequence(sequence)
	if len(tokens) == 0 {
		return
	}

	tree.mu.Lock()
	defer tree.mu.Unlock()

	for end := 1; end <= len(tokens); end++ {
		prefix := joinTokens(tokens[:end])
		state := tree.sensory[prefix]
		state.Count++
		tree.sensory[prefix] = state
	}

	tree.recomputeSensoryProbabilities()
	tree.save()
}

/*
GetAttractorBasin reads posterior support for one class and sequence.
*/
func (tree *Tree) GetAttractorBasin(class []byte, sequence []byte) CognitiveState {
	if tree == nil {
		return CognitiveState{}
	}

	tree.mu.RLock()
	defer tree.mu.RUnlock()

	classBasins := tree.basins[string(class)]
	if classBasins == nil {
		return CognitiveState{}
	}

	return classBasins[string(sequence)]
}

/*
InsertAttractorBasin writes posterior support for one class and sequence.
*/
func (tree *Tree) InsertAttractorBasin(
	class []byte,
	sequence []byte,
	state CognitiveState,
) (*Tree, bool, error) {
	if tree == nil {
		return tree, false, nil
	}

	if len(class) == 0 || len(sequence) == 0 {
		return tree, false, errEmptySequence
	}

	tree.mu.Lock()
	defer tree.mu.Unlock()

	className := string(class)
	sequenceKey := string(sequence)
	if tree.basins[className] == nil {
		tree.basins[className] = make(map[string]CognitiveState)
	}

	previous, existed := tree.basins[className][sequenceKey]
	tree.basins[className][sequenceKey] = state
	tree.save()

	return tree, !existed || previous != state, tree.err
}

func (tree *Tree) recomputeSensoryProbabilities() {
	parentTotals := make(map[string]uint64)

	for sequence, state := range tree.sensory {
		parentTotals[parentPrefix(sequence)] += state.Count
	}

	for sequence, state := range tree.sensory {
		total := parentTotals[parentPrefix(sequence)]
		state.Probability = probability(state.Count, total)
		tree.sensory[sequence] = state
	}
}

func probability(count uint64, total uint64) float64 {
	if count == 0 || total == 0 {
		return 0
	}

	return float64(count) / float64(total)
}

func (tree *Tree) load() {
	raw, err := os.ReadFile(tree.persistPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}

	if err != nil {
		tree.err = err
		return
	}

	decoded := snapshot{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		tree.err = err
		return
	}

	if decoded.Sensory != nil {
		tree.sensory = decoded.Sensory
	}

	if decoded.Basins != nil {
		tree.basins = decoded.Basins
	}
}

func (tree *Tree) save() {
	if tree.persistPath == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(tree.persistPath), 0o755); err != nil {
		tree.err = err
		return
	}

	raw, err := json.Marshal(snapshot{
		Sensory: tree.sensory,
		Basins:  tree.basins,
	})
	if err != nil {
		tree.err = err
		return
	}

	tempPath := tree.persistPath + ".tmp"
	if err := os.WriteFile(tempPath, raw, 0o644); err != nil {
		tree.err = err
		return
	}

	if err := os.Rename(tempPath, tree.persistPath); err != nil {
		tree.err = err
		return
	}

	tree.err = nil
}

func splitSequence(sequence []byte) []string {
	parts := strings.Split(string(sequence), "_")
	tokens := make([]string, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue
		}

		tokens = append(tokens, part)
	}

	return tokens
}

func joinTokens(tokens []string) string {
	return strings.Join(tokens, "_")
}

func parentPrefix(sequence string) string {
	separator := strings.LastIndexByte(sequence, '_')
	if separator < 0 {
		return ""
	}

	return sequence[:separator]
}

func appendToken(buffer []byte, prefix []byte, token []byte) []byte {
	if len(prefix) > 0 {
		buffer = append(buffer, prefix...)
		buffer = append(buffer, '_')
	}

	return append(buffer, token...)
}

func logScore(probability float64) float64 {
	if probability <= 0 {
		return math.Inf(-1)
	}

	return math.Log(probability)
}

func sortBeams(beams []BeamPath) {
	sort.SliceStable(beams, func(left, right int) bool {
		if beams[left].Score == beams[right].Score {
			return string(beams[left].Sequence) < string(beams[right].Sequence)
		}

		return beams[left].Score > beams[right].Score
	})
}
