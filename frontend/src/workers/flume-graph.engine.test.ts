import { describe, expect, it } from "vitest";
import type { FlumeNode } from "#/components/flume/types";
import { FlumeGraphEngine } from "#/workers/flume-graph.engine";

const sourceNode: FlumeNode = {
	id: "source-1",
	type: "source",
	width: 200,
	height: 80,
	x: -200,
	y: 80,
	inputData: {},
	connections: {
		inputs: {},
		outputs: {
			value: [{ nodeId: "sink-1", portName: "value" }],
		},
	},
};

const sinkNode: FlumeNode = {
	id: "sink-1",
	type: "sink",
	width: 200,
	height: 80,
	x: 200,
	y: 80,
	inputData: {},
	connections: {
		inputs: {
			value: [{ nodeId: "source-1", portName: "value" }],
		},
		outputs: {},
	},
};

const seedTwoNode = (
	engine: FlumeGraphEngine,
	routing: "straight" | "orthogonal",
) => {
	engine.setGraph({
		"source-1": sourceNode,
		"sink-1": sinkNode,
	});
	engine.setRoutingMode(routing);
	engine.setNodeLayout("source-1", 200, 80);
	engine.setNodeLayout("sink-1", 200, 80);
	engine.setPortLayout("source-1", "value", "output", 200, 40);
	engine.setPortLayout("sink-1", "value", "input", 0, 40);
};

describe("FlumeGraphEngine", () => {
	it("computes edge paths from graph + port offsets", () => {
		const engine = new FlumeGraphEngine();
		seedTwoNode(engine, "straight");

		const paths = engine.computePaths();

		expect(paths).toHaveLength(1);
		expect(paths[0]?.d).toMatch(/^M /);
	});

	it("updates paths while dragging a single node", () => {
		const engine = new FlumeGraphEngine();
		seedTwoNode(engine, "straight");

		const beforeDrag = engine.computePaths();
		const duringDrag = engine.updateDrag("source-1", 220, 160);
		const afterDrag = engine.endDrag("source-1", 220, 160);

		expect(beforeDrag[0]?.d).not.toEqual(duringDrag.paths[0]?.d);
		expect(duringDrag.paths[0]?.d).toEqual(afterDrag[0]?.d);
		expect(engine.getSnapshot().nodes["source-1"]?.x).toBe(220);
		expect(engine.getSnapshot().nodes["source-1"]?.y).toBe(160);
	});

	it("routes orthogonal edges through the occupancy grid", () => {
		const engine = new FlumeGraphEngine();
		engine.setGraph({
			"source-1": sourceNode,
			"sink-1": sinkNode,
			"block-1": {
				id: "block-1",
				type: "gate",
				width: 120,
				height: 120,
				x: 280,
				y: 80,
				inputData: {},
				connections: { inputs: {}, outputs: {} },
			},
		});
		engine.setRoutingMode("orthogonal");
		engine.setNodeLayout("source-1", 200, 80);
		engine.setNodeLayout("sink-1", 200, 80);
		engine.setNodeLayout("block-1", 120, 120);
		engine.setPortLayout("source-1", "value", "output", 200, 40);
		engine.setPortLayout("sink-1", "value", "input", 0, 40);

		const paths = engine.computePaths();

		expect(paths).toHaveLength(1);
		expect(paths[0]?.d).toMatch(/^M .* L .* L /);
	});
});
