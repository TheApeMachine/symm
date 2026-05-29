package market

/*
BranchType is the type of branch that the Perspective is asking.
*/
type BranchType uint8

const (
	BranchTypeNone BranchType = iota
	BranchHasSpiked
)

/*
Branch is a branch in the Perspective's decision tree.
*/
type Branch struct {
	Question QuestionType
}
