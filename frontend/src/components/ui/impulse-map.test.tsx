// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ImpulseMap, type ImpulsePoint } from "./impulse-map";
import { compileUI } from "./compiler";
import { renderNode, resolveBindings } from "./renderer";
import metadata from "./ui-component-metadata.generated.json";
import { createFlumeConfig } from "../flume/flume-config.generated";

const points: ImpulsePoint[] = [
	{
		id: 7,
		source: "measured",
		label: "Observed point",
		x: 12,
		y: -20,
		snr: 3,
		activation: 0.2,
		energy: 5,
		authority: 0.8,
		present: true,
	},
];
afterEach(cleanup);

describe("ImpulseMap", () => {
	it("renders supplied geometry, inspection and regions without changing observations", () => {
		const before = structuredClone(points);
		const { container, rerender } = render(
			<ImpulseMap
				points={points}
				regions={[
					{
						id: 1,
						source: "Measured region",
						snr: 3,
						authority: 0.8,
						members: 1,
					},
				]}
				contours={[
					{
						id: "boundary",
						label: "Measured boundary",
						points: [
							{ x: 0, y: 0 },
							{ x: 20, y: 0 },
							{ x: 20, y: 30 },
						],
					},
				]}
			/>,
		);
		expect(container.querySelector("circle")?.getAttribute("cx")).toBe("12");
		expect(container.querySelector("polygon")?.getAttribute("points")).toBe(
			"0,0 20,0 20,30",
		);
		fireEvent.focus(screen.getByLabelText("Observed point"));
		expect(screen.getByText(/activation 0.2/)).toBeDefined();
		expect(screen.getByText("Measured region")).toBeDefined();
		expect(points).toEqual(before);
		rerender(<ImpulseMap points={[]} />);
		expect(container.querySelector("circle")).toBeNull();
		expect(screen.queryByText(/activation/)).toBeNull();
		expect(screen.getByText("No impulse data")).toBeDefined();
	});

	it("registers structured ports and renders graph output replacements directly", () => {
		expect(
			metadata.ImpulseMap.props.find((prop) => prop.name === "points")?.type,
		).toBe("data");
		expect(
			metadata.EvidenceGraph.props.find((prop) => prop.name === "graph")?.type,
		).toBe("data");
		expect(createFlumeConfig().nodeTypes["ui.ImpulseMap"]).toBeDefined();
		const compiled = compileUI({
			nodes: {
				source: { type: "producer" },
				view: {
					type: "ui.ImpulseMap",
					connections: {
						inputs: { points: [{ nodeId: "source", portName: "points" }] },
					},
				},
			},
		});
		expect(compiled.diagnostics).toEqual([]);
		const node = compiled.routes[0].components[0];
		expect(resolveBindings(node.props!, { source: { points } }).points).toBe(
			points,
		);
		const { container, rerender } = render(
			renderNode(node, "view", { source: { points } }),
		);
		expect(container.querySelectorAll("circle")).toHaveLength(1);
		rerender(
			renderNode(node, "view", {
				source: { points: [{ ...points[0], x: -25 }] },
			}),
		);
		expect(container.querySelector("circle")?.getAttribute("cx")).toBe("-25");
		rerender(renderNode(node, "view", {}));
		expect(container.querySelectorAll("circle")).toHaveLength(0);
	});

	it("rejects a relationship whose endpoints are missing", () => {
		expect(() =>
			render(<ImpulseMap points={points} connections={[{ from: 7, to: 8 }]} />),
		).toThrow("references a missing point");
	});
});
