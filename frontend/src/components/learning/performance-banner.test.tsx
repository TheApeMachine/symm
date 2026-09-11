import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { learningFixture } from "./fixture";
import { LearningPerformanceBanner } from "./performance-banner";
import { projectLearning } from "./state";

describe("LearningPerformanceBanner", () => {
	it("renders both parallel precursor model and main agent forward testing sections", () => {
		const view = projectLearning(learningFixture(), "");
		const html = renderToStaticMarkup(<LearningPerformanceBanner view={view} />);

		// Architecture
		expect(html).toContain("PARALLEL LEARNERS (DECOUPLED)");
		expect(html).toContain("MAIN AGENT");

		// Pillar 1: Precursor model
		expect(html).toContain("PRECURSOR RECOGNITION (PARALLEL AGENTS)");
		expect(html).toContain("Learned Situations");
		expect(html).toContain("Consensus Accuracy");
		expect(html).toContain("Mean Contrast");

		// Pillar 2: Main Agent forward testing
		expect(html).toContain("MAIN AGENT FORWARD TESTING (POLICY TRADER)");
		expect(html).toContain("Measured Edge");
		expect(html).toContain("Simulated Win Rate");
		expect(html).toContain("Simulated Net P&amp;L");
		expect(html).toContain("Trades Graded");

		// Promotion gates
		expect(html).toContain("Promotion Readiness");
		expect(html).toContain("criteria");
	});

	it("handles null view gracefully without crashing", () => {
		const html = renderToStaticMarkup(<LearningPerformanceBanner view={null} />);
		expect(html).toContain("PARALLEL LEARNERS (DECOUPLED)");
		expect(html).toContain("MAIN AGENT");
	});
});
