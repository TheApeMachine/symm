// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { compileUI, type FlumeGraph } from "./compiler";
import type { OpenPosition } from "./position-list";
import { UIRouteView } from "./renderer";

afterEach(cleanup);

/*
A surface drawn from nodes still has to show live data.

Nothing in the library reaches for a store — that is what makes it portable — so
a graph names the stream it wants and the route hands it over. This proves the
name actually arrives as the rows, and that a callback named the same way is
actually called.
*/
const graph: FlumeGraph = {
	id: "sources-fixture",
	nodes: {
		positionSource: {
			type: "ui.UIData",
			inputData: { source: { value: "positions" } },
		},
		inspectSource: {
			type: "ui.UIData",
			inputData: { source: { value: "inspectSymbol" } },
		},
		route: {
			type: "ui.UIRoute",
			inputData: { path: { value: "/fixture" } },
			connections: {
				inputs: { components: [{ nodeId: "list", portName: "self" }] },
			},
		},
		list: {
			type: "ui.PositionList",
			connections: {
				inputs: {
					positions: [{ nodeId: "positionSource", portName: "value" }],
					onInspect: [{ nodeId: "inspectSource", portName: "value" }],
				},
			},
		},
	},
};

const lots: OpenPosition[] = [
	{
		symbol: "XBT/USD",
		status: "open",
		pnl: "+12.40",
		pnlValue: 12.4,
		entryPrice: "61000.00",
		mark: "61120.00",
		returnPct: "+0.20%",
	},
];

describe("a surface handed live data by name", () => {
	const compiled = compileUI(graph);

	it("compiles the source vocabulary without complaint", () => {
		expect(compiled.diagnostics).toEqual([]);
	});

	it("draws the rows the named source carries", () => {
		render(
			<UIRouteView
				route={compiled.routes[0]}
				sources={{ positions: lots, inspectSymbol: () => {} }}
			/>,
		);

		expect(screen.getByText("XBT/USD")).toBeTruthy();
		expect(screen.getByText("+12.40")).toBeTruthy();
	});

	it("says so plainly when the source carries nothing", () => {
		render(
			<UIRouteView
				route={compiled.routes[0]}
				sources={{ positions: [], inspectSymbol: () => {} }}
			/>,
		);

		expect(screen.getByText("no open positions")).toBeTruthy();
	});

	it("calls the behaviour the graph named", () => {
		const inspect = vi.fn();

		render(
			<UIRouteView
				route={compiled.routes[0]}
				sources={{ positions: lots, inspectSymbol: inspect }}
			/>,
		);

		fireEvent.click(screen.getByText("XBT/USD"));

		expect(inspect).toHaveBeenCalledWith("XBT/USD");
	});
});
