import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { researchGraphCollection } from "#/collections/research_graph";
import { buildFlumeConfigFromSchemas } from "./build-config-from-schemas";
import { useNodesState } from "./useNodesState";

const clearCollection = () => {
	if (typeof window !== "undefined") {
		window.localStorage.removeItem("caramba:research_graphs");
	}

	for (const row of Array.from(researchGraphCollection.values())) {
		try {
			researchGraphCollection.delete(row.id);
		} catch {
			// ignore
		}
	}
};

const renderUseNodesState = (graphId: string) => {
	const { nodeTypes, portTypes } = buildFlumeConfigFromSchemas({});
	const getEnvironment = () => ({
		nodeTypes,
		portTypes,
		context: {},
	});

	return renderHook(() =>
		useNodesState({
			graphId,
			projectId: null,
			nodeTypes,
			portTypes,
			context: {},
			getEnvironment,
		}),
	);
};

describe("useNodesState", () => {
	beforeEach(() => {
		clearCollection();
	});

	afterEach(() => {
		clearCollection();
	});

	it("starts with an empty node map when no row exists", () => {
		const { result } = renderUseNodesState("graph-a");

		expect(result.current.nodes).toEqual({});
		expect(result.current.hasRow).toBe(false);
	});

	it("seed() inserts a row from defaults; second call is idempotent", () => {
		const { result } = renderUseNodesState("graph-b");

		act(() => {
			result.current.seed({
				defaultNodes: [
					{ type: "source", x: 0, y: 0 },
					{ type: "sink", x: 200, y: 0 },
				],
				defaultConnections: [
					{
						output: { nodeType: "source", portName: "value" },
						input: { nodeType: "sink", portName: "value" },
					},
				],
			});
		});

		const firstRow = researchGraphCollection.get("graph-b");
		expect(firstRow).toBeDefined();
		expect(Object.keys(firstRow?.nodes as object)).toHaveLength(2);

		act(() => {
			result.current.seed({
				defaultNodes: [{ type: "source", x: 999, y: 999 }],
			});
		});

		const secondRow = researchGraphCollection.get("graph-b");
		// Should match the first row exactly — second seed is a no-op.
		expect(Object.keys(secondRow?.nodes as object)).toHaveLength(2);
	});

	it("reconcileNodeTypes is a no-op when no collection row exists", () => {
		const { result } = renderUseNodesState("graph-missing");

		expect(() => {
			act(() => {
				result.current.actions.reconcileNodeTypes();
			});
		}).not.toThrow();

		expect(researchGraphCollection.get("graph-missing")).toBeUndefined();
	});

	it("setNodeCoordinates reconciles the draft so actions see a normalized shape", () => {
		// Pre-seed a stale row: a node missing the connections field that
		// the actions expect. reconcileNodes-on-write should fill it in.
		researchGraphCollection.insert({
			id: "graph-c",
			project_id: null,
			schema_version: 1,
			nodes: {
				source: {
					id: "source",
					type: "source",
					x: 0,
					y: 0,
					width: 200,
					height: 80,
					inputData: {},
					// connections intentionally omitted
				},
			},
			comments: {},
			viewport: { scale: 1, translate: { x: 0, y: 0 } },
			updated_at: new Date(),
		});

		const { result } = renderUseNodesState("graph-c");

		act(() => {
			result.current.actions.setNodeCoordinates({
				nodeId: "source",
				x: 100,
				y: 100,
			});
		});

		const row = researchGraphCollection.get("graph-c");
		const node = (
			row?.nodes as Record<
				string,
				{ x: number; y: number; connections?: unknown }
			>
		)?.source;
		expect(node?.x).toBe(100);
		expect(node?.y).toBe(100);
		expect(node?.connections).toBeDefined();
	});
});
