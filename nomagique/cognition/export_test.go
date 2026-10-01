package cognition

import (
	"strings"
	"testing"
)

func TestRegionFramesCapsToDesignedN(t *testing.T) {
	// Legacy/corrupt: one frame with >3 region IDs and no null separators mixed
	// with a proper null-separated pair.
	long := []byte{1, 2, 3, 4, 5, 0, 10, 20, 30}
	frames := regionFrames(long)
	if len(frames) != 2 {
		t.Fatalf("frames=%v", frames)
	}
	if frames[0] != "[1,2,3]" {
		t.Fatalf("capped first frame: %q", frames[0])
	}
	if frames[1] != "[10,20,30]" {
		t.Fatalf("second frame: %q", frames[1])
	}
}

func TestTreeExportActionIsLeafOnly(t *testing.T) {
	engine := NewEngine(Config{})
	ctx := []byte{3, 8, 19, 0, 1, 2, 5, 0}
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
	// A A A with null separators → single frame
	ctx := []byte{4, 5, 215, 0, 4, 5, 215, 0, 4, 5, 215, 0}
	got := regionFrames(ctx)
	if len(got) != 1 || got[0] != "[4,5,215]" {
		t.Fatalf("AAA→A frames: got %#v", got)
	}
}

func TestRegionFramesCollapsesABBA(t *testing.T) {
	ctx := []byte{1, 2, 3, 0, 7, 8, 9, 0, 7, 8, 9, 0, 1, 2, 3, 0}
	got := regionFrames(ctx)
	want := []string{"[1,2,3]", "[7,8,9]", "[1,2,3]"}
	if len(got) != len(want) {
		t.Fatalf("ABBA→ABA len: got %#v want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ABBA→ABA: got %#v want %#v", got, want)
		}
	}
}
