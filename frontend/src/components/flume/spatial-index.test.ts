import { describe, expect, it } from "vitest";
import {
	createSpatialIndexSnapshot,
	portLayoutKey,
	resolveConnectionsFromSpatialIndex,
} from "./spatial-index";
import type { NodeMap } from "./types";

const snapshotWith = (ports: Array<[string, string, "input" | "output"]>) => {
	const snapshot = createSpatialIndexSnapshot();

	for (const [nodeId, portName, transput] of ports) {
		snapshot.nodeLayouts.set(nodeId, { width: 280, height: 200 });
		snapshot.portLayouts.set(portLayoutKey(nodeId, portName, transput), {
			offsetX: 0,
			offsetY: 10,
		});
	}

	return snapshot;
};

describe("resolveConnectionsFromSpatialIndex", () => {
	const nodes = {
		grid: {
			id: "grid",
			type: "store.Grid",
			x: 0,
			y: 0,
			width: 280,
			inputData: {},
			connections: {
				inputs: { metrics_7: [{ nodeId: "signal", portName: "out" }] },
				outputs: {},
			},
		},
		signal: {
			id: "signal",
			type: "definition:cvd_trade",
			x: 400,
			y: 0,
			width: 280,
			inputData: {},
			connections: { inputs: {}, outputs: {} },
		},
	} as unknown as NodeMap;

	it("draws an edge whose ports were drawn under their own names", () => {
		const snapshot = snapshotWith([
			["grid", "metrics_7", "input"],
			["signal", "out", "output"],
		]);

		expect(resolveConnectionsFromSpatialIndex(nodes, snapshot)).toHaveLength(1);
	});

	/*
		The grid gathers four hundred metrics on one port, drawn as one row.
		Only that row is measured, so an edge wired to a slot has to anchor on
		the port the slot belongs to or it is silently dropped.
	*/
	it("anchors an edge on the collapsed port its slot belongs to", () => {
		const snapshot = snapshotWith([
			["grid", "metrics", "input"],
			["signal", "out", "output"],
		]);

		expect(resolveConnectionsFromSpatialIndex(nodes, snapshot)).toHaveLength(1);
	});

	it("anchors the producing end on its family too", () => {
		const fanOut = {
			...nodes,
			grid: {
				...nodes.grid,
				connections: {
					inputs: { metrics: [{ nodeId: "signal", portName: "values_5" }] },
					outputs: {},
				},
			},
		} as unknown as NodeMap;

		const snapshot = snapshotWith([
			["grid", "metrics", "input"],
			["signal", "values", "output"],
		]);

		expect(resolveConnectionsFromSpatialIndex(fanOut, snapshot)).toHaveLength(
			1,
		);
	});

	/*
		A closed sub-graph draws no ports. Its edges still have to reach it,
		so they meet the node at the side they arrive on.
	*/
	it("meets a node that drew no ports at its own side", () => {
		const snapshot = createSpatialIndexSnapshot();
		snapshot.nodeLayouts.set("grid", { width: 280, height: 200 });
		snapshot.nodeLayouts.set("signal", { width: 280, height: 200 });

		const [edge] = resolveConnectionsFromSpatialIndex(nodes, snapshot);

		expect(edge).toBeDefined();
		// Leaving the producer's right side, arriving at the consumer's left.
		expect(edge.from.x).toBe(400 + 280);
		expect(edge.to.x).toBe(0);
	});

	it("drops an edge when the node itself was never measured", () => {
		const snapshot = createSpatialIndexSnapshot();

		expect(resolveConnectionsFromSpatialIndex(nodes, snapshot)).toHaveLength(0);
	});
});
