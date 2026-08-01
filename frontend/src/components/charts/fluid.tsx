import { createRef, useEffect } from "react";
import type { ManifoldFrame } from "#/collections/types";
import { drawFluidDisplay } from "#/components/charts/fluid-display";
import {
	clearCanvas,
	drawGrid,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	type TerminalPhaseResponse,
	type TerminalPhaseStatus,
	type TerminalWaveMode,
	terminalPhaseScanFromFrame,
	terminalPhaseStatusFromFrame,
	terminalWaveModesFromFrame,
} from "#/components/terminal/charts-frame";

const fluidFieldCanvasRef = createRef<HTMLCanvasElement>();
const fluidOverlayCanvasRef = createRef<HTMLCanvasElement>();
let fluidFocus = "";
let lastManifoldBatch: unknown = null;

/*
phaseOutcomeColor keeps the retained DMT basin visible as the winning
historical state changes around the phase scan.
*/
const phaseOutcomeColor = (className: string): string => {
	if (className === "buy") {
		return TERMINAL_COLORS.green;
	}

	if (className === "sell") {
		return TERMINAL_COLORS.red;
	}

	if (className === "balanced") {
		return TERMINAL_COLORS.cyan;
	}

	return TERMINAL_COLORS.muted;
};

/*
drawPhaseAxes draws the fixed reference circle and the half-radius zero ring.
Signed similarity therefore maps -1 to center, 0 to the ring, and +1 outside.
*/
const drawPhaseAxes = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
) => {
	context.strokeStyle = "rgba(105, 177, 203, 0.34)";
	context.lineWidth = 1;
	context.beginPath();
	context.arc(centerX, centerY, radius, 0, Math.PI * 2);
	context.stroke();
	context.strokeStyle = "rgba(147, 138, 126, 0.28)";
	context.beginPath();
	context.arc(centerX, centerY, radius / 2, 0, Math.PI * 2);
	context.stroke();
	context.beginPath();
	context.moveTo(centerX - radius, centerY);
	context.lineTo(centerX + radius, centerY);
	context.moveTo(centerX, centerY - radius);
	context.lineTo(centerX, centerY + radius);
	context.stroke();
};

/*
drawPhaseResponse draws each class-owned sector and its signed response without
joining different winners into one unexplained amber envelope.
*/
const drawPhaseResponse = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
	scan: TerminalPhaseResponse[],
) => {
	for (const [index, response] of scan.entries()) {
		const next = scan[(index + 1) % scan.length];
		const nextAngle =
			index === scan.length - 1 ? next.angle + Math.PI * 2 : next.angle;
		const similarity = Math.min(1, Math.max(-1, response.similarity));
		const nextSimilarity = Math.min(1, Math.max(-1, next.similarity));
		const responseRadius = (radius * (similarity + 1)) / 2;
		const nextRadius = (radius * (nextSimilarity + 1)) / 2;
		context.strokeStyle = phaseOutcomeColor(response.outcome.className);
		context.lineWidth = 1.5;
		context.beginPath();
		context.moveTo(
			centerX + Math.cos(response.angle) * responseRadius,
			centerY - Math.sin(response.angle) * responseRadius,
		);
		context.lineTo(
			centerX + Math.cos(nextAngle) * nextRadius,
			centerY - Math.sin(nextAngle) * nextRadius,
		);
		context.stroke();
		context.lineWidth = 3;
		context.beginPath();
		context.arc(centerX, centerY, radius, -response.angle, -nextAngle, true);
		context.stroke();
	}
};

/*
drawPhaseModes plots the current resident omega fingerprint at its actual
complex phase, independently of the historical response sectors.
*/
const drawPhaseModes = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
	wave: TerminalWaveMode[],
	amplitude: number,
) => {
	context.fillStyle = TERMINAL_COLORS.cyan;

	for (const mode of wave) {
		const magnitude = Math.hypot(mode.real, mode.imaginary) / amplitude;
		const phase = Math.atan2(mode.imaginary, mode.real);
		const pointRadius = radius * magnitude;
		context.fillRect(
			centerX + Math.cos(phase) * pointRadius - 1,
			centerY - Math.sin(phase) * pointRadius - 1,
			2,
			2,
		);
	}
};

/*
drawPhaseDial composes a categorical phase compass. The alignment ray rotates
to the sampled phase with the strongest constructive historical response.
*/
const drawPhaseDial = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	wave: TerminalWaveMode[],
	scan: TerminalPhaseResponse[],
	status: TerminalPhaseStatus,
) => {
	if (wave.length === 0) {
		return;
	}

	const radius = Math.min(64, Math.max(38, Math.min(width, height) * 0.14));
	const centerX = width - radius - 18;
	const centerY = height - radius - 18;
	const amplitude = Math.max(
		...wave.map((mode) => Math.hypot(mode.real, mode.imaginary)),
	);

	if (amplitude <= 0) {
		return;
	}

	context.save();
	drawPhaseAxes(context, centerX, centerY, radius);

	if (scan.length > 1) {
		drawPhaseResponse(context, centerX, centerY, radius, scan);
		const alignment = scan.reduce((best, response) =>
			response.similarity > best.similarity ? response : best,
		);
		context.strokeStyle = TERMINAL_COLORS.amber;
		context.lineWidth = 1.5;
		context.beginPath();
		context.moveTo(centerX, centerY);
		context.lineTo(
			centerX + Math.cos(alignment.angle) * radius,
			centerY - Math.sin(alignment.angle) * radius,
		);
		context.stroke();
		context.fillStyle = phaseOutcomeColor(alignment.outcome.className);
		context.beginPath();
		context.arc(
			centerX + Math.cos(alignment.angle) * radius,
			centerY - Math.sin(alignment.angle) * radius,
			3,
			0,
			Math.PI * 2,
		);
		context.fill();
	}

	drawPhaseModes(context, centerX, centerY, radius, wave, amplitude);

	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "9px JetBrains Mono, monospace";
	const alignment = scan.reduce<TerminalPhaseResponse | null>(
		(best, response) =>
			best === null || response.similarity > best.similarity ? response : best,
		null,
	);
	const angle = alignment === null ? 0 : (alignment.angle * 180) / Math.PI;
	const label =
		status.ready && alignment !== null
			? `phase compass · ${alignment.outcome.className} · α ${angle.toFixed(0)}° · ρ ${alignment.similarity.toFixed(2)}`
			: `phase compass · ${status.reason || "waiting"}`;
	context.fillText(label, centerX - radius, centerY - radius - 6);

	if (alignment !== null) {
		const ambiguity = alignment.outcome.ambiguous ? " · ambiguous" : "";
		context.fillText(
			`DMT ${(alignment.outcome.confidence * 100).toFixed(0)}% · cohort ${alignment.outcome.cohort}${ambiguity}`,
			centerX - radius,
			centerY - radius + 6,
		);
	}

	context.restore();
};

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
	lastManifoldBatch = value;
	paintTerminalFluidCompose(value, focusSymbol);
};

/*
repaintTerminalFluidChart redraws the retained field batch after binary lattice
or manifold_wave packets arrive. Those packets store only; without a redraw the
phase dial never sees the wave modes for the cut.
*/
export const repaintTerminalFluidChart = (focusSymbol: string) => {
	if (lastManifoldBatch === null) {
		return;
	}

	paintTerminalFluidCompose(lastManifoldBatch, focusSymbol);
};

const paintTerminalFluidCompose = (value: unknown, focusSymbol: string) => {
	const fieldCanvas = fluidFieldCanvasRef.current;
	const overlayCanvas = fluidOverlayCanvasRef.current;

	if (fieldCanvas === null || overlayCanvas === null) {
		return;
	}

	const overlay = resizeCanvas(overlayCanvas);

	if (overlay === null) {
		return;
	}

	const width = overlayCanvas.clientWidth;
	const height = overlayCanvas.clientHeight;
	const focusChanged = fluidFocus !== focusSymbol;

	if (focusChanged) {
		fluidFocus = focusSymbol;
		drawWaiting(overlay, width, height, "waiting for pilot-wave field");
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

	// This path currently paints the wave overlay only. An empty wave still clears
	// the overlay so stale dial state does not remain visible.
	if (wave.length === 0) {
		overlay.clearRect(0, 0, width, height);
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

	overlay.clearRect(0, 0, width, height);
	drawPhaseDial(overlay, width, height, wave, phaseScan, phaseStatus);
};

/*
TerminalFluidChart is the static canvas shell. DRAW paints via
paintTerminalFluidChart, and the field canvas only blits backend GPU images.
*/
export const TerminalFluidChart = () => {
	useEffect(() => {
		repaintTerminalFluidChart(fluidFocus);
	}, []);

	return (
		<div className="relative block size-full">
			<canvas
				ref={fluidFieldCanvasRef}
				className="absolute inset-0 size-full"
			/>
			<canvas
				ref={fluidOverlayCanvasRef}
				className="absolute inset-0 size-full"
			/>
		</div>
	);
};
