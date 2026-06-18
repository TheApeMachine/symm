package trader

import (
	"github.com/theapemachine/symm/ui"
)

/*
ConnectSnapshotFrames returns dashboard frames that must survive a browser refresh.
*/
func (crypto *Crypto) ConnectSnapshotFrames() []map[string]any {
	if crypto == nil || crypto.story == nil {
		return nil
	}

	frames := make([]map[string]any, 0, 4)

	branches := crypto.story.DecisionTreeBranches()

	if len(branches) > 0 {
		frames = append(frames, map[string]any{
			"type":     "decision_tree",
			"branches": branches,
		})
	}

	if crypto.wallet != nil {
		walletFrame := ui.WalletFrame(crypto.wallet)

		if ui.WalletFramePublishable(walletFrame) {
			frames = append(frames, walletFrame)
		}
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

	if crypto.memory != nil {
		for _, reading := range crypto.memory.LatestReadings() {
			frame := ui.CognitiveFrame(cognitiveReadingWire(reading))

			if frame != nil {
				frames = append(frames, frame)
			}
		}
	}

	return frames
}
