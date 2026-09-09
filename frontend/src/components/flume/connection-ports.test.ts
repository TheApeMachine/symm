// @vitest-environment jsdom

import { describe, expect, it } from "vitest";
import { resolvePortDropTarget } from "./connection-ports";

describe("resolvePortDropTarget", () => {
	it("walks up from nested event targets to the port handle", () => {
		const portHandle = document.createElement("button");
		portHandle.dataset.flumeComponent = "port-handle";
		portHandle.dataset.portName = "in";
		portHandle.dataset.nodeId = "gate";
		portHandle.dataset.portType = "tensor";
		portHandle.dataset.portTransputType = "input";

		const inner = document.createElement("span");
		portHandle.appendChild(inner);
		document.body.appendChild(portHandle);

		const event = {
			clientX: 106,
			clientY: 106,
			target: inner,
		} as MouseEvent;

		expect(resolvePortDropTarget(event)).toBe(portHandle);

		portHandle.remove();
	});

	it("finds a port handle below a pointer-events-none overlay", () => {
		const portHandle = document.createElement("button");
		portHandle.dataset.flumeComponent = "port-handle";
		portHandle.dataset.portName = "x";
		portHandle.dataset.nodeId = "relu";
		portHandle.dataset.portType = "tensor";
		portHandle.dataset.portTransputType = "input";

		const overlay = document.createElement("div");
		overlay.style.pointerEvents = "none";

		document.body.append(portHandle, overlay);

		const previousElementsFromPoint = document.elementsFromPoint;
		document.elementsFromPoint = () => [overlay, portHandle];

		const event = {
			clientX: 106,
			clientY: 106,
			target: overlay,
		} as MouseEvent;

		expect(resolvePortDropTarget(event)).toBe(portHandle);

		document.elementsFromPoint = previousElementsFromPoint;
		portHandle.remove();
		overlay.remove();
	});
});
