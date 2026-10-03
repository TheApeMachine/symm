package cognition

import (
	"encoding/binary"
	"strings"
	"testing"
)

func frameOf(tokens ...string) []byte {
	frame := make([]byte, 4+8*len(tokens))
	binary.BigEndian.PutUint32(frame, uint32(len(tokens)))

	for index, token := range tokens {
		copy(frame[4+index*8+8-len(token):], token)
	}

	return frame
}

func TestRegionFramesDecodesTimesteps(t *testing.T) {
	ctx := append(frameOf("R1", "R2", "R3"), frameOf("R10", "R20")...)
	frames := regionFrames(ctx)

	if len(frames) != 2 || frames[0] != "[R1,R2,R3]" || frames[1] != "[R10,R20]" {
		t.Fatalf("frames=%v", frames)
	}
}

func TestRegionFramesRejectsTruncated(t *testing.T) {
	ctx := frameOf("R1", "R2")

	if frames := regionFrames(ctx[:len(ctx)-1]); frames != nil {
		t.Fatalf("truncated signature decoded: %v", frames)
	}
}

func TestTreeExportActionIsLeafOnly(t *testing.T) {
	engine := NewEngine(Config{})
	ctx := append(frameOf("R3", "R8", "R19"), frameOf("R1", "R2", "R5")...)
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
		// Region path nodes must look like [id,...]
		if depth > 0 && !isAction && !strings.HasPrefix(node.TokenPrefix, "[") {
			t.Fatalf("path node prefix should be region frame, got %q", node.TokenPrefix)
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
	frame := frameOf("R4", "R5", "R15")
	ctx := append(append(append([]byte{}, frame...), frame...), frame...)
	got := regionFrames(ctx)

	if len(got) != 1 || got[0] != "[R4,R5,R15]" {
		t.Fatalf("AAA→A frames: got %#v", got)
	}
}

func TestRegionFramesCollapsesABBA(t *testing.T) {
	first, second := frameOf("R1", "R2", "R3"), frameOf("R7", "R8", "R9")
	ctx := append(append(append(append([]byte{}, first...), second...), second...), first...)
	got := regionFrames(ctx)
	want := []string{"[R1,R2,R3]", "[R7,R8,R9]", "[R1,R2,R3]"}

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
