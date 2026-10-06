package cognition

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
)

type treeNode struct {
	Prefix      string     `json:"prefix"`
	Probability float64    `json:"probability"`
	State       string     `json:"state"`
	Children    []treeNode `json:"children"`
}

type treeExport struct {
	Root     treeNode `json:"root"`
	Branches []struct {
		Policy string `json:"policy"`
	} `json:"branches"`
	Feasible []struct {
		Action string `json:"action"`
	} `json:"feasible"`
}

func exported(memory *Associate) treeExport {
	reading, err := drive(NewExport(memory), nil, nil)
	So(err, ShouldBeNil)
	raw, readErr := literal(reading, "tree")
	So(readErr, ShouldBeNil)

	var export treeExport
	So(json.Unmarshal([]byte(raw), &export), ShouldBeNil)
	return export
}

func observe(memory *Associate, context string, class string, times int) {
	for range times {
		_, err := drive(memory, map[string]string{
			"context": context,
			"class":   class,
		}, map[string]float64{
			"feedback": 1,
			"graded":   core.Unit,
		})
		So(err, ShouldBeNil)
	}
}

func TestExportNext(t *testing.T) {
	Convey("Given one enter association", t, func() {
		memory := NewAssociate()
		observe(memory, "R3_R8_R19/R1_R2_R5", "enter", 1)
		export := exported(memory)

		var enterMidPath bool
		var walk func(node treeNode, depth int)
		walk = func(node treeNode, depth int) {
			prefix := strings.ToUpper(node.Prefix)
			action := prefix == "ENTER" || prefix == "EXIT"

			if action && len(node.Children) > 0 {
				enterMidPath = true
			}

			if !action && depth > 0 && (prefix == "WAIT" || prefix == "ENTER" || prefix == "EXIT") {
				enterMidPath = true
			}

			for _, child := range node.Children {
				walk(child, depth+1)
			}
		}
		walk(export.Root, 0)

		So(enterMidPath, ShouldBeFalse)

		var waitLeaf bool
		var walkWait func(node treeNode)
		walkWait = func(node treeNode) {
			if strings.ToUpper(node.Prefix) == "WAIT" {
				waitLeaf = true
			}
			for _, child := range node.Children {
				walkWait(child)
			}
		}
		walkWait(export.Root)
		So(waitLeaf, ShouldBeFalse)

		So(export.Root.Children[0].Children[0].Children[0].Prefix, ShouldEqual, "ENTER")
		So(export.Root.Children[0].Children[0].Children[0].Probability, ShouldEqual, 0.75)
		So(export.Root.Children[0].Children[0].Children[0].State, ShouldEqual, "POLICY CHOICE")
	})

	Convey("Given a repeated region frame", t, func() {
		memory := NewAssociate()
		observe(memory, "R4_R5_R15/R4_R5_R15/R4_R5_R15", "enter", 1)
		export := exported(memory)

		So(len(export.Root.Children), ShouldEqual, 1)
		So(export.Root.Children[0].Prefix, ShouldEqual, "R4_R5_R15")
		So(len(export.Root.Children[0].Children), ShouldEqual, 1)
		So(export.Root.Children[0].Children[0].Prefix, ShouldEqual, "ENTER")
	})

	Convey("Given an ABBA region path", t, func() {
		memory := NewAssociate()
		observe(memory, "R1_R2_R3/R7_R8_R9/R7_R8_R9/R1_R2_R3", "enter", 1)
		export := exported(memory)

		first := export.Root.Children[0]
		second := first.Children[0]
		third := second.Children[0]
		So(first.Prefix, ShouldEqual, "R1_R2_R3")
		So(second.Prefix, ShouldEqual, "R7_R8_R9")
		So(third.Prefix, ShouldEqual, "R1_R2_R3")
		So(third.Children[0].Prefix, ShouldEqual, "ENTER")
	})

	Convey("Given many enter contexts and fewer exit contexts", t, func() {
		memory := NewAssociate()

		for index := 1; index <= 70; index++ {
			observe(memory, string([]byte{byte(index), 10, 20, 0}), "enter", 10)
		}

		for index := 1; index <= 5; index++ {
			observe(memory, string([]byte{100, byte(index), 50, 0}), "exit", 2)
		}

		export := exported(memory)
		exitFound := false
		var walk func(node treeNode)
		walk = func(node treeNode) {
			if strings.ToUpper(node.Prefix) == "EXIT" {
				exitFound = true
				return
			}

			for _, child := range node.Children {
				walk(child)
			}
		}
		walk(export.Root)
		So(exitFound, ShouldBeTrue)
		So(len(export.Branches), ShouldEqual, 75)
		So(len(export.Feasible), ShouldEqual, 75)
	})
}

func TestExportSkipsWaitLeaf(t *testing.T) {
	Convey("Given only enter associations, export never terminates in WAIT", t, func() {
		memory := NewAssociate()
		observe(memory, "R1_R2/R3_R4", "enter", 2)
		export := exported(memory)

		var waitLeaf bool
		var walk func(node treeNode)
		walk = func(node treeNode) {
			if strings.ToUpper(node.Prefix) == "WAIT" && len(node.Children) == 0 {
				waitLeaf = true
			}
			for _, child := range node.Children {
				walk(child)
			}
		}
		walk(export.Root)
		So(waitLeaf, ShouldBeFalse)
		So(export.Branches[0].Policy, ShouldEqual, "ENTER")
	})
}
