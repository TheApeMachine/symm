// @vitest-environment jsdom
import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ImpulseMapViz } from "./impulse-map-viz";
import type { ImpulseNode } from "./types";

afterEach(() => {
	cleanup();
});

describe("ImpulseMapViz", () => {
	it("centers the grid in the container and uses available space", () => {
		// Mock container dimensions 800x600
		const data: ImpulseNode[] = [
			{ id: "n1", label: "A", cluster: 1, snr: 1, activation: 0.1, x: 0, y: 0 },
			{ id: "n2", label: "B", cluster: 1, snr: 1, activation: 0.9, x: 2, y: 2 },
		];

		const { container } = render(
			<div style={{ width: "800px", height: "600px" }}>
				<ImpulseMapViz data={data} />
			</div>,
		);

		const circles = container.querySelectorAll(".nodes circle");
		expect(circles).toHaveLength(2);

		const cx1 = Number(circles[0].getAttribute("cx"));
		const cy1 = Number(circles[0].getAttribute("cy"));
		const cx2 = Number(circles[1].getAttribute("cx"));
		const cy2 = Number(circles[1].getAttribute("cy"));

		// Average center of the 2 nodes must be at the center of the viewport (400, 300)
		const centerX = (cx1 + cx2) / 2;
		const centerY = (cy1 + cy2) / 2;

		expect(Math.round(centerX)).toBe(400);
		expect(Math.round(centerY)).toBe(300);

		// The distance between the 2 nodes should use available space (not squeezed into ~38px in the corner)
		const dist = Math.hypot(cx2 - cx1, cy2 - cy1);
		// On 800x600 with usable ~500px, 2 units span should be well over 200px
		expect(dist).toBeGreaterThan(200);
	});

	it("refits the grid to the container when the grid grows", () => {
		const initialData: ImpulseNode[] = [
			{ id: "n1", label: "A", cluster: 1, snr: 1, activation: 0.1, x: 0, y: 0 },
			{ id: "n2", label: "B", cluster: 1, snr: 1, activation: 0.2, x: 1, y: 1 },
		];

		const { container, rerender } = render(
			<div style={{ width: "800px", height: "600px" }}>
				<ImpulseMapViz data={initialData} />
			</div>,
		);

		let circles = container.querySelectorAll(".nodes circle");
		expect(circles).toHaveLength(2);
		const initialCx2 = Number(circles[1].getAttribute("cx"));

		// Grid grows: add more nodes extending coordinate bounds from [0, 1] to [0, 5]
		const expandedData: ImpulseNode[] = [
			{ id: "n1", label: "A", cluster: 1, snr: 1, activation: 0.1, x: 0, y: 0 },
			{ id: "n2", label: "B", cluster: 1, snr: 1, activation: 0.2, x: 1, y: 1 },
			{ id: "n3", label: "C", cluster: 2, snr: 1, activation: 0.8, x: 5, y: 5 },
		];

		act(() => {
			rerender(
				<div style={{ width: "800px", height: "600px" }}>
					<ImpulseMapViz data={expandedData} />
				</div>,
			);
		});

		circles = container.querySelectorAll(".nodes circle");
		expect(circles).toHaveLength(3);

		// With the grid growing to span 5 units, node 2 (at coordinate 1) must be refit closer to the center
		const refitCx2 = Number(circles[1].getAttribute("cx"));
		expect(refitCx2).not.toBe(initialCx2);

		// Check that the full expanded grid center remains at 400, 300
		const cx1 = Number(circles[0].getAttribute("cx"));
		const cy1 = Number(circles[0].getAttribute("cy"));
		const cx3 = Number(circles[2].getAttribute("cx"));
		const cy3 = Number(circles[2].getAttribute("cy"));

		const midX = (cx1 + cx3) / 2;
		const midY = (cy1 + cy3) / 2;
		expect(Math.round(midX)).toBe(400);
		expect(Math.round(midY)).toBe(300);
	});

	it("updates in real-time when regions/nodes are activated", () => {
		const initialData: ImpulseNode[] = [
			{ id: "n1", label: "A", cluster: 1, snr: 1, activation: 0.05, x: 0, y: 0 },
			{ id: "n2", label: "B", cluster: 2, snr: 1, activation: 0.05, x: 2, y: 2 },
		];

		const { container, rerender } = render(
			<div style={{ width: "800px", height: "600px" }}>
				<ImpulseMapViz
					data={initialData}
					activeEvents={[]}
				/>
			</div>,
		);

		let circles = container.querySelectorAll(".nodes circle");
		const inactiveFill = circles[1].getAttribute("fill");

		// Live tape activates region 2 / node 2
		const updatedData: ImpulseNode[] = [
			{ id: "n1", label: "A", cluster: 1, snr: 1, activation: 0.05, x: 0, y: 0 },
			{ id: "n2", label: "B", cluster: 2, snr: 1, activation: 0.95, x: 2, y: 2 },
		];

		act(() => {
			rerender(
				<div style={{ width: "800px", height: "600px" }}>
					<ImpulseMapViz
						data={updatedData}
						activeEvents={[{ id: 2, label: "Region #2", activity: 0.95, members: 1 }]}
					/>
				</div>,
			);
		});

		circles = container.querySelectorAll(".nodes circle");
		const activeFill = circles[1].getAttribute("fill");
		expect(activeFill).not.toBe(inactiveFill);

		// Highly active node gets glowing stroke and halo
		expect(circles[1].getAttribute("stroke")).toBe("#fbbf24");
		const halos = container.querySelectorAll(".halos circle");
		expect(halos.length).toBeGreaterThan(0);

		// Active regions empirical overlay displays the active region
		const activeOverlay = container.querySelector(".space-y-1\\.5");
		expect(activeOverlay?.textContent).toContain("Region #2");
	});

	it("toggles between Initial Grid and Discovered Regions with correct controls", () => {
		const data: ImpulseNode[] = [
			{ id: "n1", label: "A", cluster: 1, snr: 1, activation: 0.8, x: 0, y: 0 },
			{ id: "n2", label: "B", cluster: 1, snr: 1, activation: 0.9, x: 1, y: 1 },
		];

		const { container } = render(
			<div style={{ width: "800px", height: "600px" }}>
				<ImpulseMapViz data={data} />
			</div>,
		);

		const regionsBtn = container.querySelector('[data-l="view-sympathy-regions"]') as HTMLButtonElement;
		const gridBtn = container.querySelector('[data-l="view-initial-grid"]') as HTMLButtonElement;

		expect(regionsBtn).toBeTruthy();
		expect(gridBtn).toBeTruthy();

		// Click Discovered Regions
		act(() => {
			fireEvent.click(regionsBtn);
		});

		// Contours and region badges should be active
		const contours = container.querySelector(".contours");
		expect(contours).toBeTruthy();

		// Click back to Initial Grid
		act(() => {
			fireEvent.click(gridBtn);
		});

		const paths = container.querySelectorAll(".contours path");
		expect(paths.length).toBe(0);
	});
});
