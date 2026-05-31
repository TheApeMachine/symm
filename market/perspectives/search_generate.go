package perspectives

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

type DocumentSearch struct {
	mu         sync.Mutex
	random     *rand.Rand
	profile    SearchProfile
	best       *Document
	bestReward float64
	hasBest    bool
}

func NewDocumentSearch(profile SearchProfile, random *rand.Rand) (*DocumentSearch, error) {
	if err := profile.Validate(); err != nil {
		return nil, err
	}

	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return &DocumentSearch{
		random:  random,
		profile: profile,
	}, nil
}

func (search *DocumentSearch) Next() Document {
	search.mu.Lock()
	defer search.mu.Unlock()

	if search.best != nil && search.random.Float64() < 0.40 {
		return MutateDocument(*search.best, search.random)
	}

	return GenerateDocument(search.profile, search.random)
}

func (search *DocumentSearch) Observe(document Document, reward float64) {
	search.mu.Lock()
	defer search.mu.Unlock()

	if search.hasBest && reward <= search.bestReward {
		return
	}

	clone := cloneDocument(document)
	search.best = &clone
	search.bestReward = reward
	search.hasBest = true
}

func GenerateDocument(profile SearchProfile, random *rand.Rand) Document {
	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	categories := profile.searchCategories()
	playbooks := make([]PlaybookSpec, 0, len(searchPlaybookTemplates))

	for _, template := range searchPlaybookTemplates {
		playbooks = append(playbooks, PlaybookSpec{
			Name:   template.Name,
			Regime: template.Regime,
			Policy: template.Policy,
			Deny:   generateDenySpecs(categories, random),
			Entry:  generateEntrySpecs(categories, random),
			Exit:   generateExitSpecs(categories, random),
		})
	}

	return Document{Version: 1, Playbooks: playbooks}
}

type playbookTemplate struct {
	Name   string
	Regime string
	Policy string
}

var searchPlaybookTemplates = []playbookTemplate{
	{Name: string(PlaybookTrend), Regime: "bullish", Policy: "standard"},
	{Name: string(PlaybookDrive), Regime: "trending", Policy: "drive"},
	{Name: string(PlaybookLeadLag), Regime: "trending", Policy: "standard"},
	{Name: string(PlaybookScarcity), Regime: "choppy", Policy: "standard"},
	{Name: string(PlaybookPump), Regime: "trending", Policy: "pump"},
}

func (profile SearchProfile) searchCategories() []CategoryStat {
	categories := make([]CategoryStat, 0, len(profile.Categories))

	for _, category := range profile.Categories {
		if category.Name == "" || category.Count == 0 || category.MaxSNR <= 0 {
			continue
		}

		categories = append(categories, category)
	}

	if len(categories) > 0 {
		return categories
	}

	return append([]CategoryStat(nil), profile.Categories...)
}

func generateEntrySpecs(categories []CategoryStat, random *rand.Rand) []BranchSpec {
	routeCount := 1 + random.Intn(4)
	routes := make([]BranchSpec, 0, routeCount)
	gateThreshold := quantized(random, 0.70, 2.50, 0.10)

	for routeIndex := 0; routeIndex < routeCount; routeIndex++ {
		routes = append(routes, generateCategoryChain(categories, random, ActionEnter))
	}

	if random.Float64() < 0.25 {
		routes = []BranchSpec{{
			Metric:    MetricInPlay,
			Condition: ">=",
			Value:     floatPtr(1),
			Branches:  routes,
		}}
	}

	return []BranchSpec{
		{
			Metric:    MetricScoreCostRatio,
			Condition: "<",
			Value:     floatPtr(gateThreshold),
			Action:    ActionLabel(ActionDeny),
		},
		{
			Metric:    MetricScoreCostRatio,
			Condition: ">=",
			Value:     floatPtr(gateThreshold),
			Branches:  routes,
		},
	}
}

func generateDenySpecs(categories []CategoryStat, random *rand.Rand) []BranchSpec {
	branchCount := 1 + random.Intn(5)
	branches := make([]BranchSpec, 0, branchCount)

	for branchIndex := 0; branchIndex < branchCount; branchIndex++ {
		action := ActionDeny

		if random.Float64() < 0.15 {
			action = ActionWait
		}

		branches = append(branches, categorySpec(sampleCategory(categories, random), action, random))
	}

	return branches
}

func generateExitSpecs(categories []CategoryStat, random *rand.Rand) []BranchSpec {
	branchCount := 2 + random.Intn(5)
	branches := make([]BranchSpec, 0, branchCount)

	for branchIndex := 0; branchIndex < branchCount; branchIndex++ {
		action := ActionTakeProfit

		if random.Float64() < 0.45 {
			action = ActionStopLoss
		}

		if random.Float64() < 0.05 {
			action = ActionShort
		}

		branches = append(branches, categorySpec(sampleCategory(categories, random), action, random))
	}

	return branches
}

func generateCategoryChain(
	categories []CategoryStat,
	random *rand.Rand,
	action ActionType,
) BranchSpec {
	depth := 1 + random.Intn(4)
	selected := sampleDistinctCategories(categories, random, depth)

	if len(selected) == 0 {
		return BranchSpec{}
	}

	var child *BranchSpec

	for categoryIndex := len(selected) - 1; categoryIndex >= 0; categoryIndex-- {
		branch := categorySpec(selected[categoryIndex], ActionNone, random)

		if child == nil {
			branch.Action = ActionLabel(action)
		} else {
			branch.Branches = []BranchSpec{*child}
		}

		child = &branch
	}

	return *child
}

func sampleDistinctCategories(
	categories []CategoryStat,
	random *rand.Rand,
	count int,
) []CategoryStat {
	selected := make([]CategoryStat, 0, count)
	seen := make(map[string]bool, count)
	attempts := 0

	for len(selected) < count && attempts < count*8 {
		attempts++
		category := sampleCategory(categories, random)

		if category.Name == "" || seen[category.Name] {
			continue
		}

		seen[category.Name] = true
		selected = append(selected, category)
	}

	return selected
}

func sampleCategory(categories []CategoryStat, random *rand.Rand) CategoryStat {
	if len(categories) == 0 {
		return CategoryStat{}
	}

	total := 0.0

	for _, category := range categories {
		total += category.searchWeight()
	}

	target := random.Float64() * total

	for _, category := range categories {
		target -= category.searchWeight()

		if target <= 0 {
			return category
		}
	}

	return categories[len(categories)-1]
}

func categorySpec(category CategoryStat, action ActionType, random *rand.Rand) BranchSpec {
	spec := BranchSpec{
		Category:  category.Name,
		Condition: ">",
		Value:     floatPtr(category.threshold(random)),
	}

	if action != ActionNone {
		spec.Action = ActionLabel(action)
	}

	return spec
}

func (category CategoryStat) searchWeight() float64 {
	countWeight := math.Sqrt(float64(category.Count))
	signalWeight := 0.5 + math.Max(category.MeanSNR, category.P75SNR)

	if signalWeight <= 0 {
		signalWeight = 0.5
	}

	return countWeight * signalWeight
}

func (category CategoryStat) threshold(random *rand.Rand) float64 {
	candidates := []float64{
		category.P50SNR,
		category.P75SNR,
		category.P90SNR,
		category.MeanSNR,
	}
	thresholds := make([]float64, 0, len(candidates))

	for _, candidate := range candidates {
		if candidate <= 0 || candidate > category.MaxSNR {
			continue
		}

		thresholds = append(thresholds, candidate)
	}

	if len(thresholds) == 0 {
		return quantizeValue(category.MaxSNR, 0.05)
	}

	threshold := thresholds[random.Intn(len(thresholds))]

	return quantizeValue(threshold, 0.05)
}

func quantizeValue(value float64, step float64) float64 {
	if step <= 0 {
		return value
	}

	return math.Round(value/step) * step
}
