import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { learningStore } from "#/collections/learning";
import { LearningRecognitionT } from "#/providers/telemetry/telemetry/learning-recognition";
import { learningFixture } from "./fixture";
import { RecognitionPanel } from "./recognition-panel";

/*
The learning surface has exactly one scroller per column. A panel that opens its
own inside one of them is not a cosmetic problem: the band above it holds a
fixed height, so the nested scroller inherits whatever few pixels are left and
the panel becomes unreachable — visible, but only scrollable through a sliver.

This asserts the containment rather than the appearance, because that is what
actually broke, and it renders the panel with telemetry present: the empty
early-return path never reaches the element that carried the bug.
*/
describe("learning surface scroll containment", () => {
	it("does not open a scroller inside the column that already scrolls", () => {
		const state = learningFixture();
		state.recognition = new LearningRecognitionT();
		learningStore.setState(() => state);

		const markup = renderToStaticMarkup(<RecognitionPanel />);

		// The panel really rendered its content, not its "nothing yet" notice.
		expect(markup).toContain("Impulse map");
		expect(markup).not.toContain("overflow-auto");
	});
});
