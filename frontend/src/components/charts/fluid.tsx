import { createRef } from "react";
import type { ManifoldFrame } from "#/collections/types";
import type { FluidFieldLayer } from "#/collections/terminal";
import {
	clearCanvas,
	drawGrid,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	finiteNumber,
	fluidGridDimensions,
	frameAuxMatrix,
	frameReading,
	type TerminalPhaseResponse,
	type TerminalPhaseStatus,
	type TerminalWaveMode,
	terminalFluidParticlesFromFrame,
	terminalPhaseScanFromFrame,
	terminalPhaseStatusFromFrame,
	terminalWaveModesFromFrame,
} from "#/components/terminal/charts-frame";
import {
	drawFluidField,
	isFluidFieldMatrix,
	resolveFluidDisplayLattice,
} from "#/components/terminal/fluid-field";
import { drawFluidParticles } from "#/components/terminal/fluid-particles";

const fluidCanvasRef = createRef<HTMLCanvasElement>();
let fluidContour = false;
let fluidLayer: FluidFieldLayer = "Composite";
let fluidFocus = "";

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
		const nextAngle = index === scan.length - 1 ? next.angle + Math.PI * 2 : next.angle;
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
	const canvas = fluidCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	const width = canvas.clientWidth;
	const height = canvas.clientHeight;
	const focusChanged = fluidFocus !== focusSymbol;

	if (focusChanged) {
		fluidFocus = focusSymbol;
		drawWaiting(context, width, height, "waiting for pilot-wave field");
	}

	const frames = (
		Array.isArray(value) ? value : value != null ? [value] : []
	) as ManifoldFrame[];
	const frame =
		frames.find(
			(entry) => focusSymbol === "" || entry.symbol === focusSymbol,
		) ?? (focusSymbol === "" ? (frames.at(-1) ?? null) : null);

	if (frame === null) {
		return;
	}

	const rho = frameAuxMatrix(frame, "rho");
	const psiMag2 = frameAuxMatrix(frame, "psiMag2");
	const particles = terminalFluidParticlesFromFrame(frame);
	const display = resolveFluidDisplayLattice(
		isFluidFieldMatrix(rho) ? rho : [],
		psiMag2,
		fluidLayer,
	);
	const { columns, rows } = fluidGridDimensions(frame, display);
	const reading = frameReading(frame);
	const wave = terminalWaveModesFromFrame(frame);
	const phaseScan = terminalPhaseScanFromFrame(frame);
	const phaseStatus = terminalPhaseStatusFromFrame(frame);

	if (display.length === 0 && particles.length === 0) {
		return;
	}

	if (display.length > 0) {
		drawFluidField(
			context,
			width,
			height,
			isFluidFieldMatrix(rho) ? rho : [],
			fluidContour,
			{
				particles,
				pressureGradX:
					finiteNumber(frame?.pressureGradX) ??
					finiteNumber(reading?.pressureGradX) ??
					0,
				pressureGradZ:
					finiteNumber(frame?.pressureGradZ) ??
					finiteNumber(reading?.pressureGradZ) ??
					0,
				psiMag2,
				layer: fluidLayer,
				guidanceVelX: frameAuxMatrix(frame, "guidanceVelX"),
				guidanceVelZ: frameAuxMatrix(frame, "guidanceVelZ"),
			},
		);
	} else {
		clearCanvas(context, width, height);
		drawGrid(context, width, height);
	}

	drawFluidParticles(context, width, height, particles, columns, rows);
	drawPhaseDial(context, width, height, wave, phaseScan, phaseStatus);
};

/*
TerminalFluidChart is the static canvas shell. DRAW paints via paintTerminalFluidChart.
*/
export const TerminalFluidChart = ({
	contour = false,
	layer = "Composite",
}: {
	contour?: boolean;
	layer?: FluidFieldLayer;
}) => {
	fluidContour = contour;
	fluidLayer = layer;

	return <canvas ref={fluidCanvasRef} className="block size-full" />;
};
