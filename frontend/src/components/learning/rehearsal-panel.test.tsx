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
	it("shows actual pool imbalance, selected counts and separate live results", () => {
		const state = learningFixture();
		state.rehearsal = new LearningRehearsalT();
		Object.assign(state.rehearsal, {
			status: "replaying",
			workers: 3,
			profitable: 12n,
			subfriction: 2n,
			declining: 5n,
			perWorker: 6n,
			decisions: 11n,
			trained: 10n,
			lastSymbol: "BTC/USD",
			lastAction: "enter",
			lastReturn: -0.02,
			lastFailure: "entry too early",
		});
		const view = projectLearning(state, "");
		expect(view.rehearsal).toBe(state.rehearsal);
		const html = renderToStaticMarkup(<RehearsalPanel view={view} />);
		expect(html).toContain("12 available, 2 selected per worker");
		expect(html).toContain("5 available, 2 selected per worker");
		expect(html).toContain("3 historical workers");
	});

	it("keeps missing classes and unavailable economics visible", () => {
		const state = learningFixture();
		state.rehearsal = new LearningRehearsalT();
		Object.assign(state.rehearsal, {
			status: "waiting for gradeable episodes",
			workers: 2,
			ungraded: 7n,
		});
		const html = renderToStaticMarkup(
			<RehearsalPanel view={projectLearning(state, "")} />,
		);
		expect(html).toContain("Incomplete variety");
		expect(html).toContain("7 excluded");
		expect(html).toContain("0 available, 0 selected per worker");
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
					value: 0.01,
					graded: true,
				}),
				Object.assign(new LearningMarkT(), {
					id: 2n,
					index: 3,
					kind: "exit",
					power: 0,
					reduce: true,
					graded: false,
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
		// The undefined third observation breaks the line rather than crossing zero.
		expect(html).toContain('d="M 0.0 37.0 L 333.3 3.0 M 1000.0 20.0"');
		expect(html).toContain("enter ·1/2 at observation 1 · 100.0 bp");
		expect(html).toContain("exit ↓ ·1/1 at observation 3 · not graded yet");
		expect(html).toContain("No fragment mounted");
		expect(html).not.toContain("Tape fragment worker 2 is replaying");
	});

	it("does not invent worker state before telemetry arrives", () => {
		const html = renderToStaticMarkup(<RehearsalPanel view={null} />);
		expect(html).toContain("Historical workers have not reported");
		expect(html).not.toContain("0 historical workers");
	});
});
