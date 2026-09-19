package tables

import "time"

/*
Run records the immutable startup facts of one SYMM process execution.
*/
type Run struct {
	Epoch        int64     `json:"epoch"`
	StartedAt    time.Time `json:"startedAt"`
	CodeCommit   string    `json:"codeCommit"`
	BuildID      string    `json:"buildId"`
	ConfigDigest string    `json:"configDigest"`
	Status       string    `json:"status"`
}
