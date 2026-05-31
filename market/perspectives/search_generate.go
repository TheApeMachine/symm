package perspectives

import (
	"math"
	"math/rand"
	"time"
)

const maxGeneratedPlaybooks = 5

type branchSection uint8

const (
	branchSectionEntry branchSection = iota
	branchSectionDeny
	branchSectionExit
)

var searchPlaybookNames = []string{
	string(PlaybookTrend),
	string(PlaybookDrive),
	string(PlaybookLeadLag),
	string(PlaybookScarcity),
	string(PlaybookPump),
}

var searchRegimes = []string{"none", "dead", "choppy", "trending", "bullish", "bearish"}

var searchPolicies = []string{"standard", "drive", "pump"}

var searchCategoryConditions = []string{">", ">=", "<", "<="}

/*
GenerateDocument builds a valid candidate document from replay-observed
primitives without using playbook templates or a fixed tree shape.
*/
func GenerateDocument(profile SearchProfile, random *rand.Rand) Document {
	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	playbookCount := 1 + random.Intn(maxGeneratedPlaybooks)
	names := append([]string(nil), searchPlaybookNames...)
	random.Shuffle(len(names), func(leftIndex int, rightIndex int) {
		names[leftIndex], names[rightIndex] = names[rightIndex], names[leftIndex]
	})

	document := Document{
		Version:   1,
		Playbooks: make([]PlaybookSpec, 0, playbookCount),
	}

	for index := range playbookCount {
		document.Playbooks = append(
			document.Playbooks,
			randomPlaybookSpec(names[index], profile, random),
		)
	}

	return normalizeSearchDocument(document, profile, random)
}

func randomPlaybookSpec(
	name string,
	profile SearchProfile,
	random *rand.Rand,
) PlaybookSpec {
	return PlaybookSpec{
		Name:   name,
		Regime: searchRegimes[random.Intn(len(searchRegimes))],
		Policy: searchPolicies[random.Intn(len(searchPolicies))],
		Deny:   randomBranchList(branchSectionDeny, profile, random),
		Entry:  randomBranchList(branchSectionEntry, profile, random),
		Exit:   randomBranchList(branchSectionExit, profile, random),
	}
}

func randomBranchList(
	section branchSection,
	profile SearchProfile,
	random *rand.Rand,
) []BranchSpec {
	branchCount := 1 + random.Intn(4)

	if section == branchSectionDeny {
		branchCount = random.Intn(4)
	}

	branches := make([]BranchSpec, 0, branchCount)

	for range branchCount {
		branches = append(branches, randomRoute(section, profile, random))
	}

	return branches
}

func randomRoute(
	section branchSection,
	profile SearchProfile,
	random *rand.Rand,
) BranchSpec {
	if random.Float64() < 0.18 {
		return randomAnyRoute(section, profile, random)
	}

	if random.Float64() < 0.28 {
		return randomAllRoute(section, profile, random)
	}

	return randomBranchChain(section, profile, random)
}

func randomAnyRoute(
	section branchSection,
	profile SearchProfile,
	random *rand.Rand,
) BranchSpec {
	branchCount := 2 + random.Intn(3)
	branches := make([]BranchSpec, 0, branchCount)

	for range branchCount {
		branches = append(branches, randomBranchChain(section, profile, random))
	}

	return BranchSpec{Any: branches}
}

func randomAllRoute(
	section branchSection,
	profile SearchProfile,
	random *rand.Rand,
) BranchSpec {
	depth := 2 + random.Intn(3)
	branches := make([]BranchSpec, 0, depth)

	for index := range depth {
		branch := randomPrimitiveBranch(section, profile, random)

		if index == depth-1 {
			branch.Action = randomAction(section, random)
		}

		branches = append(branches, branch)
	}

	return BranchSpec{All: branches}
}

func randomBranchChain(
	section branchSection,
	profile SearchProfile,
	random *rand.Rand,
) BranchSpec {
	depth := 1 + random.Intn(4)
	root := randomPrimitiveBranch(section, profile, random)
	current := &root

	for level := 1; level < depth; level++ {
		child := randomPrimitiveBranch(section, profile, random)
		current.Branches = []BranchSpec{child}
		current = &current.Branches[0]
	}

	current.Action = randomAction(section, random)

	return root
}

func randomPrimitiveBranch(
	section branchSection,
	profile SearchProfile,
	random *rand.Rand,
) BranchSpec {
	if section == branchSectionEntry && random.Float64() < 0.12 {
		return BranchSpec{
			Metric:    MetricInPlay,
			Condition: ">=",
			Value:     floatPtr(1),
		}
	}

	category := sampleCategory(profile.searchCategories(), random)

	return categorySpec(category, randomCategoryCondition(random), random)
}

func randomCategoryCondition(random *rand.Rand) string {
	return searchCategoryConditions[random.Intn(len(searchCategoryConditions))]
}

func randomAction(section branchSection, random *rand.Rand) string {
	switch section {
	case branchSectionDeny:
		if random.Float64() < 0.25 {
			return ActionLabel(ActionWait)
		}

		return ActionLabel(ActionDeny)
	case branchSectionExit:
		if random.Float64() < 0.50 {
			return ActionLabel(ActionTakeProfit)
		}

		if random.Float64() < 0.10 {
			return ActionLabel(ActionShort)
		}

		return ActionLabel(ActionStopLoss)
	default:
		if random.Float64() < 0.08 {
			return ActionLabel(ActionWait)
		}

		if random.Float64() < 0.08 {
			return ActionLabel(ActionDeny)
		}

		return ActionLabel(ActionEnter)
	}
}

func (profile SearchProfile) searchCategories() []CategoryStat {
	categories := make([]CategoryStat, 0, len(profile.Categories))

	for _, category := range profile.Categories {
		if category.Name == "" || category.Count == 0 || category.MaxSNR <= 0 {
			continue
		}

		categories = append(categories, category)
	}

	return categories
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

func categorySpec(
	category CategoryStat,
	condition string,
	random *rand.Rand,
) BranchSpec {
	return BranchSpec{
		Category:  category.Name,
		Condition: condition,
		Value:     floatPtr(category.threshold(random)),
	}
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
