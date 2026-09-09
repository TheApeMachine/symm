import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { learningFixture } from "./fixture";
import type { Candidate, LearningEvent, LearningView, Skill } from "./state";
import { projectLearning } from "./state";
import {
	ActionSpectrumPlot,
	EdgeDistributionPlot,
	LearningTrajectoryPlot,
	LearningVisualizer,
} from "./visualizer";

describe("EdgeDistributionPlot", () => {
	it("renders the recovered normal curve using measured outcome moments", () => {
		const skill: Skill = {
			mode: "learning",
			account: "sim",
			since: "2026-09-05T12:00:00Z",
			reason: "calibrating",
			samples: 1000,
			support: 150.5,
			defined: true,
			varianceDefined: true,
			mean: -0.00074,
			variance: 0.000002,
			wins: 450,
			losses: 550,
		};

		const markup = renderToStaticMarkup(<EdgeDistributionPlot skill={skill} />);

		expect(markup).toContain("Mean -7.4 bp");
		expect(markup).toContain("450 positive");
		expect(markup).toContain("550 negative");
		expect(markup).toContain("Observed SD 14.1 bp");
		expect(markup).toContain("0.0 bp (Breakeven)");
		expect(markup).not.toContain("Confidence:");
		expect(markup).not.toContain("live gate");
		expect(markup).toContain('data-edge-curve="normal-fit"');
		for (const mean of [-0.00074, 0, 0.00074]) {
			const fitted = renderToStaticMarkup(
				<EdgeDistributionPlot skill={{ ...skill, mean }} />,
			);
			const curve = fitted.match(/data-edge-curve="normal-fit" d="([^"]+)"/);
			expect(curve).not.toBeNull();
			const points = [...curve![1].matchAll(/[ML] ([\d.]+) ([\d.]+)/g)].map(
				(point) => ({ x: Number(point[1]), y: Number(point[2]) }),
			);
			const peak = points.reduce((highest, point) =>
				point.y < highest.y ? point : highest,
			);
			expect(peak.y).toBeLessThan(points[0].y);
			expect(peak.y).toBeLessThan(points.at(-1)!.y);
			// The retained symmetric axis places breakeven at SVG x=265.
			expect(Math.sign(peak.x - 265)).toBe(Math.sign(mean));
		}
		for (const mean of [-0.00074, 0, 0.00074]) {
			const varied = renderToStaticMarkup(
				<EdgeDistributionPlot skill={{ ...skill, mean, variance: 0 }} />,
			);
			expect(varied).toContain("Observed SD 0.0 bp");
			expect(varied).not.toMatch(/NaN|Infinity/);
			expect(varied).not.toContain('data-edge-curve="normal-fit"');
			expect(varied).toContain("Zero observed spread");
		}
		const immature = renderToStaticMarkup(
			<EdgeDistributionPlot skill={{ ...skill, varianceDefined: false }} />,
		);
		expect(immature).toContain("Spread not yet measured");
		expect(immature).not.toContain('data-edge-curve="normal-fit"');
		const empty = renderToStaticMarkup(<EdgeDistributionPlot />);
		expect(empty).toContain("No completed tape outcomes yet.");
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
	it("renders recorded policy profit without summing overlapping rate targets", () => {
		const events: LearningEvent[] = [
			{
				id: 1,
				lane: 0,
				mode: "policy",
				kind: "valued",
				at: "2026-09-05T12:01:00Z",
				action: "buy",
				power: 0,
				reduce: false,
				cash: "1000",
				inventory: "1",
				authority: 0.8,
				profit: -20,
				target: 0.00025,
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
			<LearningTrajectoryPlot
				events={[
					{ ...events[0], id: 2, at: "2026-09-05T12:02:00Z", profit: -10 },
					...events,
					{ ...events[0], mode: "virtual", profit: 100 },
				]}
				initialCapital="200"
			/>,
		);

		expect(markup).toContain("Consolidated wallet profit");
		expect(markup).toContain("2 valuations");
		expect(markup).toContain("-500.0");
		expect(markup).not.toContain("+5.0 bp net");
	});
});

describe("LearningTrajectoryPlot unavailable inputs", () => {
	it("does not invent a zero return when no valuations exist", () => {
		const markup = renderToStaticMarkup(
			<LearningTrajectoryPlot events={[]} initialCapital="200" />,
		);
		expect(markup).toContain("trajectory unavailable");
		expect(markup).not.toContain("bp net");
	});
});

describe("LearningVisualizer", () => {
	it("renders the visualizer frame with tabs and footer stats", () => {
		const view: LearningView = {
			...projectLearning(learningFixture(), ""),
			skill: {
				mode: "learning",
				account: "sim",
				since: "2026-09-05T12:00:00Z",
				reason: "calibrating",
				samples: 100,
				support: 20,
				defined: true,
				varianceDefined: true,
				mean: 0.0003,
				variance: 0.000001,
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
			<LearningVisualizer view={view} events={[]} />,
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
