import { createRef } from "react";
import type { ManifoldFrame } from "#/collections/types";
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
	drawFluidParticles,
	isFluidFieldMatrix,
	resolvePilotDisplayLattice,
} from "#/components/terminal/fluid-field";

const fluidCanvasRef = createRef<HTMLCanvasElement>();
let fluidContour = false;
let fluidFocus = "";

/*
drawPhaseDial overlays the resident complex modes and their historical corpus
scan. Mode points use their actual complex phase and normalized amplitude;
the line retains the scanner's signed response around one complete turn.
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
	context.lineWidth = 1;
	context.strokeStyle = "rgba(105, 177, 203, 0.34)";
	context.beginPath();
	context.arc(centerX, centerY, radius, 0, Math.PI * 2);
	context.stroke();

	if (scan.length > 1) {
		context.strokeStyle = TERMINAL_COLORS.amber;
		context.beginPath();

		scan.forEach((response, index) => {
			const responseRadius =
				radius * (0.5 + 0.45 * ((response.similarity + 1) / 2));
			const pointX = centerX + Math.cos(response.angle) * responseRadius;
			const pointY = centerY + Math.sin(response.angle) * responseRadius;

			if (index === 0) {
				context.moveTo(pointX, pointY);
				return;
			}

			context.lineTo(pointX, pointY);
		});

		context.closePath();
		context.stroke();
	}

	context.fillStyle = TERMINAL_COLORS.cyan;

	for (const mode of wave) {
		const magnitude = Math.hypot(mode.real, mode.imaginary) / amplitude;
		const phase = Math.atan2(mode.imaginary, mode.real);
		const pointRadius = radius * magnitude;
		context.fillRect(
			centerX + Math.cos(phase) * pointRadius - 1,
			centerY + Math.sin(phase) * pointRadius - 1,
			2,
			2,
		);
	}

	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "9px JetBrains Mono, monospace";
	const label = status.ready
		? "phase dial · historical response"
		: `phase dial · ${status.reason || "waiting"}`;
	context.fillText(label, centerX - radius, centerY - radius - 6);
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
	const display = resolvePilotDisplayLattice(
		isFluidFieldMatrix(rho) ? rho : [],
		psiMag2,
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
}: {
	contour?: boolean;
}) => {
	fluidContour = contour;

	return <canvas ref={fluidCanvasRef} className="block size-full" />;
};
