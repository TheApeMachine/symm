// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { EvidenceGraph } from "./evidence-graph";
import type { Graph } from "./evidence-graph.types";
import { buildScene, drawEvidenceGraph } from "./evidence-graph-viz";

vi.mock("./evidence-graph-viz", async (original) => ({
	...(await original<typeof import("./evidence-graph-viz")>()),
	drawEvidenceGraph: vi.fn(),
}));

const graph: Graph = {
	symbol: "TEST",
	at: "2026-09-22T12:00:00Z",
	nodes: [
		{
			key: "category/flow",
			kind: "category",
			category: "flow",
			measurement: { source: "category", metric: "flow" },
		},
	],
	edges: [],
};

afterEach(() => {
	cleanup();
	vi.restoreAllMocks();
	vi.unstubAllGlobals();
});

describe("EvidenceGraph", () => {
	it("redraws supplied data and resize events, exposes picking, and releases observation", () => {
		let resized = () => {};
		const disconnect = vi.fn();
		vi.stubGlobal(
			"ResizeObserver",
			class {
				constructor(callback: () => void) {
					resized = callback;
				}
				observe = vi.fn();
				disconnect = disconnect;
			},
		);
		vi.spyOn(HTMLCanvasElement.prototype, "clientWidth", "get").mockReturnValue(
			640,
		);
		vi.spyOn(
			HTMLCanvasElement.prototype,
			"clientHeight",
			"get",
		).mockReturnValue(400);
		vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
			setTransform: vi.fn(),
		} as unknown as CanvasRenderingContext2D);
		const { rerender, unmount } = render(<EvidenceGraph graph={graph} />);
		expect(drawEvidenceGraph).toHaveBeenLastCalledWith(
			expect.anything(),
			640,
			400,
			graph,
			expect.anything(),
			undefined,
		);
		const position = buildScene(graph, 640, 400).positions.get(
			graph.nodes[0].key,
		)!;
		fireEvent.mouseMove(screen.getByLabelText("Evidence graph"), {
			clientX: position.x,
			clientY: position.y,
		});
		expect(screen.getByText("flow")).toBeDefined();
		resized();
		expect(drawEvidenceGraph).toHaveBeenLastCalledWith(
			expect.anything(),
			640,
			400,
			graph,
			expect.anything(),
			graph.nodes[0].key,
		);
		rerender(<EvidenceGraph />);
		expect(screen.queryByText("flow")).toBeNull();
		expect(drawEvidenceGraph).toHaveBeenLastCalledWith(
			expect.anything(),
			640,
			400,
			null,
			undefined,
			undefined,
		);
		unmount();
		expect(disconnect).toHaveBeenCalled();
	});
});
