package cognition

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type point struct {
	x int
	y int
}

type pairEvidence struct {
	sympathy  float64
	magnitude float64
}

/*
RemapperServer is the spatial remapper: it reorganises metric cells on a finite 2D
grid so they cluster sympathetically, gates on deterministic strict-descent settling,
extracts watershed authority basins, and derives hot-region tokens via Otsu thresholding.
*/
type RemapperServer struct {
	*runtime.System
	settled     bool
	revision    int64
	vocabulary  string
	tokens      []string
	out         []byte
	positions   []point
	prevSettled []point
	metricIDs   []string
	cursor      []byte
	gridWidth    int
	gridHeight   int
	lastEvidence []byte
}

func NewRemapper() *RemapperServer {
	server := &RemapperServer{
		System: runtime.NewSystem(context.Background(), "cognition.remapper"),
	}

	server.Transition(runtime.READY)
	return server
}

func (server *RemapperServer) Write(ctx context.Context, call Remapper_write) error {
	args := call.Args()
	server.tokens = nil
	server.out = nil

	if args.Reset() {
		server.positions = nil
		server.prevSettled = nil
		server.revision = 0
		server.vocabulary = ""
		server.cursor = nil
		server.lastEvidence = nil
		server.settled = false
	}

	cursorBytes, _ := args.Cursor()
	if len(cursorBytes) > 0 {
		server.cursor = bytes.Clone(cursorBytes)
	}

	idsList, err := args.Ids()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.remapper: failed to read IDs",
			err,
		))
	}

	totalMetrics := idsList.Len()
	if totalMetrics == 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.remapper: metric IDs cannot be empty",
			nil,
		))
	}

	metricNames := make([]string, totalMetrics)
	for index := range totalMetrics {
		metricID, err := idsList.At(index)
		if err != nil || metricID == "" {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"cognition.remapper: metric ID cannot be missing or empty",
				err,
			))
		}
		metricNames[index] = metricID
	}
	server.metricIDs = metricNames

	activationsList, err := args.Activations()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.remapper: failed to read activations",
			err,
		))
	}

	if activationsList.Len() != totalMetrics {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.remapper: activations length does not match metric IDs length",
			nil,
		))
	}

	activations := make([]float64, totalMetrics)
	for index := range totalMetrics {
		activations[index] = activationsList.At(index)
	}

	authoritiesList, err := args.Authorities()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.remapper: failed to read authorities",
			err,
		))
	}

	if authoritiesList.Len() != totalMetrics {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.remapper: authorities length does not match metric IDs length",
			nil,
		))
	}

	authorities := make([]float64, totalMetrics)
	for index := range totalMetrics {
		authorities[index] = authoritiesList.At(index)
	}

	observedList, _ := args.Observed()
	var observed []bool
	if observedList.IsValid() && observedList.Len() == totalMetrics {
		observed = make([]bool, totalMetrics)
		for index := range totalMetrics {
			observed[index] = observedList.At(index)
		}
	}

	evidenceBytes, err := args.Evidence()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"cognition.remapper: failed to read evidence data",
			err,
		))
	}

	evidenceMap := make(map[string]pairEvidence)
	if len(evidenceBytes) > 0 {
		var decoded map[string]any
		if err := sonic.Unmarshal(evidenceBytes, &decoded); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"cognition.remapper: invalid evidence JSON",
				err,
			))
		}
		for key, val := range decoded {
			if obj, ok := val.(map[string]any); ok {
				if evObj, hasEv := obj["evidence"].(map[string]any); hasEv {
					symp, _ := evObj["sympathy"].(float64)
					mag, _ := evObj["magnitude"].(float64)
					evidenceMap[key] = pairEvidence{sympathy: symp, magnitude: mag}
				} else {
					symp, _ := obj["sympathy"].(float64)
					mag, _ := obj["magnitude"].(float64)
					evidenceMap[key] = pairEvidence{sympathy: symp, magnitude: mag}
				}
			}
		}
	}

	evidenceChanged := !bytes.Equal(server.lastEvidence, evidenceBytes)
	if evidenceChanged {
		server.lastEvidence = bytes.Clone(evidenceBytes)
		server.settled = false
	}

	// Derive dynamic grid geometry from total metric count
	gridWidth := int(math.Ceil(math.Sqrt(float64(totalMetrics))))
	if gridWidth < 1 {
		gridWidth = 1
	}
	gridHeight := int(math.Ceil(float64(totalMetrics) / float64(gridWidth)))
	if gridHeight < 1 {
		gridHeight = 1
	}
	server.gridWidth = gridWidth
	server.gridHeight = gridHeight

	// Initial layout setup if uninitialized or size changed
	if len(server.positions) != totalMetrics {
		server.positions = make([]point, totalMetrics)
		server.prevSettled = make([]point, totalMetrics)

		for index := range totalMetrics {
			cellPoint := point{
				x: index % server.gridWidth,
				y: index / server.gridWidth,
			}
			server.positions[index] = cellPoint
			server.prevSettled[index] = cellPoint
		}
	}

	// Strict settling gate: without evidence, layout structurally cannot settle
	if len(evidenceMap) == 0 {
		server.settled = false
	} else if !server.settled {
		maxNormDistSq := float64(server.gridWidth*server.gridWidth + server.gridHeight*server.gridHeight)
		improved := server.solvePermutation(evidenceMap, authorities, maxNormDistSq)

		if !improved {
			server.settled = true
			server.revision++

			for index := range totalMetrics {
				server.prevSettled[index] = server.positions[index]
			}
		}
	}

	// Watershed regions and hot-region token generation
	basins, basinMembers := server.extractBasins(authorities)
	vocabHash := server.computeVocabularyHash(basinMembers)
	server.vocabulary = vocabHash

	var hotTokens []string
	if server.settled {
		hotTokens = server.selectHotTokens(basins, basinMembers, activations, authorities, observed)
		if len(hotTokens) > 0 {
			fmt.Printf("REMAPPER WRITE: settled=true, hotTokens=%v, totalBasins=%d\n", hotTokens, len(basinMembers))
		}
	}
	server.tokens = hotTokens

	regionsPayload := make([]map[string]any, len(basinMembers))
	for index, members := range basinMembers {
		regionsPayload[index] = map[string]any{
			"id":      fmt.Sprintf("R%d", index+1),
			"members": members,
		}
	}

	outPayload := map[string]any{
		"settled":    server.settled,
		"revision":   server.revision,
		"vocabulary": server.vocabulary,
		"tokens":     server.tokens,
		"regions":    regionsPayload,
	}

	if len(server.cursor) > 0 {
		var decodedCursor any
		if err := sonic.Unmarshal(server.cursor, &decodedCursor); err == nil {
			outPayload["cursor"] = decodedCursor
		}
	}

	encodedOut, err := sonic.Marshal(outPayload)
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.remapper: failed to encode out payload",
			err,
		))
	}

	server.out = encodedOut
	return nil
}

func (server *RemapperServer) Done(ctx context.Context, call Remapper_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.remapper: failed to allocate results",
			err,
		))
	}

	results.SetSettled(server.settled)
	results.SetRevision(server.revision)

	if err := results.SetVocabulary(server.vocabulary); err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.remapper: failed to set vocabulary",
			err,
		))
	}

	tokensList, err := results.NewTokens(int32(len(server.tokens)))
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"cognition.remapper: failed to allocate tokens list",
			err,
		))
	}

	for index, token := range server.tokens {
		if err := tokensList.Set(index, token); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"cognition.remapper: failed to set token",
				err,
			))
		}
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"cognition.remapper: failed to set out",
				err,
			))
		}
	}

	return nil
}

func (server *RemapperServer) solvePermutation(
	evidence map[string]pairEvidence,
	authorities []float64,
	maxDistSq float64,
) bool {
	totalMetrics := len(server.positions)
	if totalMetrics < 2 || len(evidence) == 0 {
		return false
	}

	idMap := make(map[string]int, totalMetrics)
	for i, id := range server.metricIDs {
		idMap[id] = i
	}

	// Dense pair cost matrix for O(1) lookup
	pairCosts := make([]float64, totalMetrics*totalMetrics)
	hasNonZero := false
	for k, ev := range evidence {
		cost := ev.sympathy
		if cost == 0 {
			continue
		}
		parts := strings.Split(k, ":")
		if len(parts) == 2 {
			i1, ok1 := idMap[parts[0]]
			i2, ok2 := idMap[parts[1]]
			if ok1 && ok2 && i1 != i2 {
				pairCosts[i1*totalMetrics+i2] += cost
				pairCosts[i2*totalMetrics+i1] += cost
				hasNonZero = true
			}
		}
	}

	if !hasNonZero {
		return false
	}

	distSq := func(p1, p2 point) float64 {
		dx := float64(p1.x - p2.x)
		dy := float64(p1.y - p2.y)
		return dx*dx + dy*dy
	}

	anySwapAccepted := false

	// Single strict-descent sweep over canonical candidate pairs
	for first := 0; first < totalMetrics; first++ {
		firstPos := server.positions[first]
		for second := first + 1; second < totalMetrics; second++ {
			secondPos := server.positions[second]

			// Calculate delta energy for swapping first and second
			// 1. Displacement delta from prevSettled
			var delta float64
			if first < len(server.prevSettled) && second < len(server.prevSettled) {
				prevFirst := server.prevSettled[first]
				prevSecond := server.prevSettled[second]

				oldDispFirst := distSq(firstPos, prevFirst)
				newDispFirst := distSq(secondPos, prevFirst)
				oldDispSecond := distSq(secondPos, prevSecond)
				newDispSecond := distSq(firstPos, prevSecond)

				delta += (authorities[first]*(newDispFirst-oldDispFirst) + authorities[second]*(newDispSecond-oldDispSecond)) / maxDistSq
			}

			// 2. Interaction delta with other metrics k
			rowFirst := first * totalMetrics
			rowSecond := second * totalMetrics

			for k := 0; k < totalMetrics; k++ {
				if k == first || k == second {
					continue
				}
				costDiff := pairCosts[rowFirst+k] - pairCosts[rowSecond+k]
				if costDiff == 0 {
					continue
				}
				kPos := server.positions[k]
				distDiff := (distSq(secondPos, kPos) - distSq(firstPos, kPos)) / maxDistSq
				delta += costDiff * distDiff
			}

			// Strict improvement threshold: strict descent delta < 0
			if delta < 0 {
				server.positions[first], server.positions[second] = secondPos, firstPos
				firstPos = server.positions[first]
				anySwapAccepted = true
			}
		}
	}

	return anySwapAccepted
}

func (server *RemapperServer) extractBasins(authorities []float64) ([]int, [][]string) {
	totalMetrics := len(server.positions)
	if totalMetrics == 0 {
		return nil, nil
	}

	// Construct grid of authority heights
	grid := make([][]float64, server.gridHeight)
	for y := range server.gridHeight {
		grid[y] = make([]float64, server.gridWidth)
	}

	metricAtCell := make(map[point]int)
	for index, pos := range server.positions {
		grid[pos.y][pos.x] = authorities[index]
		metricAtCell[pos] = index
	}

	// Find steepest ascent neighbor for each cell to identify watershed basins
	parent := make([]int, totalMetrics)
	for index := range totalMetrics {
		parent[index] = index
	}

	var findRoot func(int) int
	findRoot = func(index int) int {
		if parent[index] != index {
			parent[index] = findRoot(parent[index])
		}
		return parent[index]
	}

	dxs := []int{0, 1, 0, -1}
	dys := []int{-1, 0, 1, 0}

	for index, pos := range server.positions {
		maxHeight := grid[pos.y][pos.x]
		bestNeighbor := -1

		for neighborIdx := 0; neighborIdx < 4; neighborIdx++ {
			nx := pos.x + dxs[neighborIdx]
			ny := pos.y + dys[neighborIdx]

			if nx < 0 || nx >= server.gridWidth || ny < 0 || ny >= server.gridHeight {
				continue
			}

			targetMetric, exists := metricAtCell[point{x: nx, y: ny}]
			if !exists {
				continue
			}

			neighborHeight := grid[ny][nx]
			if neighborHeight > maxHeight {
				maxHeight = neighborHeight
				bestNeighbor = targetMetric
			}
		}

		if bestNeighbor != -1 {
			rootCurrent := findRoot(index)
			rootTarget := findRoot(bestNeighbor)
			if rootCurrent != rootTarget {
				parent[rootCurrent] = rootTarget
			}
		}
	}

	basinMap := make(map[int][]string)
	basinIndexMap := make(map[int]int)
	basins := make([]int, totalMetrics)

	for index := range totalMetrics {
		root := findRoot(index)
		basins[index] = root
		basinMap[root] = append(basinMap[root], server.metricIDs[index])
	}

	sortedRoots := make([]int, 0, len(basinMap))
	for root := range basinMap {
		sortedRoots = append(sortedRoots, root)
	}
	sort.Ints(sortedRoots)

	basinMembers := make([][]string, len(sortedRoots))
	for idx, root := range sortedRoots {
		basinIndexMap[root] = idx
		members := basinMap[root]
		sort.Strings(members)
		basinMembers[idx] = members
	}

	for index := range totalMetrics {
		basins[index] = basinIndexMap[findRoot(index)]
	}

	return basins, basinMembers
}

func (server *RemapperServer) computeVocabularyHash(basinMembers [][]string) string {
	hasher := sha256.New()
	for idx, members := range basinMembers {
		hasher.Write([]byte(fmt.Sprintf("b%d:", idx)))
		for _, member := range members {
			hasher.Write([]byte(member + ","))
		}
	}
	return hex.EncodeToString(hasher.Sum(nil))[:16]
}

func (server *RemapperServer) selectHotTokens(
	basins []int,
	basinMembers [][]string,
	activations []float64,
	authorities []float64,
	observed []bool,
) []string {
	totalBasins := len(basinMembers)
	if totalBasins == 0 {
		return nil
	}

	basinActivations := make([]float64, totalBasins)
	hasNonZero := false

	nonZeroAct := 0
	nonZeroObs := 0
	for i := range activations {
		if activations[i] != 0 {
			nonZeroAct++
		}
		if len(observed) > i && observed[i] {
			nonZeroObs++
			if i < len(server.metricIDs) {
				fmt.Printf("OBSERVED METRIC [%d] %s = %v\n", i, server.metricIDs[i], activations[i])
			}
		}
	}
	fmt.Printf("SELECT_HOT_TOKENS: total=%d, nonZeroAct=%d, nonZeroObs=%d, lenObs=%d, auth0=%f\n", len(activations), nonZeroAct, nonZeroObs, len(observed), authorities[0])

	for index, basinIdx := range basins {
		if basinIdx >= 0 && basinIdx < totalBasins {
			if len(observed) == len(activations) && !observed[index] {
				continue
			}
			energy := math.Abs(activations[index]) * authorities[index]
			basinActivations[basinIdx] += energy
			if energy > 0 {
				hasNonZero = true
			}
		}
	}

	if !hasNonZero {
		return nil
	}

	// Otsu thresholding on basin activations
	threshold := server.otsuThreshold(basinActivations)
	fmt.Printf("REMAPPER OTSU: threshold=%f, basinActivations=%v\n", threshold, basinActivations)

	var hotTokens []string
	for idx, activation := range basinActivations {
		if activation >= threshold && activation > 0 {
			hotTokens = append(hotTokens, fmt.Sprintf("R%d", idx+1))
		}
	}

	sort.Strings(hotTokens)
	return hotTokens
}

func (server *RemapperServer) otsuThreshold(values []float64) float64 {
	total := len(values)
	if total <= 1 {
		if total == 1 {
			return values[0]
		}
		return 0
	}

	sorted := make([]float64, total)
	copy(sorted, values)
	sort.Float64s(sorted)

	sumAll := 0.0
	for _, val := range sorted {
		sumAll += val
	}

	bestVariance := -1.0
	bestThreshold := sorted[0]

	weightBackground := 0.0
	sumBackground := 0.0

	for index := 0; index < total-1; index++ {
		weightBackground += 1.0
		weightForeground := float64(total) - weightBackground

		val := sorted[index]
		sumBackground += val
		sumForeground := sumAll - sumBackground

		meanBackground := sumBackground / weightBackground
		meanForeground := sumForeground / weightForeground

		meanDiff := meanBackground - meanForeground
		betweenClassVariance := weightBackground * weightForeground * meanDiff * meanDiff

		if betweenClassVariance > bestVariance {
			bestVariance = betweenClassVariance
			bestThreshold = (val + sorted[index+1]) / 2.0
		}
	}

	return bestThreshold
}
