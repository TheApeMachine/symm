import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { LearningPerformanceBanner } from "./performance-banner";

describe("LearningPerformanceBanner", () => {
	it("renders precursor cognition, staged learning, and held-out/forward sections with data-metric attributes", () => {
		const html = renderToStaticMarkup(<LearningPerformanceBanner />);

		// Architecture & Stages
		expect(html).toContain("PRECURSOR COGNITION MODEL");
		expect(html).toContain("STAGED LEARNING");
		expect(html).toContain('data-l="training-stage"');
		expect(html).toContain('data-l="stage-blocker"');
		expect(html).toContain('data-l="gate-count"');

		// Pillar 1: Precursor model & tape fragments
		expect(html).toContain("PRECURSOR COGNITION");
		expect(html).toContain("Learned Situations");
		expect(html).toContain('data-metric="decisions"');
		expect(html).toContain('data-metric="steps"');
		expect(html).toContain('data-metric="confidence"');
		expect(html).toContain('data-metric="contrast"');
		expect(html).toContain('data-metric="precursor_length"');
		expect(html).toContain('data-metric="fragments_up"');
		expect(html).toContain('data-metric="fragments_up_friction"');
		expect(html).toContain('data-metric="fragments_down"');
		expect(html).toContain('data-metric="fragments_chop"');
		expect(html).toContain('data-metric="fragments_flat"');
		expect(html).toContain('data-metric="fragments_unsupported"');

		// Pillar 2: Held-out & forward paper metrics
		expect(html).toContain("HELD-OUT SKILL AND FORWARD PAPER");
		expect(html).toContain('data-metric="hist_mean_return"');
		expect(html).toContain('data-metric="hist_lower_bound"');
		expect(html).toContain('data-metric="hist_opportunities"');
		expect(html).toContain('data-metric="hist_correct_enter"');
		expect(html).toContain('data-metric="hist_false_enter"');
		expect(html).toContain('data-metric="fwd_paper_mean_return"');
		expect(html).toContain('data-metric="fwd_paper_trades"');
		expect(html).toContain('data-metric="fwd_enter_predictions"');
	});
});
