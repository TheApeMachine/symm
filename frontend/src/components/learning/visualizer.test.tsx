import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Candidate, LearningEvent, LearningView, Skill } from "./state";
import {
	ActionSpectrumPlot,
	EdgeDistributionPlot,
	LearningTrajectoryPlot,
	LearningVisualizer,
} from "./visualizer";

describe("EdgeDistributionPlot", () => {
	it("renders normal PDF distribution and lower bound for defined skill", () => {
		const skill: Skill = {
			mode: "learning",
			account: "sim",
			since: "2026-09-05T12:00:00Z",
			reason: "calibrating",
			samples: 1000,
			support: 150.5,
			defined: true,
			varianceDefined: true,
			qualified: false,
			mean: -0.00074,
			variance: 0.000002,
			standardError: 0.00014,
			lowerBound: -0.00116,
			confidence: 0.02,
			sigma: 3.0,
			memory: 50,
			promotions: 0,
			demotions: 0,
			wins: 450,
			losses: 550,
		};

		const markup = renderToStaticMarkup(<EdgeDistributionPlot skill={skill} />);

		expect(markup).toContain("0.0 bp (Breakeven)");
		expect(markup).toContain("3σ Bound (-11.6 bp)");
		expect(markup).toContain("μ -7.4 bp");
		expect(markup).toContain("450W");
		expect(markup).toContain("550L");
		expect(markup).toContain("CALIBRATING");
	});
});

describe("ActionSpectrumPlot", () => {
	it("renders candidate action spectrum with highlighted policy choice", () => {
		const candidates: Candidate[] = [
			{
				kind: "buy",
				power: 0,
				reduce: false,
				selected: true,
				prior: {
					Samples: 42,
					Defined: true,
					Mean: 0.0005,
					Variance: 0.00001,
					VarianceDefined: true,
					Support: 12.5,
					Maturity: 0.8,
					Authority: 0.9,
				},
			},
			{
				kind: "sell",
				power: 1,
				reduce: false,
				selected: false,
				prior: {
					Samples: 20,
					Defined: true,
					Mean: -0.0003,
					Variance: 0.00001,
					VarianceDefined: true,
					Support: 8.2,
					Maturity: 0.6,
					Authority: 0.7,
				},
			},
		];

		const markup = renderToStaticMarkup(
			<ActionSpectrumPlot candidates={candidates} />,
		);

		expect(markup).toContain("buy ·1/1");
		expect(markup).toContain("choice");
		expect(markup).toContain("sell ·1/2");
		expect(markup).toContain("Negative return expectation");
		expect(markup).toContain("Positive return expectation");
	});
});

describe("LearningTrajectoryPlot", () => {
	it("renders cumulative return trajectory from resolved events", () => {
		const events: LearningEvent[] = [
			{
				id: 1,
				lane: 0,
				mode: "learning",
				kind: "resolved",
				at: "2026-09-05T12:01:00Z",
				action: "buy",
				power: 0,
				reduce: false,
				cash: "1000",
				inventory: "1",
				authority: 0.8,
				profit: 2.5,
				target: 0.00025,
				complete: true,
				episode: 1,
				horizonNs: 100000000,
				prior: {
					Samples: 1,
					Defined: true,
					Mean: 0.00025,
					Variance: 0,
					VarianceDefined: false,
					Support: 1,
					Maturity: 0.1,
					Authority: 0.8,
				},
			},
		];

		const markup = renderToStaticMarkup(
			<LearningTrajectoryPlot events={events} />,
		);

		expect(markup).toContain("Cumulative outcome trajectory");
		expect(markup).toContain("resolved windows");
		expect(markup).toContain("+2.5 bp net");
	});
});

describe("LearningVisualizer", () => {
	it("renders the visualizer frame with tabs and footer stats", () => {
		const view: Partial<LearningView> = {
			skill: {
				mode: "learning",
				account: "sim",
				since: "2026-09-05T12:00:00Z",
				reason: "calibrating",
				samples: 100,
				support: 20,
				defined: true,
				varianceDefined: true,
				qualified: false,
				mean: 0.0003,
				variance: 0.000001,
				standardError: 0.00005,
				lowerBound: 0.00015,
				confidence: 0.95,
				sigma: 3.0,
				memory: 50,
				promotions: 0,
				demotions: 0,
				wins: 60,
				losses: 40,
			},
			candidates: [
				{
					kind: "buy",
					power: 0,
					reduce: false,
					selected: true,
					prior: {
						Samples: 10,
						Defined: true,
						Mean: 0.0003,
						Variance: 0.000001,
						VarianceDefined: true,
						Support: 5,
						Maturity: 0.5,
						Authority: 0.8,
					},
				},
			],
		};

		const markup = renderToStaticMarkup(
			<LearningVisualizer view={view as LearningView} events={[]} />,
		);

		expect(markup).toContain("Learning visualizer");
		expect(markup).toContain("Edge distribution");
		expect(markup).toContain("Action spectrum");
		expect(markup).toContain("Trajectory");
		expect(markup).toContain("pointer-events-auto");
		expect(markup).toContain("3.0 bp");
		expect(markup).toContain("buy ·1/1");
	});
});
