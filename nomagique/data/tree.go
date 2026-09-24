package data

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/theapemachine/errnie"
)

/*
TreeServer grows a prefix tree from paths.
*/
type TreeServer struct {
	root       []byte
	candidates []byte
	branches   []byte
}

func NewTree() *TreeServer {
	return &TreeServer{}
}

/*
branch is one node of a grown tree, in the shape a tree view draws.
*/
type branch struct {
	ID          string    `json:"id"`
	Prefix      string    `json:"prefix"`
	Probability float64   `json:"probability"`
	Tokens      []string  `json:"tokens,omitempty"`
	Label       string    `json:"label,omitempty"`
	Share       float64   `json:"share,omitempty"`
	Visits      int       `json:"visits"`
	IsEnd       bool      `json:"isEnd,omitempty"`
	State       string    `json:"state,omitempty"`
	Children    []*branch `json:"children,omitempty"`
	visits      int
	endings     map[string]int
	index       map[string]*branch
}

/*
ending is one node where paths end, with the label most of them carried.
*/
type ending struct {
	ID         string  `json:"id"`
	Signature  string  `json:"signature"`
	Depth      int     `json:"depth"`
	Visits     int     `json:"visits"`
	Confidence float64 `json:"confidence"`
	Policy     string  `json:"policy"`
}

/*
candidate is one label seen ending at a node.
*/
type candidate struct {
	ID          string  `json:"id"`
	Rank        int     `json:"rank"`
	Action      string  `json:"action"`
	Prefix      string  `json:"prefix"`
	Probability float64 `json:"probability"`
	State       string  `json:"state"`
}

func (server *TreeServer) Write(ctx context.Context, call Tree_write) error {
	server.root, server.candidates, server.branches = nil, nil, nil
	payload, err := call.Args().Items()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.tree: failed to read items", err))
	}

	separator, err := call.Args().Separator()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.tree: failed to read separator", err))
	}

	if separator == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "data.tree: separator must be declared", nil))
	}

	if len(payload) == 0 {
		return nil
	}

	var items []struct {
		Path  string `json:"path"`
		Label string `json:"label"`
	}

	if err := json.Unmarshal(payload, &items); err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.tree: items are not a JSON array of paths", err))
	}

	root := &branch{ID: "root", index: map[string]*branch{}, endings: map[string]int{}}
	total := 0

	for _, item := range items {
		if item.Path == "" {
			continue
		}

		total++
		root.visits++
		node := root

		for _, step := range strings.Split(item.Path, separator) {
			child, known := node.index[step]

			if !known {
				prefix := step

				if node != root {
					prefix = node.Prefix + separator + step
				}

				child = &branch{
					ID: prefix, Prefix: prefix, Tokens: strings.Split(step, ","),
					index: map[string]*branch{}, endings: map[string]int{},
				}
				node.index[step] = child
				node.Children = append(node.Children, child)
			}

			child.visits++
			node = child
		}

		node.IsEnd = true
		node.endings[item.Label]++
	}

	if total == 0 {
		return nil
	}

	var candidates []candidate
	endings := []ending{}
	var settle func(*branch, int)

	settle = func(node *branch, depth int) {
		node.Probability = float64(node.visits) / float64(total)
		node.Visits = node.visits

		if node.IsEnd {
			node.State = "EVALUATED"
			ended := 0

			for _, count := range node.endings {
				ended += count
			}

			policy, most := "", 0

			for label, count := range node.endings {
				candidates = append(candidates, candidate{
					ID: node.Prefix + "#" + label, Action: label, Prefix: node.Prefix,
					Probability: float64(count) / float64(ended), State: "EVALUATED",
				})

				if count > most || (count == most && label < policy) {
					policy, most = label, count
				}
			}

			node.Label, node.Share = policy, float64(most)/float64(ended)

			endings = append(endings, ending{
				ID: node.Prefix, Signature: node.Prefix, Depth: depth, Visits: ended,
				Confidence: float64(most) / float64(ended), Policy: policy,
			})
		}

		for _, child := range node.Children {
			settle(child, depth+1)
		}
	}

	settle(root, 0)
	sort.SliceStable(endings, func(left, right int) bool {
		return endings[left].Visits > endings[right].Visits
	})
	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].Probability > candidates[right].Probability
	})

	for index := range candidates {
		candidates[index].Rank = index + 1
	}

	encodedRoot, err := json.Marshal(root)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.tree: failed to encode root", err))
	}

	encodedCandidates, err := json.Marshal(candidates)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.tree: failed to encode candidates", err))
	}

	encodedBranches, err := json.Marshal(endings)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.tree: failed to encode branches", err))
	}

	server.root, server.candidates, server.branches = encodedRoot, encodedCandidates, encodedBranches

	if len(candidates) == 0 {
		server.candidates = []byte("[]")
	}

	return nil
}

func (server *TreeServer) Done(ctx context.Context, call Tree_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.tree: failed to allocate results", err))
	}

	results.SetIdle()

	if server.root == nil {
		return nil
	}

	results.SetGrown()

	for _, set := range []error{
		results.Grown().SetRoot(server.root),
		results.Grown().SetCandidates(server.candidates),
		results.Grown().SetBranches(server.branches),
	} {
		if set != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "data.tree: failed to set a result", fmt.Errorf("%w", set)))
		}
	}

	server.root, server.candidates, server.branches = nil, nil, nil
	return nil
}
