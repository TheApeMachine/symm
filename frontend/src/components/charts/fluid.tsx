import { createRef, useEffect } from "react";
import { createStore } from "@tanstack/store";
import type { ManifoldFrame } from "#/collections/types";
import { drawFluidDisplay } from "#/components/charts/fluid-display";
import { paintPhaseDial } from "#/components/charts/phase-dial";
import {
	clearCanvas,
	drawGrid,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	terminalPhaseScanFromFrame,
	terminalPhaseStatusFromFrame,
	terminalWaveModesFromFrame,
} from "#/components/terminal/charts-frame";

const fluidFieldCanvasRef = createRef<HTMLCanvasElement>();
let fluidFocus = "";
const manifoldBatchStore = createStore<unknown>(null);

const drawWaiting = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	message: string,
) => {
	clearCanvas(context, width, height);
	drawGrid(context, width, height);
	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "11px JetBrains Mono, monospace";
	context.fillText(message, 18, 52);
};

/*
paintTerminalFluidChart updates the current focused manifold projection. A
summary-only delta cannot erase the last complete field; changing focus clears
the prior symbol while the backend prepares the newly requested projection.
*/
export const paintTerminalFluidChart = (
	value: unknown,
	focusSymbol: string,
) => {
	manifoldBatchStore.setState(() => value);
	paintTerminalFluidCompose(value, focusSymbol);
};

/*
repaintTerminalFluidChart redraws the retained field batch after binary lattice
or manifold_wave packets arrive. Those packets store only; without a redraw the
phase dial never sees the wave modes for the cut.
*/
export const repaintTerminalFluidChart = (focusSymbol: string) => {
	const fieldCanvas = fluidFieldCanvasRef.current;

	if (fieldCanvas !== null) {
		drawFluidDisplay(
			fieldCanvas,
			fieldCanvas.clientWidth,
			fieldCanvas.clientHeight,
		);
	}

	if (manifoldBatchStore.state === null) {
		return;
	}

	paintTerminalFluidCompose(manifoldBatchStore.state, focusSymbol);
};

const paintTerminalFluidCompose = (value: unknown, focusSymbol: string) => {
	const fieldCanvas = fluidFieldCanvasRef.current;

	if (fieldCanvas === null) {
		return;
	}

	const width = fieldCanvas.clientWidth;
	const height = fieldCanvas.clientHeight;

	if (fluidFocus !== focusSymbol) {
		fluidFocus = focusSymbol;
		const waiting = resizeCanvas(fieldCanvas);

		if (waiting !== null) {
			drawWaiting(waiting, width, height, "waiting for pilot-wave field");
		}
	}

	const frames = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ManifoldFrame[];
	const focused =
		frames.find(
			(entry) => focusSymbol === "" || entry.symbol === focusSymbol,
		) ?? (focusSymbol === "" ? (frames.at(-1) ?? null) : null);
	const frame = focused;

	if (frame === null) {
		return;
	}

	const wave = terminalWaveModesFromFrame(frame);
	const phaseScan = terminalPhaseScanFromFrame(frame);
	const phaseStatus = terminalPhaseStatusFromFrame(frame);

	// An empty wave still publishes, so the dial clears instead of holding a stale
	// fingerprint from the previous cut.
	paintPhaseDial({ wave, scan: phaseScan, status: phaseStatus });

	if (wave.length === 0) {
		return;
	}

	const painted = drawFluidDisplay(fieldCanvas, width, height);

	if (!painted) {
		const fieldContext = resizeCanvas(fieldCanvas);

		if (fieldContext === null) {
			return;
		}

		clearCanvas(fieldContext, width, height);
		drawGrid(fieldContext, width, height);
	}
};

/*
TerminalFluidChart is the static canvas shell. DRAW paints via
paintTerminalFluidChart, and the field canvas only blits backend GPU images.
*/
export const TerminalFluidChart = () => {
	useEffect(() => {
		repaintTerminalFluidChart(fluidFocus);
	}, []);

	return <canvas ref={fluidFieldCanvasRef} className="block size-full" />;
};
