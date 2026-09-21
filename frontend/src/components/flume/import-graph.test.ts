import { describe, expect, it } from "vitest";
import { pipelineGraphCollection } from "#/collections/pipeline_graph";
import {
	convertJSONGraphToFlumeNodes,
	importJSONGraphToCollection,
} from "./import-graph";

describe("import-graph", () => {
	const sampleSystemGraph = {
		id: "system:orchestration",
		name: "system:orchestration",
		nodes: {
			source: {
				id: "source",
				type: "data.Source",
				connections: {
					inputs: {},
					outputs: {
						out: [{ nodeId: "signals", portName: "in" }],
					},
				},
			},
			signals: {
				id: "signals",
				type: "pipeline.Signals",
				connections: {
					inputs: {
						in: [{ nodeId: "source", portName: "out" }],
					},
					outputs: {
						out: [{ nodeId: "logic", portName: "in" }],
					},
				},
			},
			logic: {
				id: "logic",
				type: "pipeline.Logic",
				connections: {
					inputs: {
						in: [{ nodeId: "signals", portName: "out" }],
					},
					outputs: {
						out: [{ nodeId: "ui", portName: "in" }],
					},
				},
			},
			ui: {
				id: "ui",
				type: "transport.Broadcast",
				connections: {
					inputs: {
						in: [{ nodeId: "logic", portName: "out" }],
					},
					outputs: {
						out: [{ nodeId: "execution", portName: "in" }],
					},
				},
			},
			execution: {
				id: "execution",
				type: "pipeline.Execution",
				connections: {
					inputs: {
						in: [{ nodeId: "ui", portName: "out" }],
					},
					outputs: {
						out: [{ nodeId: "sink", portName: "in" }],
					},
				},
			},
			sink: {
				id: "sink",
				type: "data.Sink",
				connections: {
					inputs: {
						in: [{ nodeId: "execution", portName: "out" }],
					},
					outputs: {},
				},
			},
		},
	};

	it("converts declarative JSON graph to laid out Flume nodes", () => {
		const flumeNodes = convertJSONGraphToFlumeNodes(sampleSystemGraph);

		expect(Object.keys(flumeNodes)).toHaveLength(6);
		expect(flumeNodes.source).toBeDefined();
		expect(flumeNodes.signals).toBeDefined();
		expect(flumeNodes.logic).toBeDefined();
		expect(flumeNodes.ui).toBeDefined();
		expect(flumeNodes.execution).toBeDefined();
		expect(flumeNodes.sink).toBeDefined();

		// Check strictly increasing X coordinates along the pipeline flow
		expect(flumeNodes.source.x).toBeLessThan(flumeNodes.signals.x);
		expect(flumeNodes.signals.x).toBeLessThan(flumeNodes.logic.x);
		expect(flumeNodes.logic.x).toBeLessThan(flumeNodes.ui.x);
		expect(flumeNodes.ui.x).toBeLessThan(flumeNodes.execution.x);
		expect(flumeNodes.execution.x).toBeLessThan(flumeNodes.sink.x);
	});

	it("imports graph into pipelineGraphCollection and persists it", () => {
		const targetId = "test-system-graph";
		const nodes = importJSONGraphToCollection(sampleSystemGraph, targetId);

		expect(Object.keys(nodes)).toHaveLength(6);

		const row = pipelineGraphCollection.get(targetId);
		expect(row).toBeDefined();
		expect(row?.id).toBe(targetId);
		expect(Object.keys(row?.nodes ?? {})).toHaveLength(6);
	});
});
