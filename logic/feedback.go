package logic

import "time"

type Feedback struct {
	Symbol string
	Value  float64
	Time   time.Time
}

func NewFeedback(
	symbol string,
	value float64,
	time time.Time,
) *Feedback {
	return &Feedback{Symbol: symbol, Value: value, Time: time}
}
