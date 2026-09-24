// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import {
	screenPointToCanvas,
	screenRectToCanvas,
} from "./connectionCalculator";

describe("screenPointToCanvas", () => {
	it("uses the stage center as the canvas origin", () => {
		const stageRect = {
			x: 100,
			y: 200,
			width: 800,
			height: 600,
			top: 200,
			left: 100,
			right: 900,
			bottom: 800,
			toJSON: () => ({}),
		} as DOMRect;

		expect(screenPointToCanvas(500, 500, stageRect, 1)).toEqual({ x: 0, y: 0 });
		expect(screenPointToCanvas(600, 550, stageRect, 2)).toEqual({
			x: 50,
			y: 25,
		});
	});

	it("converts rect bounds into canvas space", () => {
		const stageRect = {
			x: 0,
			y: 0,
			width: 1000,
			height: 800,
			top: 0,
			left: 0,
			right: 1000,
			bottom: 800,
			toJSON: () => ({}),
		} as DOMRect;

		const rect = {
			x: 600,
			y: 420,
			width: 12,
			height: 12,
			top: 420,
			left: 600,
			right: 612,
			bottom: 432,
			toJSON: () => ({}),
		} as DOMRect;

		const canvasRect = screenRectToCanvas(rect, stageRect, 1);

		expect(canvasRect.x).toBe(100);
		expect(canvasRect.y).toBe(20);
		expect(canvasRect.width).toBe(12);
		expect(canvasRect.height).toBe(12);
	});
});

describe("syncConnectionElements", () => {
	it("renders and updates connection elements in the stage container", async () => {
		const editorId = "test-editor";
		const container = document.createElement("div");
		container.id = `__node_editor_connections__${editorId}`;
		document.body.appendChild(container);

		const { syncConnectionElements, deleteConnection } = await import(
			"./connectionCalculator"
		);

		syncConnectionElements(
			[
				{
					id: "conn-1",
					outputNodeId: "node-1",
					outputPortName: "out",
					inputNodeId: "node-2",
					inputPortName: "in",
				},
			],
			editorId,
			"smooth",
		);

		const line1 = container.querySelector<SVGPathElement>(
			'[data-connection-id="conn-1"]',
		);
		expect(line1).not.toBeNull();
		expect(line1?.parentElement?.tagName.toLowerCase()).toBe("svg");

		// Re-sync with a second connection added
		syncConnectionElements(
			[
				{
					id: "conn-1",
					outputNodeId: "node-1",
					outputPortName: "out",
					inputNodeId: "node-2",
					inputPortName: "in",
				},
				{
					id: "conn-2",
					outputNodeId: "node-2",
					outputPortName: "out",
					inputNodeId: "node-3",
					inputPortName: "in",
				},
			],
			editorId,
			"smooth",
		);

		expect(
			container.querySelector('[data-connection-id="conn-1"]'),
		).not.toBeNull();
		expect(
			container.querySelector('[data-connection-id="conn-2"]'),
		).not.toBeNull();

		// Remove conn-1 via roster sync
		syncConnectionElements(
			[
				{
					id: "conn-2",
					outputNodeId: "node-2",
					outputPortName: "out",
					inputNodeId: "node-3",
					inputPortName: "in",
				},
			],
			editorId,
			"smooth",
		);

		expect(container.querySelector('[data-connection-id="conn-1"]')).toBeNull();
		expect(
			container.querySelector('[data-connection-id="conn-2"]'),
		).not.toBeNull();

		// Delete conn-2 explicitly
		deleteConnection({ id: "conn-2" });
		expect(container.querySelector('[data-connection-id="conn-2"]')).toBeNull();

		container.remove();
	});
});
