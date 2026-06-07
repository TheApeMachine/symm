package reasoning

import "fmt"

/*
StampStrategies gives every root branch a setup identity and propagates it onto
each action in that branch's subtree. The name is how a forest stops being one
anonymous blob: replay scoreboards, audit outcomes, and dashboards attribute
every trade to the named setup that produced it. Unnamed roots get a stable
positional name so hand-written and generated forests behave identically.
*/
func StampStrategies(forest []Thought) {
	for index := range forest {
		if forest[index].Name == "" {
			forest[index].Name = fmt.Sprintf("branch_%d", index+1)
		}

		stampSubtree(&forest[index], forest[index].Name)
	}
}

func stampSubtree(node *Thought, strategy string) {
	if node.Do.Type != ActionNone && node.Do.Strategy == "" {
		node.Do.Strategy = strategy
	}

	for index := range node.Then {
		stampSubtree(&node.Then[index], strategy)
	}
}
