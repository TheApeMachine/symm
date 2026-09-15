import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { RecognitionPanel } from "./recognition-panel";

describe("learning surface scroll containment", () => {
	it("does not open a scroller inside the column that already scrolls", () => {
		const markup = renderToStaticMarkup(<RecognitionPanel />);
		expect(markup).toContain("Precursor recognition");
		expect(markup).not.toContain("overflow-auto");
	});
});
