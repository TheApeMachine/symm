package perspectives

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const maxSearchRolloutSteps = 4

type DocumentSearch struct {
	mu         sync.Mutex
	random     *rand.Rand
	profile    SearchProfile
	root       *documentSearchNode
	pending    map[string][][]*documentSearchNode
	best       *Document
	bestReward float64
	minReward  float64
	maxReward  float64
	hasBest    bool
}

type documentSearchNode struct {
	document Document
	children []*documentSearchNode
	untried  []documentAction
	visits   int
	reward   float64
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
		pending: make(map[string][][]*documentSearchNode),
	}, nil
}

func (search *DocumentSearch) Next() Document {
	search.mu.Lock()
	defer search.mu.Unlock()

	if search.root == nil {
		document := GenerateDocument(search.profile, search.random)
		search.root = newDocumentSearchNode(document, search.profile, search.random)
		search.rememberPending(document, []*documentSearchNode{search.root})

		return document
	}

	node, path := search.selectExpansionPath()
	document := search.rollout(node.document)
	search.rememberPending(document, path)

	return document
}

func (search *DocumentSearch) Observe(document Document, reward float64) {
	search.mu.Lock()
	defer search.mu.Unlock()

	search.observeRewardBounds(reward)
	search.observeBest(document, reward)
	path := search.takePending(document)

	if len(path) == 0 {
		search.observeUntracked(document, reward)

		return
	}

	for _, node := range path {
		node.visits++
		node.reward += reward
	}
}

func (search *DocumentSearch) BestReward() float64 {
	search.mu.Lock()
	defer search.mu.Unlock()

	return search.bestReward
}

func (search *DocumentSearch) selectExpansionPath() (
	*documentSearchNode,
	[]*documentSearchNode,
) {
	node := search.root
	path := []*documentSearchNode{node}

	for len(node.untried) == 0 && len(node.children) > 0 {
		node = search.selectChild(node)
		path = append(path, node)
	}

	if len(node.untried) == 0 {
		return node, path
	}

	actionIndex := search.random.Intn(len(node.untried))
	action := node.untried[actionIndex]
	node.untried = append(node.untried[:actionIndex], node.untried[actionIndex+1:]...)

	childDocument := action.Apply(node.document, search.profile, search.random)
	child := newDocumentSearchNode(childDocument, search.profile, search.random)
	node.children = append(node.children, child)
	path = append(path, child)

	return child, path
}

func (search *DocumentSearch) selectChild(parent *documentSearchNode) *documentSearchNode {
	var selected *documentSearchNode
	selectedScore := math.Inf(-1)

	for _, child := range parent.children {
		if child.visits == 0 {
			return child
		}

		score := search.ucbScore(parent, child)

		if score > selectedScore {
			selected = child
			selectedScore = score
		}
	}

	if selected != nil {
		return selected
	}

	return parent.children[search.random.Intn(len(parent.children))]
}

func (search *DocumentSearch) ucbScore(
	parent *documentSearchNode,
	child *documentSearchNode,
) float64 {
	mean := child.reward / float64(child.visits)
	exploitation := 0.0
	rewardRange := search.maxReward - search.minReward

	if rewardRange > 0 {
		exploitation = (mean - search.minReward) / rewardRange
	}

	exploration := math.Sqrt(2 * math.Log(float64(parent.visits+1)) / float64(child.visits))

	return exploitation + exploration
}

func (search *DocumentSearch) rollout(document Document) Document {
	steps := 1 + search.random.Intn(maxSearchRolloutSteps)
	rolled := cloneDocument(document)

	for range steps {
		actions := searchActions(rolled, search.profile, search.random)

		if len(actions) == 0 {
			return normalizeSearchDocument(rolled, search.profile, search.random)
		}

		action := actions[search.random.Intn(len(actions))]
		rolled = action.Apply(rolled, search.profile, search.random)
	}

	return normalizeSearchDocument(rolled, search.profile, search.random)
}

func (search *DocumentSearch) rememberPending(
	document Document,
	path []*documentSearchNode,
) {
	key := documentSearchKey(document)
	search.pending[key] = append(search.pending[key], path)
}

func (search *DocumentSearch) takePending(document Document) []*documentSearchNode {
	key := documentSearchKey(document)
	paths := search.pending[key]

	if len(paths) == 0 {
		return nil
	}

	path := paths[len(paths)-1]

	if len(paths) == 1 {
		delete(search.pending, key)

		return path
	}

	search.pending[key] = paths[:len(paths)-1]

	return path
}

func (search *DocumentSearch) observeRewardBounds(reward float64) {
	if !search.hasBest {
		search.minReward = reward
		search.maxReward = reward

		return
	}

	if reward < search.minReward {
		search.minReward = reward
	}

	if reward > search.maxReward {
		search.maxReward = reward
	}
}

func (search *DocumentSearch) observeBest(document Document, reward float64) {
	if search.hasBest && reward <= search.bestReward {
		return
	}

	clone := cloneDocument(document)
	search.best = &clone
	search.bestReward = reward
	search.hasBest = true
}

func (search *DocumentSearch) observeUntracked(document Document, reward float64) {
	if search.root != nil {
		return
	}

	search.root = newDocumentSearchNode(document, search.profile, search.random)
	search.root.visits = 1
	search.root.reward = reward
}

func newDocumentSearchNode(
	document Document,
	profile SearchProfile,
	random *rand.Rand,
) *documentSearchNode {
	document = normalizeSearchDocument(document, profile, random)

	return &documentSearchNode{
		document: document,
		untried:  searchActions(document, profile, random),
	}
}

func documentSearchKey(document Document) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "v:%d;", document.Version)

	for _, playbook := range document.Playbooks {
		fmt.Fprintf(
			&builder,
			"p:%s:%s:%s;",
			playbook.Name,
			playbook.Regime,
			playbook.Policy,
		)
		writeBranchSearchKey(&builder, "d", playbook.Deny)
		writeBranchSearchKey(&builder, "e", playbook.Entry)
		writeBranchSearchKey(&builder, "x", playbook.Exit)
	}

	return builder.String()
}

func writeBranchSearchKey(
	builder *strings.Builder,
	prefix string,
	branches []BranchSpec,
) {
	fmt.Fprintf(builder, "%s[", prefix)

	for _, branch := range branches {
		builder.WriteString("{")
		writeBranchSearchKey(builder, "any", branch.Any)
		writeBranchSearchKey(builder, "all", branch.All)
		writeBranchSearchKey(builder, "child", branch.Branches)
		fmt.Fprintf(
			builder,
			"c:%s;o:%s;m:%s;u:%s;k:%s;a:%s;",
			branch.Category,
			branch.Observation,
			branch.Metric,
			branch.Unit,
			branch.Condition,
			branch.Action,
		)

		if branch.Value != nil {
			fmt.Fprintf(builder, "v:%0.12f;", *branch.Value)
		}

		builder.WriteString("}")
	}

	builder.WriteString("]")
}
