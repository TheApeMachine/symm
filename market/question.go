package market

/*
QuestionType is the type of question that the Perspective is asking.
*/
type QuestionType uint8

const (
	QuestionTypeNone QuestionType = iota
	QuestionIsSpoofing
	QuestionTypeOrigin
	QuestionTypeQuality
	QuestionTypeTiming
)

/*
Question is a node in the Perspective's decision tree.
*/
type Question struct {
}
