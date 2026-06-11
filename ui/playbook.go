package ui

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

/*
DecisionTreeWireFrame encodes the embedded playbook for a websocket client.
*/
func DecisionTreeWireFrame() (map[string]any, error) {
	tree, err := logic.LoadTree()

	if errnie.Error(err) != nil {
		return nil, err
	}

	return uiWireFrame(&qpool.QValue[any]{
		Type:  "decision_tree",
		Value: tree,
	})
}
