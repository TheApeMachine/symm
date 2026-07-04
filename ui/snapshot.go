package ui

import (
	"strings"
	"sync"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
Snapshot keeps the latest backend-owned UI artifacts for new websocket clients.
*/
type Snapshot struct {
	artifacts sync.Map
}

func NewSnapshot() *Snapshot {
	return &Snapshot{}
}

func (snapshot *Snapshot) Observe(artifact *datura.Artifact) error {
	if snapshot == nil {
		return errnie.Err(errnie.Validation, "ui: snapshot is nil", nil)
	}

	if artifact == nil {
		return errnie.Err(errnie.Validation, "ui: snapshot artifact is nil", nil)
	}

	snapshot.artifacts.Store(snapshot.key(artifact), artifact)
	return nil
}

func (snapshot *Snapshot) Replay(conn *websocket.Conn) error {
	if snapshot == nil {
		return errnie.Err(errnie.Validation, "ui: snapshot is nil", nil)
	}

	if conn == nil {
		return errnie.Err(errnie.Validation, "ui: websocket connection is nil", nil)
	}

	var err error

	snapshot.artifacts.Range(func(_, value any) bool {
		artifact, ok := value.(*datura.Artifact)

		if !ok || artifact == nil {
			err = errnie.Err(errnie.Validation, "ui: snapshot contains invalid artifact", nil)
			return false
		}

		if writeErr := conn.WriteMessage(websocket.BinaryMessage, artifact.Pack()); writeErr != nil {
			err = errnie.Err(errnie.IO, "ui: replay latest frame failed", writeErr)
			return false
		}

		return true
	})

	return err
}

func (snapshot *Snapshot) key(artifact *datura.Artifact) string {
	role := artifactPart(artifact.Role())
	origin := artifactPart(artifact.Origin())
	scope := artifactPart(artifact.Scope())

	if role == "measurement" && origin != "" {
		return strings.Join([]string{role, origin}, "/")
	}

	if role == "decision" && scope != "" {
		return strings.Join([]string{role, scope}, "/")
	}

	if role != "" {
		return role
	}

	if origin != "" && scope != "" {
		return strings.Join([]string{origin, scope}, "/")
	}

	if origin != "" {
		return origin
	}

	if scope != "" {
		return scope
	}

	return "artifact"
}

func artifactPart(value string, err error) string {
	if err != nil {
		return ""
	}

	return value
}
