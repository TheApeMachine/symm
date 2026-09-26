// @vitest-environment jsdom
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { FluidInspector, type NativeManifoldFrame } from "./component";

const scene = vi.hoisted(() => ({
	updateFields: vi.fn(),
	updateParticles: vi.fn(),
	setOptions: vi.fn(),
	setSlices: vi.fn(),
	dispose: vi.fn(),
}));
vi.mock("./scene", () => ({
	FluidScene: class {
		constructor() {
			return scene;
		}
	},
}));
vi.mock("./kuramoto-ring", () => ({ KuramotoRing: () => null }));
beforeEach(() => {
	vi.clearAllMocks();
});
const frame: NativeManifoldFrame = {
	epoch: "9",
	sequence: "9007199254740993",
	version: "1",
	gridX: 2,
	gridY: 2,
	gridZ: 2,
	spacing: 0.5,
	population: 1,
	positions: [0.2, 0.3, 0.4],
	velocities: [0.1, 0, 0],
	masses: [1],
	energies: [2],
	phases: [0.5],
	frequencies: [0.2],
	amplitudes: [Math.sqrt(2)],
	heat: [1],
	densityMomentum: Array(32).fill(0),
	fieldEnergy: Array(8).fill(0),
	waveReal: Array(8).fill(0),
	waveImaginary: Array(8).fill(0),
	densityScale: 1,
	momentumScale: 0.1,
	energyScale: 2,
	waveScale: 1,
	divergence: 0.1,
	guidanceSpeed: 0.2,
	coherence: 0.3,
	pressureGradient: 0.4,
	viscosity: 0.5,
	synchronization: 0.6,
	physicalTime: 0.7,
	acceptedStep: 0.01,
	substeps: 1,
};
it("renders one native physical frame, retaining its exact causal stamp and interactive field controls", () => {
	const view = render(<FluidInspector />);
	expect(screen.getByText("Awaiting physical frame")).toBeTruthy();
	expect(scene.updateParticles).not.toHaveBeenCalled();
	view.rerender(<FluidInspector frame={frame} />);
	expect(screen.getByText(/9007199254740993/)).toBeTruthy();
	expect(scene.updateFields.mock.calls[0]![0].sequence).toBe(9007199254740993n);
	expect(scene.updateParticles.mock.calls[0]![0].count).toBe(1);
	expect(Array.from(scene.updateParticles.mock.calls[0]![0].mass)).toEqual([1]);
	fireEvent.click(screen.getByRole("button", { name: "particles" }));
	expect(scene.setOptions.mock.lastCall![0].particles).toBe(false);
	fireEvent.click(screen.getByRole("button", { name: "slices" }));
	expect(screen.getAllByRole("slider")).toHaveLength(3);
	view.unmount();
	expect(scene.dispose).toHaveBeenCalledOnce();
});
