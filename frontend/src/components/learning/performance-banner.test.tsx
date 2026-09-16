import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { LearningPerformanceBanner } from "./performance-banner";

describe("LearningPerformanceBanner", () => {
	it("renders both precursor cognition and forward evaluation sections with data-metric attributes", () => {
		const html = renderToStaticMarkup(<LearningPerformanceBanner />);

		// Architecture
		expect(html).toContain("VOLUME-CLOCK LEARNING");

		// Pillar 1: Precursor model
		expect(html).toContain("PRECURSOR COGNITION");
		expect(html).toContain("Learned Situations");
		expect(html).toContain('data-metric="decisions"');
		expect(html).toContain('data-metric="steps"');
		expect(html).toContain('data-metric="resolved"');
		expect(html).toContain('data-metric="unsupported"');
		expect(html).not.toContain('data-metric="confidence"');

		// Pillar 2: Forward evaluation
		expect(html).toContain("FORWARD EVALUATION");
		expect(html).toContain("Measured Edge");
		expect(html).toContain('data-metric="edge"');
		expect(html).toContain('data-metric="win_rate"');
		expect(html).toContain('data-metric="accuracy"');
		expect(html).toContain('data-metric="evaluated"');

		// Training does not claim execution readiness.
		expect(html).toContain("Training only");
		expect(html).toContain('data-l="gate-count"');
	});
});
