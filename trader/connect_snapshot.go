package trader

import (
	"github.com/theapemachine/symm/ui"
)

/*
ConnectSnapshotFrames returns dashboard frames that must survive a browser refresh.
*/
func (crypto *Crypto) ConnectSnapshotFrames() []map[string]any {
	frames := make([]map[string]any, 0, 2)

	branches := crypto.story.DecisionTreeBranches()

	if len(branches) > 0 {
		frames = append(frames, map[string]any{
			"type":     "decision_tree",
			"branches": branches,
		})
	}

	walletFrame := ui.WalletFrame(crypto.balances.Snapshot())

	if ui.WalletFramePublishable(walletFrame) {
		frames = append(frames, walletFrame)
	}

	measurements := crypto.story.Measurements()

	if len(measurements) > 0 {
		frames = append(frames, ui.StateFrame(
			measurements,
			crypto.storyTicks.Load(),
			crypto.story.PlaybookEvaluationCount(),
			crypto.story.AnchorWalkTrace(),
		))
	}

	return frames
}
