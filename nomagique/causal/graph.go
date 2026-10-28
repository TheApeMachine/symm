package causal

import "fmt"

type causalDAG struct {
	nodeCount int
	parents   [][]int
	children  [][]int
}

type graphVisit struct {
	node       int
	fromParent bool
}

func newDAG(parents [][]int) (causalDAG, error) {
	if len(parents) == 0 {
		return causalDAG{}, fmt.Errorf("causal: graph requires at least one node")
	}

	nodeCount := len(parents)
	parentCopy := make([][]int, nodeCount)
	children := make([][]int, nodeCount)

	for node := range nodeCount {
		parentCopy[node] = append([]int(nil), parents[node]...)

		for _, parent := range parentCopy[node] {
			if parent < 0 || parent >= nodeCount {
				return causalDAG{}, fmt.Errorf(
					"causal: parent %d of node %d outside graph width %d",
					parent, node, nodeCount,
				)
			}

			if parent == node {
				return causalDAG{}, fmt.Errorf("causal: node %d cannot be its own parent", node)
			}

			children[parent] = append(children[parent], node)
		}
	}

	dag := causalDAG{
		nodeCount: nodeCount,
		parents:   parentCopy,
		children:  children,
	}

	if err := dag.ensureAcyclic(); err != nil {
		return causalDAG{}, err
	}

	return dag, nil
}

func (dag causalDAG) ensureAcyclic() error {
	state := make([]int, dag.nodeCount)

	var visit func(int) error

	visit = func(node int) error {
		switch state[node] {
		case 1:
			return fmt.Errorf("causal: graph contains a cycle through node %d", node)
		case 2:
			return nil
		}

		state[node] = 1

		for _, child := range dag.children[node] {
			if err := visit(child); err != nil {
				return err
			}
		}

		state[node] = 2

		return nil
	}

	for node := 0; node < dag.nodeCount; node++ {
		if err := visit(node); err != nil {
			return err
		}
	}

	return nil
}

func (dag causalDAG) backdoorAdmissible(treatment, target int, controls []int) (bool, error) {
	if err := dag.validateNode(treatment); err != nil {
		return false, err
	}

	if err := dag.validateNode(target); err != nil {
		return false, err
	}

	for _, control := range controls {
		if err := dag.validateNode(control); err != nil {
			return false, err
		}
	}

	controlSet := nodeSet(controls)
	descendants := dag.descendants(treatment)

	for control := range controlSet {
		if _, blocked := descendants[control]; blocked {
			return false, nil
		}
	}

	cutGraph := dag.withoutOutgoing(treatment)

	return cutGraph.mSeparated(treatment, target, controlSet), nil
}

func (dag causalDAG) descendants(node int) map[int]struct{} {
	reachable := make(map[int]struct{})
	queue := append([]int(nil), dag.children[node]...)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if _, seen := reachable[current]; seen {
			continue
		}

		reachable[current] = struct{}{}
		queue = append(queue, dag.children[current]...)
	}

	return reachable
}

func (dag causalDAG) mSeparated(start, target int, conditioning map[int]struct{}) bool {
	reachable := dag.bayesBallReachable(start, conditioning)
	_, connected := reachable[target]

	return !connected
}

func (dag causalDAG) bayesBallReachable(start int, conditioning map[int]struct{}) map[int]struct{} {
	reached := map[int]struct{}{start: {}}
	seen := make(map[graphVisit]struct{})
	queue := []graphVisit{
		{node: start, fromParent: true},
		{node: start, fromParent: false},
	}

	for len(queue) > 0 {
		visit := queue[0]
		queue = queue[1:]

		if _, recorded := seen[visit]; recorded {
			continue
		}

		seen[visit] = struct{}{}
		reached[visit.node] = struct{}{}

		_, conditioned := conditioning[visit.node]

		if conditioned {
			if visit.fromParent {
				for _, parent := range dag.parents[visit.node] {
					queue = append(queue, graphVisit{node: parent, fromParent: true})
				}
			} else {
				for _, child := range dag.children[visit.node] {
					queue = append(queue, graphVisit{node: child, fromParent: false})
				}
			}

			continue
		}

		if visit.fromParent {
			for _, child := range dag.children[visit.node] {
				queue = append(queue, graphVisit{node: child, fromParent: false})
			}

			continue
		}

		for _, parent := range dag.parents[visit.node] {
			queue = append(queue, graphVisit{node: parent, fromParent: true})
		}

		for _, child := range dag.children[visit.node] {
			queue = append(queue, graphVisit{node: child, fromParent: false})
		}
	}

	return reached
}

func (dag causalDAG) withoutOutgoing(treatment int) causalDAG {
	parents := make([][]int, dag.nodeCount)
	children := make([][]int, dag.nodeCount)

	for node := range dag.parents {
		parents[node] = append([]int(nil), dag.parents[node]...)

		if node == treatment {
			continue
		}

		children[node] = append([]int(nil), dag.children[node]...)
	}

	for _, child := range dag.children[treatment] {
		filtered := parents[child][:0]

		for _, parent := range parents[child] {
			if parent == treatment {
				continue
			}

			filtered = append(filtered, parent)
		}

		parents[child] = filtered
	}

	return causalDAG{
		nodeCount: dag.nodeCount,
		parents:   parents,
		children:  children,
	}
}

func (dag causalDAG) validateNode(node int) error {
	if node < 0 || node >= dag.nodeCount {
		return fmt.Errorf("causal: node %d outside graph width %d", node, dag.nodeCount)
	}

	return nil
}

func nodeSet(nodes []int) map[int]struct{} {
	set := make(map[int]struct{}, len(nodes))

	for _, node := range nodes {
		set[node] = struct{}{}
	}

	return set
}
