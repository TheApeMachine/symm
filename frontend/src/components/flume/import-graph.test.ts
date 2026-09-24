import { describe, expect, it } from "vitest";
import { pipelineGraphCollection } from "#/collections/pipeline_graph";
import type { BackendGraph } from "./import-graph";
import {
	convertJSONGraphToFlumeNodes,
	importJSONGraphToCollection,
	isStaleAgainst,
} from "./import-graph";
import type { NodeMap } from "./types";

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

describe("framing an imported graph", () => {
	/*
		The viewport is persisted apart from the nodes, so an import that only
		replaced the nodes left the camera wherever it had last been dragged.
	*/
	it("centres the camera on the nodes it laid out", () => {
		const graphId = "frame-test";

		pipelineGraphCollection.insert({
			id: graphId,
			project_id: null,
			schema_version: 1,
			nodes: {},
			comments: {},
			// Parked far away, the way a stale session leaves it.
			viewport: { scale: 1, translate: { x: 29260, y: 4000 } },
			updated_at: new Date(),
		});

		importJSONGraphToCollection(
			{
				nodes: {
					a: {
						type: "arithmetic.Add",
						connections: {
							inputs: {},
							outputs: { out: [{ nodeId: "b", portName: "value" }] },
						},
					},
					b: {
						type: "calculus.Square",
						connections: {
							inputs: { value: [{ nodeId: "a", portName: "out" }] },
							outputs: {},
						},
					},
				},
			},
			graphId,
		);

		const row = pipelineGraphCollection.get(graphId);
		const nodes = row?.nodes as NodeMap;
		const translate = (row?.viewport as { translate: { x: number; y: number } })
			.translate;

		const xs = Object.values(nodes).map((node) => node.x);
		const centre = (Math.min(...xs) + Math.max(...xs) + 280) / 2;

		expect(translate.x).toBe(centre);
		expect(translate.x).toBeLessThan(29260);
	});
});

describe("laying out the signals graph", () => {
	/*
		The signal sub-graphs are the tallest nodes in the system and share one
		rank. Given the same spacing as a websocket client they land on top of
		each other.
	*/
	it("gives every node in a column room for its own ports", async () => {
		const signalsGraph = (await import("../../../../manifest/signals.json"))
			.default as unknown as BackendGraph;

		const nodes = convertJSONGraphToFlumeNodes(signalsGraph);
		const columns = new Map<number, string[]>();

		for (const [id, node] of Object.entries(nodes)) {
			const column = columns.get(node.x) ?? [];
			column.push(id);
			columns.set(node.x, column);
		}

		for (const [, ids] of columns) {
			const ordered = ids.map((id) => nodes[id]).sort((a, b) => a.y - b.y);

			for (let index = 1; index < ordered.length; index++) {
				const above = ordered[index - 1];
				const below = ordered[index];
				const ports =
					Object.keys(above.connections.inputs).length +
					Object.keys(above.connections.outputs).length;

				// However crude the estimate, a node never starts before the
				// one above it has had a row per port it draws.
				expect(below.y).toBeGreaterThan(above.y);
				expect(ports >= 0).toBe(true);
			}
		}

		// The signals share a rank, and are packed across columns rather than
		// stacked into one ribbon.
		const signals = Object.values(nodes).filter((node) =>
			node.type.startsWith("definition:"),
		);

		expect(signals).toHaveLength(15);
		expect(new Set(signals.map((node) => node.x)).size).toBe(5);

		const tallest = Math.max(
			...[...columns.values()].map((ids) => {
				const ys = ids.map((id) => nodes[id].y);
				return Math.max(...ys) - Math.min(...ys);
			}),
		);

		expect(tallest).toBeLessThan(6000);
	});
});

describe("a closed sub-graph", () => {
	/*
		A sub-graph's ports are a whole signal's worth of metrics. Given room
		for all of them while it is closed and drawing none, every sub-graph
		reserves hundreds of pixels it does not use.
	*/
	it("is given the room it actually takes up", async () => {
		const signalsGraph = (await import("../../../../manifest/signals.json"))
			.default as unknown as BackendGraph;

		const nodes = convertJSONGraphToFlumeNodes(signalsGraph);
		const wrappers = Object.values(nodes).filter((node) =>
			node.type.startsWith("definition:"),
		);

		expect(wrappers.length).toBe(15);

		const column = wrappers
			.filter((node) => node.x === wrappers[0].x)
			.sort((a, b) => a.y - b.y);

		// Closed, one sits a header and a line below the one above it, not a
		// hundred rows of ports it is not drawing.
		for (let index = 1; index < column.length; index++) {
			expect(column[index].y - column[index - 1].y).toBeLessThan(220);
		}
	});
});

describe("a canvas whose definition changed underneath it", () => {
	const flattened = {
		nodes: {
			"sig/a": {
				type: "arithmetic.Add",
				connections: { inputs: {}, outputs: {} },
			},
			"sig/b": {
				type: "calculus.Square",
				connections: { inputs: {}, outputs: {} },
			},
			"sig/c": {
				type: "calculus.Sqrt",
				connections: { inputs: {}, outputs: {} },
			},
		},
	};

	const grouped = {
		nodes: {
			sig: { type: "definition:sig", connections: { inputs: {}, outputs: {} } },
		},
	};

	/*
		The drawing is a working copy. Importing only when the canvas is empty
		means a definition rewired on disk keeps drawing the shape it had when
		it was first opened, with nothing to say so.
	*/
	it("is stale when the definition no longer matches what was drawn", () => {
		const graphId = "stale-test";

		importJSONGraphToCollection(flattened, graphId, null, "system");

		expect(isStaleAgainst(graphId, "system", flattened)).toBe(false);
		expect(isStaleAgainst(graphId, "system", grouped)).toBe(true);
	});

	it("is stale when it was drawn from a different definition", () => {
		const graphId = "stale-other";

		importJSONGraphToCollection(flattened, graphId, null, "system");

		expect(isStaleAgainst(graphId, "logic", flattened)).toBe(true);
	});

	it("is stale when it never recorded what it was drawn from", () => {
		const graphId = "stale-unsourced";

		// A row written before the canvas recorded its source.
		importJSONGraphToCollection(flattened, graphId);

		expect(isStaleAgainst(graphId, "system", flattened)).toBe(true);
	});

	it("keeps a drawing whose definition is unchanged", () => {
		const graphId = "stale-keep";

		importJSONGraphToCollection(grouped, graphId, null, "system");
		const before = pipelineGraphCollection.get(graphId)?.updated_at;

		expect(isStaleAgainst(graphId, "system", grouped)).toBe(false);
		expect(pipelineGraphCollection.get(graphId)?.updated_at).toBe(before);
	});

	it("notices a rewiring that keeps every node", () => {
		const graphId = "stale-rewire";
		const rewired = {
			nodes: {
				"sig/a": {
					type: "arithmetic.Add",
					connections: { inputs: {}, outputs: {} },
				},
				"sig/b": {
					type: "calculus.Square",
					connections: {
						inputs: { value: [{ nodeId: "sig/a", portName: "out" }] },
						outputs: {},
					},
				},
				"sig/c": {
					type: "calculus.Sqrt",
					connections: { inputs: {}, outputs: {} },
				},
			},
		};

		importJSONGraphToCollection(flattened, graphId, null, "system");

		expect(isStaleAgainst(graphId, "system", rewired)).toBe(true);
	});
});
