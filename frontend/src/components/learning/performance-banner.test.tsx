import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { LearningPerformanceBanner } from "./performance-banner";

describe("LearningPerformanceBanner", () => {
	it("renders both precursor cognition and forward evaluation sections with data-metric attributes", () => {
		const html = renderToStaticMarkup(<LearningPerformanceBanner />);

		// Architecture
		expect(html).toContain("PARALLEL LEARNERS (DECOUPLED)");

		// Pillar 1: Precursor model
		expect(html).toContain("PRECURSOR COGNITION");
		expect(html).toContain("Learned Situations");
		expect(html).toContain('data-metric="decisions"');
		expect(html).toContain('data-metric="steps"');
		expect(html).toContain('data-metric="confidence"');
		expect(html).toContain('data-metric="contrast"');

		// Pillar 2: Forward evaluation
		expect(html).toContain("FORWARD EVALUATION");
		expect(html).toContain("Measured Edge");
		expect(html).toContain('data-metric="edge"');
		expect(html).toContain('data-metric="win_rate"');
		expect(html).toContain('data-metric="accuracy"');
		expect(html).toContain('data-metric="resolved"');

		// Readiness gates
		expect(html).toContain("Execution Readiness");
		expect(html).toContain('data-l="gate-count"');
	});
});
