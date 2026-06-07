package reasoning

/*
This file is the tree language — a playbook that expresses a thought process, not a
one-step reflex. It is the one decision language: the live story, the replay scorer,
and the optimizer all read and write it.

A Thought is one node in the reasoning:

	when: <predicate>   the condition that makes this thought relevant
	then: [<thought>]   the reasoning that follows ONCE `when` holds — these are
	                    monitored on the ticks that FOLLOW, so `then` reads as
	                    "and then, over time, watch for ...". This is what makes
	                    depth a temporal sequence instead of a snapshot conjunction.
	do:   <action>      the decision taken here, if any. A node may both `do` and
	                    `then` — "act, and keep thinking" (enter, then manage).
*/
type Thought struct {
	// Name labels a root branch as one named setup ("quick_pump", "slow_pump"):
	// every trade it produces is attributed to this name in replay scoreboards
	// and position_outcome audits. Empty names are stamped "branch_N" on load.
	Name string    `yaml:"name,omitempty"`
	When Predicate `yaml:"when"`
	Then []Thought `yaml:"then,omitempty"`
	Do   Act       `yaml:"do,omitempty"`
}
