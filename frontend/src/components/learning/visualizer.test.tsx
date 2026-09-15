import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { LearningVisualizer } from "./visualizer";

describe("LearningVisualizer", () => {
	it("renders the learning visualizer canvas without crashing", () => {
		const markup = renderToStaticMarkup(<LearningVisualizer />);
		expect(markup).toContain("Learning");
		expect(markup).toContain("Edge:");
		expect(markup).toContain("Policy choice:");
	});
});
