package logic

/*
Evaluation is the playbook outcome for one symbol window, including the branch
that fired so dashboards can tie decisions to tree nodes.
*/
type Evaluation struct {
	Action *Action
	Key    string
}
