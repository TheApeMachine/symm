import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { LearningMarkT } from "#/providers/telemetry/telemetry/learning-mark";
import { LearningRehearsalT } from "#/providers/telemetry/telemetry/learning-rehearsal";
import { LearningStepT } from "#/providers/telemetry/telemetry/learning-step";
import { LearningTrackT } from "#/providers/telemetry/telemetry/learning-track";
import { learningFixture } from "./fixture";
import { RehearsalPanel } from "./rehearsal-panel";
import { projectLearning } from "./state";

describe("RehearsalPanel", () => {
	it("reports mounted fragments and archive residency without inventing episode classes", () => {
		const state = learningFixture();
		state.status = "recognising precursors";
		state.rehearsal = new LearningRehearsalT();
		Object.assign(state.rehearsal, {
			status: "recognising precursors",
			workers: 3,
			episodes: 12n,
			trained: 12n,
			profitable: 5n,
			declining: 4n,
			quiet: 3n,
			illiquid: 0n,
			runs: 2,
			observations: 4000n,
			budget: 500000n,
			warming: 180n,
			lastSymbol: "BTC/USD",
			lastAction: "enter",
		});
		const view = projectLearning(state, "");
		expect(view.rehearsal).toBe(state.rehearsal);
		const html = renderToStaticMarkup(<RehearsalPanel view={view} />);
		expect(html).toContain("12 mounted fragments · 3 learners");
		expect(html).toContain("2 runs read");
		expect(html).toContain("4,000 of 500,000 observations resident");
		expect(html).toContain("Impulse map formed after 180 frames");
		expect(html).toContain("showing BTC/USD · enter");
		// Each class is the count the producer reported, and a class the record
		// supplied nothing for reads as absent rather than as a measured zero.
		expect(html).toContain('aria-label="Price rises: 5 mounted"');
		expect(html).toContain('aria-label="Price falls: 4 mounted"');
		expect(html).toContain('aria-label="No price development: 3 mounted"');
		expect(html).toContain('aria-label="Never quoted: 0 mounted"');
		expect(html).toContain("absent");
	});

	it("says the map is still forming when it has not", () => {
		const state = learningFixture();
		state.status = "forming the impulse map";
		state.rehearsal = new LearningRehearsalT();
		Object.assign(state.rehearsal, {
			status: "forming the impulse map",
			workers: 2,
			episodes: 3n,
		});
		const html = renderToStaticMarkup(
			<RehearsalPanel view={projectLearning(state, "")} />,
		);
		expect(html).toContain("Impulse map still forming");
		expect(html).not.toContain("Latest:");
	});

	it("draws each worker's mounted tape, playhead and issued actions", () => {
		const state = learningFixture();
		state.rehearsal = new LearningRehearsalT();
		const track = new LearningTrackT();
		Object.assign(track, {
			id: 0,
			symbol: "BTC/USD",
			index: 2,
			length: 4,
			queued: 3,
			stride: 1,
			entry: 1,
			exit: 3,
			opportunity: "price rises",
			steps: [10, 12, 0, 11].map((value, index) => {
				const step = new LearningStepT();
				Object.assign(step, {
					atNs: BigInt(index),
					value,
					defined: index !== 2,
				});
				return step;
			}),
			marks: [
				Object.assign(new LearningMarkT(), {
					id: 1n,
					index: 1,
					kind: "enter",
					power: 1,
					reduce: false,
					value: 1,
					graded: true,
					verdict: "called ignition",
				}),
				Object.assign(new LearningMarkT(), {
					id: 2n,
					index: 3,
					kind: "exit",
					power: 0,
					reduce: true,
					value: -0.5,
					graded: true,
					verdict: "early for extremum",
				}),
			],
		});
		const idle = new LearningTrackT();
		Object.assign(idle, { id: 1 });
		Object.assign(state.rehearsal, { workers: 2, tracks: [track, idle] });
		const html = renderToStaticMarkup(
			<RehearsalPanel view={projectLearning(state, "")} />,
		);
		expect(html).toContain("Tape fragment worker 1 is replaying");
		expect(html).toContain("Observation 2 of 4");
		expect(html).toContain("Observation 2 of 4 · 3 fragments queued");
		expect(html).toContain('d="M 0 37.0 L 1 3.0 M 3 20.0"');
		expect(html).toContain('viewBox="0.56 0 2 40"');
		expect(html).toContain("left:72.00%");
		expect(html).toContain("left:22.00%;width:100.00%;background:var(--up)");
		expect(html).toContain("enter at observation 1 · called ignition · 1.00");
		expect(html).toContain(
			"exit at observation 3 · early for extremum · -0.50",
		);
		expect(html).toContain(
			"Ignition: the observation the excursion started at",
		);
		expect(html).toContain("Extremum: the observation the excursion ended at");
		expect(html).toContain("No fragment mounted");
		expect(html).not.toContain("Tape fragment worker 2 is replaying");
	});

	it("does not invent worker state before telemetry arrives", () => {
		const html = renderToStaticMarkup(<RehearsalPanel view={null} />);
		expect(html).toContain("No rehearsal frame has arrived");
		expect(html).not.toContain("0 learners");
	});
});
