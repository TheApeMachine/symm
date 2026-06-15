package logic

import "time"

type Signal interface {
	Measure(*Feedback, time.Time) (Measurement, error)
}
