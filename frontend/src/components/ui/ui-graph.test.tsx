// @vitest-environment jsdom

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { createFlumeConfig } from "../flume/flume-config.generated";
import { compileUI, type FlumeGraph } from "./compiler";
import {
	clearSeries,
	type CompiledUINode,
	type CompiledUIRoute,
	readSeriesForTest,
	renderNode,
	renderUIRoute,
} from "./renderer";
import metadataJson from "./ui-component-metadata.generated.json";
import { uiComponents } from "./ui-component-registry.generated";

afterEach(() => {
	cleanup();
});

describe("UI Component Reflection & Registry", () => {
	it("discovers various component shapes from the public library", () => {
		// Simple component
		expect(uiComponents.Badge).toBeDefined();
		expect(metadataJson.Badge).toBeDefined();

		// CVA variant component
		expect(uiComponents.Button).toBeDefined();
		expect(metadataJson.Button).toBeDefined();

		// Component inheriting DOM props
		expect(uiComponents.Panel).toBeDefined();
		expect(metadataJson.Panel).toBeDefined();

		// Structural component
		expect(uiComponents.Flex).toBeDefined();
		expect(metadataJson.Flex).toBeDefined();

		// Compound sub-components
		expect(uiComponents["Flex.Row"]).toBeDefined();
		expect(metadataJson["Flex.Row"]).toBeDefined();
		expect(uiComponents["Flex.Column"]).toBeDefined();
		expect(metadataJson["Flex.Column"]).toBeDefined();
		expect(uiComponents["Panel.Header"]).toBeDefined();
		expect(metadataJson["Panel.Header"]).toBeDefined();
		expect(uiComponents["Panel.Title"]).toBeDefined();
		expect(metadataJson["Panel.Title"]).toBeDefined();
	});

	it("reflects prop types accurately without exposing non-serializable properties", () => {
		const panelMeta = metadataJson.Panel;
		expect(panelMeta).toBeDefined();
		expect(panelMeta.hasChildren).toBe(true);

		const propNames = panelMeta.props.map((p: any) => p.name);

		// className exposed
		expect(propNames).toContain("className");

		// variant union mapped to select with options
		const variantProp = panelMeta.props.find((p: any) => p.name === "variant");
		expect(variantProp).toBeDefined();
		expect(variantProp?.type).toBe("select");
		expect(variantProp?.options).toContain("sunken");
		expect(variantProp?.options).toContain("surface");
		expect(variantProp?.options).toContain("raised");

		// size mapped to select
		const sizeProp = panelMeta.props.find((p: any) => p.name === "size");
		expect(sizeProp).toBeDefined();
		expect(sizeProp?.type).toBe("select");
		expect(sizeProp?.options).toContain("m");

		// ref and callback event props strictly excluded
		expect(propNames).not.toContain("ref");
		expect(propNames).not.toContain("key");
		expect(propNames).not.toContain("children");
		expect(propNames.some((n: string) => n.startsWith("on"))).toBe(false);

		// Flex.Row has boolean and gap props
		const flexRowMeta = metadataJson["Flex.Row"];
		expect(flexRowMeta).toBeDefined();
		const flexPropNames = flexRowMeta.props.map((p: any) => p.name);
		expect(flexPropNames).toContain("fullHeight");
		const fullHeightProp = flexRowMeta.props.find(
			(p: any) => p.name === "fullHeight",
		);
		expect(fullHeightProp?.type).toBe("boolean");
	});

	it("resolves all registry entries to valid React components", () => {
		for (const [name, Component] of Object.entries(uiComponents)) {
			expect(
				Component,
				`Registry component "${name}" should be defined`,
			).toBeDefined();
			expect(
				typeof Component === "function" ||
					(typeof Component === "object" && Component !== null),
				`Registry component "${name}" should be a callable component or forwardRef object`,
			).toBe(true);
		}
	});

	it("generates Flume node definitions matching reflected metadata", () => {
		const config = createFlumeConfig();
		expect(config).toBeDefined();

		// Verify UI component node types exist in Flume
		expect(config.nodeTypes["ui.Panel"]).toBeDefined();
		expect(config.nodeTypes["ui.Panel"].category).toBe("UI Components");

		expect(config.nodeTypes["ui.Badge"]).toBeDefined();
		expect(config.nodeTypes["ui.Badge"].category).toBe("UI Components");

		expect(config.nodeTypes["ui.Flex.Row"]).toBeDefined();
		expect(config.nodeTypes["ui.Flex.Row"].category).toBe("UI Components");
	});
});

describe("Runtime Renderer & Live Data Bindings", () => {
	it("renders an authored UI tree with structural nesting and configured props", () => {
		const authoredGraph: CompiledUINode = {
			name: "Flex.Column",
			className: "h-full p-4 gap-4",
			children: [
				{
					name: "Panel",
					props: { variant: "sunken", size: "m" },
					className: "flex-1",
					children: [
						{
							name: "Panel.Header",
							children: [
								{
									name: "Panel.Title",
									props: { children: "Market Diagnostics" },
								},
							],
						},
						{
							name: "Badge",
							props: { variant: "success", children: "LIVE" },
						},
					],
				},
				{
					name: "Panel",
					props: { variant: "raised" },
					children: [
						{
							name: "Button",
							props: { variant: "solid", children: "Execute Trade" },
						},
					],
				},
			],
		};

		const { container } = render(<>{renderNode(authoredGraph)}</>);

		// Check for rendered content
		expect(screen.getByText("Market Diagnostics")).toBeDefined();
		expect(screen.getByText("LIVE")).toBeDefined();
		expect(screen.getByText("Execute Trade")).toBeDefined();

		// Structural class check
		expect(container.querySelector(".h-full")).not.toBeNull();
		expect(container.querySelector(".flex-1")).not.toBeNull();
	});

	it("resolves live observable state bindings dynamically during render", () => {
		const boundNode: CompiledUINode = {
			name: "Badge",
			props: {
				variant: "success",
				children: { binding: { node: "metric_flow", port: "status_text" } },
			},
		};

		const observableState = {
			metric_flow: {
				status_text: "HEALTHY_FEED",
			},
		};

		render(<>{renderNode(boundNode, undefined, observableState)}</>);
		expect(screen.getByText("HEALTHY_FEED")).toBeDefined();
	});

	it("fails cleanly with an informative error on unknown components", () => {
		const invalidNode: CompiledUINode = {
			name: "NonExistentSuperWidget",
		};

		expect(() => renderNode(invalidNode)).toThrow(
			/unknown component "NonExistentSuperWidget"/,
		);
	});

	it("renders a compiled UI route container with root subgraph", () => {
		const route: CompiledUIRoute = {
			path: "/cortex",
			title: "Cortex Surface",
			components: [
				{
					name: "Flex.Row",
					className: "h-full w-full",
					children: [
						{
							name: "Panel",
							props: { children: "Cortex Main Content" },
						},
					],
				},
			],
		};

		render(<>{renderUIRoute(route)}</>);
		expect(screen.getByText("Cortex Main Content")).toBeDefined();
	});
});

describe("UI Graph Compiler (compileUI)", () => {
	it("compiles a persisted Flume graph into a validated CompiledUIRoute", () => {
		const flumeGraph: FlumeGraph = {
			id: "test_ui_graph",
			name: "test_ui_graph",
			nodes: {
				route1: {
					id: "route1",
					type: "ui.UIRoute",
					inputData: {
						path: { value: "/diagnostics" },
						title: { value: "Diagnostics" },
					},
					connections: {
						inputs: {
							components: [{ nodeId: "column1", portName: "out" }],
						},
					},
				},
				column1: {
					id: "column1",
					type: "ui.Flex.Column",
					inputData: {
						className: { value: "h-full p-4 gap-4" },
					},
					connections: {
						inputs: {
							components: [{ nodeId: "panel1", portName: "out" }],
							components_1: [{ nodeId: "panel2", portName: "out" }],
						},
						outputs: {
							out: [{ nodeId: "route1", portName: "components" }],
						},
					},
				},
				panel1: {
					id: "panel1",
					type: "ui.Panel",
					inputData: {
						variant: { value: "sunken" },
					},
					connections: {
						inputs: {
							components: [{ nodeId: "badge1", portName: "out" }],
						},
						outputs: {
							out: [{ nodeId: "column1", portName: "components" }],
						},
					},
				},
				badge1: {
					id: "badge1",
					type: "ui.Badge",
					inputData: {
						variant: { value: "success" },
						children: { value: "OPERATIONAL" },
					},
					connections: {
						outputs: {
							out: [{ nodeId: "panel1", portName: "components" }],
						},
					},
				},
				panel2: {
					id: "panel2",
					type: "ui.Panel",
					inputData: {
						variant: { value: "raised" },
					},
					connections: {
						outputs: {
							out: [{ nodeId: "column1", portName: "components_1" }],
						},
					},
				},
			},
		};

		const result = compileUI(flumeGraph);
		expect(result.diagnostics).toHaveLength(0);
		expect(result.routes).toHaveLength(1);

		const route = result.routes[0];
		expect(route.path).toBe("/diagnostics");
		expect(route.title).toBe("Diagnostics");
		expect(route.components).toHaveLength(1);

		const col = route.components[0];
		expect(col.name).toBe("Flex.Column");
		expect(col.className).toBe("h-full p-4 gap-4");
		expect(col.children).toHaveLength(2);

		// Deterministic ordering: panel1 first, panel2 second
		expect(col.children?.[0].name).toBe("Panel");
		expect(col.children?.[0].props?.variant).toBe("sunken");
		expect(col.children?.[0].children?.[0].name).toBe("Badge");
		expect(col.children?.[0].children?.[0].props?.children).toBe("OPERATIONAL");

		expect(col.children?.[1].name).toBe("Panel");
		expect(col.children?.[1].props?.variant).toBe("raised");

		// Render the compiled route
		render(<>{renderUIRoute(route)}</>);
		expect(screen.getByText("OPERATIONAL")).toBeDefined();
	});

	it("validates select variants against reflected metadata and emits diagnostics on invalid values", () => {
		const flumeGraph: FlumeGraph = {
			nodes: {
				btn: {
					id: "btn",
					type: "ui.Button",
					inputData: {
						variant: { value: "primary" }, // Invalid variant, should be solid | outline | quiet | bare
					},
				},
			},
		};

		const result = compileUI(flumeGraph);
		expect(result.diagnostics.length).toBeGreaterThan(0);
		const variantDiag = result.diagnostics.find(
			(d) => d.kind === "invalid_variant",
		);
		expect(variantDiag).toBeDefined();
		expect(variantDiag?.message).toMatch(
			/Invalid value "primary" for variant\/select prop "variant" on component "Button"/,
		);
	});

	it("validates unknown components and emits clear diagnostic", () => {
		const flumeGraph: FlumeGraph = {
			nodes: {
				weird: {
					id: "weird",
					type: "ui.FakeWidget",
				},
			},
		};

		const result = compileUI(flumeGraph);
		const unknownDiag = result.diagnostics.find(
			(d) => d.kind === "unknown_component",
		);
		expect(unknownDiag).toBeDefined();
		expect(unknownDiag?.message).toMatch(
			/UI component "FakeWidget" is not registered/,
		);
	});

	it("preserves live data bindings when upstream node is a non-UI data source", () => {
		const flumeGraph: FlumeGraph = {
			nodes: {
				feed: {
					id: "feed",
					type: "arithmetic.Add",
					connections: {
						outputs: {
							out: [{ nodeId: "meter1", portName: "value" }],
						},
					},
				},
				meter1: {
					id: "meter1",
					type: "ui.Meter",
					connections: {
						inputs: {
							value: [{ nodeId: "feed", portName: "out" }],
						},
					},
				},
			},
		};

		const result = compileUI(flumeGraph);
		expect(result.routes).toHaveLength(1);
		const meterNode = result.routes[0].components[0];
		expect(meterNode.name).toBe("Meter");
		expect(meterNode.props?.value).toEqual({
			binding: {
				node: "feed",
				port: "out",
			},
		});
	});
});

describe("Authored UI manifest", () => {
	/*
		The manifest the backend ships, compiled and rendered by the same path
		the app uses. A graph that only compiles proves nothing about whether
		anything reaches the screen.
	*/
	it("renders the authored overview graph", async () => {
		const manifest = (await import("../../../../manifest/ui_overview.json"))
			.default as unknown as FlumeGraph;

		const compilation = compileUI(manifest);

		expect(compilation.diagnostics).toEqual([]);
		expect(compilation.routes).toHaveLength(1);

		const [route] = compilation.routes;
		expect(route.path).toBe("/overview");
		expect(route.title).toBe("Signal Overview");

		render(<>{renderUIRoute(route)}</>);

		// Each panel the graph authored, by the title it was given.
		expect(screen.getByText("Correlation")).toBeDefined();
		expect(screen.getByText("Liquidity")).toBeDefined();
		expect(screen.getByText("Arrivals")).toBeDefined();

		// And what each panel was told to show.
		expect(screen.getByText("signed correlation")).toBeDefined();
		expect(screen.getByText("relative spread")).toBeDefined();
		expect(screen.getByText("conditional intensity")).toBeDefined();
	});

	it("nests components the way the graph wired them", async () => {
		const manifest = (await import("../../../../manifest/ui_overview.json"))
			.default as unknown as FlumeGraph;

		const [route] = compileUI(manifest).routes;
		const [page] = route.components;

		expect(page.name).toBe("Flex.Column");
		expect(page.className).toBe("h-full min-h-0 gap-4 p-4");
		expect(page.children).toHaveLength(3);

		const [correlation] = page.children ?? [];
		expect(correlation.name).toBe("Panel");

		// The heading is a component of its own, not an attribute on the panel.
		const [heading] = correlation.children ?? [];
		expect(heading.name).toBe("Panel.Header");
		expect(heading.props?.title).toBe("Correlation");
		expect((correlation.children ?? []).map((child) => child.name)).toEqual([
			"Panel.Header",
			"Meter",
			"Badge",
		]);
	});

	it("refuses a component the library does not have", () => {
		const compilation = compileUI({
			nodes: {
				route: {
					id: "route",
					type: "ui.UIRoute",
					inputData: { path: { value: "/broken" } },
					connections: {
						inputs: { components: [{ nodeId: "ghost", portName: "out" }] },
						outputs: {},
					},
				},
				ghost: {
					id: "ghost",
					type: "ui.NotAComponent",
					connections: {
						inputs: {},
						outputs: { out: [{ nodeId: "route", portName: "components" }] },
					},
				},
			},
		} as unknown as FlumeGraph);

		// Naming a component that does not exist is reported against the node
		// that named it, rather than rendering as nothing.
		const unknown = compilation.diagnostics.filter(
			(diagnostic) => diagnostic.kind === "unknown_component",
		);

		expect(unknown.length).toBeGreaterThan(0);
		expect(unknown[0].nodeId).toBe("ghost");
	});
});

describe("an unfilled control", () => {
	/*
		Flume stores a control the author never typed into as an empty object.
		Stat renders its value slot directly, so passing that object on throws
		"Objects are not valid as a React child" and takes the whole page down.
	*/
	it("carries no value rather than an empty object", () => {
		const compilation = compileUI({
			nodes: {
				row: {
					id: "row",
					type: "ui.Flex.Row",
					inputData: { className: {} },
					connections: {
						inputs: {
							components: [{ nodeId: "spark", portName: "out" }],
							components_1: [{ nodeId: "stat", portName: "out" }],
						},
						outputs: {},
					},
				},
				spark: {
					id: "spark",
					type: "ui.Sparkline",
					inputData: { title: {} },
					connections: {
						inputs: {},
						outputs: { out: [{ nodeId: "row", portName: "components" }] },
					},
				},
				stat: {
					id: "stat",
					type: "ui.Stat",
					// The slot the author never filled in.
					inputData: { value: {}, label: {} },
					connections: {
						inputs: {},
						outputs: { out: [{ nodeId: "row", portName: "components_1" }] },
					},
				},
			},
		});

		expect(compilation.diagnostics).toEqual([]);

		const rendered = compilation.routes[0].components;
		const stat = rendered
			.flatMap((node) => node.children ?? [])
			.find((child) => child.name === "Stat");

		expect(stat?.props?.value).toBeUndefined();

		// And the page renders rather than throwing.
		render(<>{renderUIRoute(compilation.routes[0])}</>);
	});
});

describe("a component that draws a series", () => {
	/*
		The whole point of the editor is plugging data into a component. A
		series prop was dropped by the reflector, so Sparkline had no port to
		plug anything into.
	*/
	it("exposes the port its data arrives on", () => {
		const compilation = compileUI({
			nodes: {
				feed: {
					id: "feed",
					type: "arithmetic.Add",
					connections: {
						outputs: { out: [{ nodeId: "spark", portName: "points" }] },
					},
				},
				spark: {
					id: "spark",
					type: "ui.Sparkline",
					connections: {
						inputs: { points: [{ nodeId: "feed", portName: "out" }] },
					},
				},
			},
		});

		expect(compilation.diagnostics).toEqual([]);

		const [spark] = compilation.routes[0].components;
		expect(spark.props?.points).toEqual({
			binding: { node: "feed", port: "out" },
		});
	});

	it("accumulates the scalars it is handed into the history it draws", async () => {
		clearSeries();

		const graph = {
			nodes: {
				feed: {
					id: "feed",
					type: "arithmetic.Add",
					connections: {
						outputs: { out: [{ nodeId: "spark", portName: "points" }] },
					},
				},
				spark: {
					id: "spark",
					type: "ui.Sparkline",
					connections: {
						inputs: { points: [{ nodeId: "feed", portName: "out" }] },
					},
				},
			},
		};

		const [route] = compileUI(graph).routes;

		const { rerender } = render(
			<>{renderUIRoute(route, { feed: { out: 1 } })}</>,
		);
		rerender(<>{renderUIRoute(route, { feed: { out: 2 } })}</>);
		rerender(<>{renderUIRoute(route, { feed: { out: 3 } })}</>);

		// The readings are held against the producer, so the history is the
		// series the sparkline draws rather than only the latest scalar.
		await waitFor(() => {
			const path = document.querySelector("svg polyline, svg path");
			expect(path).toBeDefined();
		});

		expect(readSeriesForTest("feed.out")).toEqual([1, 2, 3]);
	});

	it("renders successive backend Add results in a Sparkline", async () => {
		clearSeries();

		const graph: FlumeGraph = {
			nodes: {
				addNode: {
					id: "addNode",
					type: "arithmetic.Add",
					inputData: {
						a: { value: 15 },
						b: { value: 25 },
					},
					connections: {
						outputs: {
							out: [{ nodeId: "sparklineNode", portName: "points" }],
						},
					},
				},
				sparklineNode: {
					id: "sparklineNode",
					type: "ui.Sparkline",
					connections: {
						inputs: {
							points: [{ nodeId: "addNode", portName: "out" }],
						},
					},
				},
			},
		};

		// Backend output fixtures; arithmetic is tested by the Go compiler tests.
		const initialEvaluation = { addNode: { out: 40 } };

		// 2. Compile UI route
		const compilation = compileUI(graph);
		expect(compilation.routes).toHaveLength(1);
		const [route] = compilation.routes;

		// 3. Render route with evaluated data
		const { rerender } = render(
			renderUIRoute(route, initialEvaluation) as React.ReactElement,
		);

		const updatedEvaluation = { addNode: { out: 50 } };

		rerender(renderUIRoute(route, updatedEvaluation) as React.ReactElement);

		await waitFor(() => {
			const path = document.querySelector("svg polyline, svg path");
			expect(path).toBeDefined();
		});

		expect(readSeriesForTest("addNode.out")).toEqual([40, 50]);
	});
});
