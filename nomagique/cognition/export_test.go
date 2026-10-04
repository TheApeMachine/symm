package cognition

import (
	"strings"
	"testing"
)

func TestRegionFramesDecodesTimesteps(t *testing.T) {
	ctx := []byte("R1_R2_R3/R10_R20")
	frames := regionFrames(ctx)

	if len(frames) != 2 || frames[0] != "R1_R2_R3" || frames[1] != "R10_R20" {
		t.Fatalf("frames=%v", frames)
	}
}

func TestTreeExportActionIsLeafOnly(t *testing.T) {
	engine := NewEngine(Config{})
	ctx := []byte("R3_R8_R19/R1_R2_R5")
	if _, err := engine.Observe(Association{
		Context:  ctx,
		Class:    []byte(ActionEnter),
		Feedback: 1,
		Graded:   true,
	}); err != nil {
		t.Fatalf("observe: %v", err)
	}

	export := engine.TreeExport()
	if export.Root == nil {
		t.Fatal("nil root")
	}

	var enterMidPath bool
	var walk func(node *TrieNodeJSON, depth int)
	walk = func(node *TrieNodeJSON, depth int) {
		if node == nil {
			return
		}
		prefix := strings.ToUpper(node.TokenPrefix)
		isAction := prefix == "ENTER" || prefix == "EXIT"
		if isAction {
			if len(node.Children) > 0 {
				t.Fatalf("action leaf %q has children", node.TokenPrefix)
			}
		} else if depth > 0 && (prefix == "WAIT" || prefix == "ENTER" || prefix == "EXIT") {
			enterMidPath = true
		}
		for _, child := range node.Children {
			walk(child, depth+1)
		}
	}
	walk(export.Root, 0)
	if enterMidPath {
		t.Fatal("ENTER/WAIT painted on mid-path region node")
	}
}

func TestRegionFramesCollapsesAAA(t *testing.T) {
	ctx := []byte("R4_R5_R15/R4_R5_R15/R4_R5_R15")
	got := regionFrames(ctx)

	if len(got) != 1 || got[0] != "R4_R5_R15" {
		t.Fatalf("AAA→A frames: got %#v", got)
	}
}

func TestRegionFramesCollapsesABBA(t *testing.T) {
	ctx := []byte("R1_R2_R3/R7_R8_R9/R7_R8_R9/R1_R2_R3")
	got := regionFrames(ctx)
	want := []string{"R1_R2_R3", "R7_R8_R9", "R1_R2_R3"}

	if len(got) != len(want) {
		t.Fatalf("ABBA→ABA len: got %#v want %#v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ABBA→ABA: got %#v want %#v", got, want)
		}
	}
}

func TestTreeExportStratifiedExitVisibility(t *testing.T) {
	engine := NewEngine(Config{})

	// Populate 70 distinct ENTER contexts with high count.
	for i := byte(1); i <= 70; i++ {
		ctx := []byte{i, 10, 20, 0}
		for c := 0; c < 10; c++ {
			if _, err := engine.Observe(Association{
				Context:  ctx,
				Class:    []byte(ActionEnter),
				Feedback: 1,
				Graded:   true,
			}); err != nil {
				t.Fatalf("observe enter: %v", err)
			}
		}
	}

	// Populate 5 EXIT contexts with lower count.
	for i := byte(1); i <= 5; i++ {
		ctx := []byte{100, i, 50, 0}
		for c := 0; c < 2; c++ {
			if _, err := engine.Observe(Association{
				Context:  ctx,
				Class:    []byte(ActionExit),
				Feedback: 1,
				Graded:   true,
			}); err != nil {
				t.Fatalf("observe exit: %v", err)
			}
		}
	}

	export := engine.TreeExport()
	if export.Root == nil {
		t.Fatal("nil root")
	}

	exitFound := false
	var walk func(node *TrieNodeJSON)
	walk = func(node *TrieNodeJSON) {
		if node == nil {
			return
		}
		if strings.ToUpper(node.TokenPrefix) == "EXIT" {
			exitFound = true
			return
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(export.Root)

	if !exitFound {
		t.Fatal("EXIT candidate was starved by higher frequency ENTER candidates; stratified selection failed")
	}
}
