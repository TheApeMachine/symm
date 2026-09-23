// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
	clearGraphResults,
	setGraphResults,
} from "#/components/flume/graph-results.store";
import { createFlumeConfig } from "#/components/flume/flume-config.generated";

const fixture = vi.hoisted(() => ({
	nodes: {
		forward: {
			id: "forward",
			type: "ui.ForwardView",
			connections: {
				inputs: {
					episode: [{ nodeId: "producer", portName: "episode" }],
					summary: [{ nodeId: "producer", portName: "summary" }],
					branches: [{ nodeId: "producer", portName: "branches" }],
					outcomeBins: [{ nodeId: "producer", portName: "bins" }],
				},
			},
		},
		recognition: {
			id: "recognition",
			type: "ui.RecognitionView",
			connections: {
				inputs: {
					metricMap: [{ nodeId: "producer", portName: "metrics" }],
					recentActivity: [{ nodeId: "producer", portName: "activity" }],
				},
			},
		},
		trie: {
			id: "trie",
			type: "ui.TrieView",
			connections: {
				inputs: { root: [{ nodeId: "producer", portName: "trie" }] },
			},
		},
		producer: { id: "producer", type: "fixture.RecordedResults" },
	},
}));
// Only the fetch is substituted; this exercises the real surface component,
// graph compiler, generated registry, result store, and the extracted
// visualizations the graph names.
vi.mock("#/service/compute", () => ({
	fetchDefinition: async () => fixture,
	fetchDefinitions: async () => ["local-default"],
}));
import { GraphSurface } from "#/components/surface/graph-surface";
afterEach(() => {
	cleanup();
	clearGraphResults("local-default");
});

describe("a graph drawn as a surface", () => {
	it("renders learning views through graph bindings and clears old results", async () => {
		const config = createFlumeConfig();
		for (const name of [
			"ForwardView",
			"EpisodeTape",
			"PolicyBranches",
			"OutcomeDistribution",
			"RecognitionView",
			"TrieView",
		]) {
			expect(config.nodeTypes[`ui.${name}`]).toBeDefined();
		}
		const version = JSON.stringify(fixture.nodes);
		setGraphResults("local-default", version, {
			producer: {
				summary: { edge: 0.001, accuracy: 0, decisions: "9007199254740995" },
				episode: {
					id: "recorded",
					label: "Recorded episode",
					points: [
						{ x: 0, y: 101 },
						{ x: 1, y: 103 },
					],
				},
				branches: [
					{
						id: "path",
						signature: "Observed path",
						depth: 2,
						visits: "9007199254740993",
						confidence: 0,
						policy: "WAIT",
					},
				],
				bins: [{ id: "bin", lower: -1, upper: 2, count: 3 }],
				metrics: { accuracy: 0, input_count: 7 },
				activity: [
					{
						id: "recorded-event",
						time: "10:00",
						actionStr: "Observed EXIT",
						edgeStr: "0 bp",
					},
				],
				trie: { id: "root", prefix: "Observed root", probability: 1 },
			},
		});
		render(<GraphSurface name="local-default" />);
		expect((await screen.findAllByText("Recorded episode"))[0]).toBeDefined();
		expect(screen.getByText("9007199254740995")).toBeDefined();
		expect(screen.getByText("Observed path")).toBeDefined();
		expect(screen.getByText("9007199254740993")).toBeDefined();
		expect(screen.getByText("Observed EXIT")).toBeDefined();
		expect(screen.getByText("Observed root")).toBeDefined();
		expect(screen.queryByText(/Compilation Warnings/)).toBeNull();
		act(() => setGraphResults("local-default", version, {}));
		expect(screen.getByText("No episode observations")).toBeDefined();
		expect(screen.getByText("No recorded activity")).toBeDefined();
		expect(screen.getByText("No recorded branches")).toBeDefined();
		await waitFor(() => expect(screen.queryByText("Observed root")).toBeNull());
	});
});
