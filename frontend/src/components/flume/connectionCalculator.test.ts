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
