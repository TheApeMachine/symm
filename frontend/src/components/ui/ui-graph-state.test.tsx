// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { compileUI, type FlumeGraph } from "./compiler";
import { UIRouteView } from "./renderer";

afterEach(cleanup);

/*
A surface drawn from nodes still has to answer a click.

The graph below is the whole vocabulary: one state node saying what choice the
surface starts on, two nodes that make a choice, and two that appear under one.
Nothing in it names a component that knows about tabs.
*/
const graph: FlumeGraph = {
	id: "state-fixture",
	nodes: {
		choice: {
			type: "ui.UIState",
			inputData: { key: { value: "pane" }, initial: { value: "left" } },
		},
		route: {
			type: "ui.UIRoute",
			inputData: { path: { value: "/fixture" } },
			connections: {
				inputs: {
					components: [{ nodeId: "tabs", portName: "self" }],
					components_1: [{ nodeId: "leftPane", portName: "self" }],
					components_2: [{ nodeId: "rightPane", portName: "self" }],
				},
			},
		},
		tabs: {
			type: "ui.Tabs",
			connections: {
				inputs: {
					components: [{ nodeId: "leftTab", portName: "self" }],
					components_1: [{ nodeId: "rightTab", portName: "self" }],
				},
			},
		},
		leftTab: {
			type: "ui.Tabs.Tab",
			inputData: { stateValue: { value: "left" } },
			connections: {
				inputs: {
					selects: [{ nodeId: "choice", portName: "value" }],
					components: [{ nodeId: "leftTabLabel", portName: "self" }],
				},
			},
		},
		rightTab: {
			type: "ui.Tabs.Tab",
			inputData: { stateValue: { value: "right" } },
			connections: {
				inputs: {
					selects: [{ nodeId: "choice", portName: "value" }],
					components: [{ nodeId: "rightTabLabel", portName: "self" }],
				},
			},
		},
		leftTabLabel: { type: "ui.Text", inputData: { value: { value: "Left tab" } } },
		rightTabLabel: {
			type: "ui.Text",
			inputData: { value: { value: "Right tab" } },
		},
		leftPane: {
			type: "ui.Text",
			inputData: {
				value: { value: "left pane body" },
				stateValue: { value: "left" },
			},
			connections: {
				inputs: { visibleWhen: [{ nodeId: "choice", portName: "value" }] },
			},
		},
		rightPane: {
			type: "ui.Text",
			inputData: {
				value: { value: "right pane body" },
				stateValue: { value: "right" },
			},
			connections: {
				inputs: { visibleWhen: [{ nodeId: "choice", portName: "value" }] },
			},
		},
	},
};

describe("a surface whose choices are nodes", () => {
	const compiled = compileUI(graph);

	it("compiles the state vocabulary without complaint", () => {
		expect(compiled.diagnostics).toEqual([]);
		expect(compiled.routes[0].state).toEqual({ pane: "left" });
	});

	it("starts on the choice the state node declares", () => {
		render(<UIRouteView route={compiled.routes[0]} />);

		expect(screen.getByText("left pane body")).toBeTruthy();
		expect(screen.queryByText("right pane body")).toBeNull();
	});

	it("draws the other pane once the other choice is made", () => {
		render(<UIRouteView route={compiled.routes[0]} />);

		fireEvent.click(screen.getByText("Right tab"));

		expect(screen.getByText("right pane body")).toBeTruthy();
		expect(screen.queryByText("left pane body")).toBeNull();
	});

	it("does not mount what is behind an unselected choice", () => {
		// Left out rather than hidden: a pane nobody is looking at must not
		// be subscribing to anything.
		const { container } = render(<UIRouteView route={compiled.routes[0]} />);

		expect(container.innerHTML).not.toContain("right pane body");
	});
});
