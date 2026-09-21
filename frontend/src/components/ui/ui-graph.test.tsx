// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import React from "react";
import { uiComponents } from "./ui-component-registry.generated";
import metadataJson from "./ui-component-metadata.generated.json";
import { type CompiledUINode, type CompiledUIRoute, renderNode, renderUIRoute } from "./renderer";
import { createFlumeConfig } from "../flume/flume-config.generated";

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
		const fullHeightProp = flexRowMeta.props.find((p: any) => p.name === "fullHeight");
		expect(fullHeightProp?.type).toBe("boolean");
	});

	it("resolves all registry entries to valid React components", () => {
		for (const [name, Component] of Object.entries(uiComponents)) {
			expect(Component, `Registry component "${name}" should be defined`).toBeDefined();
			expect(
				typeof Component === "function" || (typeof Component === "object" && Component !== null),
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

describe("Runtime Renderer", () => {
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
							props: { variant: "primary", children: "Execute Trade" },
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

	it("parses propsJson string when props object is not passed", () => {
		const node: CompiledUINode = {
			name: "Badge",
			propsJson: JSON.stringify({ variant: "warning", children: "RECONCILING" }),
		};

		render(<>{renderNode(node)}</>);
		expect(screen.getByText("RECONCILING")).toBeDefined();
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
