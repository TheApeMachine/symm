import { createStore } from "@tanstack/store";
import { createRef, useEffect } from "react";
import {
	clearCanvas,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import type {
	TerminalPhaseStatus,
	TerminalWaveMode,
} from "#/components/terminal/charts-frame";
import type { FluidOscillator } from "#/components/fluid-3d/wire";

/*
The order-book phase dial is the live oscillator state, not an HCAM corpus
scan. Each order is plotted at its evolved phase with radius proportional to
its oscillator amplitude. Bid and ask resultant vectors show the amplitude-
weighted phase alignment within each side. The resident omega modes remain a
faint field context; they are not rotated or presented as retrieval results.
*/

const phaseDialCanvasRef = createRef<HTMLCanvasElement>();

type PhaseDialState = {
	oscillators: FluidOscillator[];
	wave: TerminalWaveMode[];
	status: TerminalPhaseStatus;
};

export type PhaseChannelResultant = {
	side: "bid" | "ask";
	count: number;
	totalAmplitude: number;
	coherence: number;
	phase: number;
};

const phaseDialStore = createStore<PhaseDialState>({
	oscillators: [],
	wave: [],
	status: { ready: false, reason: "" },
});

const phaseSides = ["bid", "ask"] as const;

/*
phaseChannelResultants computes each book side's weighted Kuramoto vector.
Vector length is normalized by that side's observed amplitude mass, while its
angle is the phase of the complex sum.
*/
export const phaseChannelResultants = (
	oscillators: FluidOscillator[],
): PhaseChannelResultant[] =>
	phaseSides.map((side) => {
		let real = 0;
		let imaginary = 0;
		let totalAmplitude = 0;
		let count = 0;

		for (const oscillator of oscillators) {
			if (oscillator.side !== side) {
				continue;
			}

			real += oscillator.amplitude * Math.cos(oscillator.phase);
			imaginary += oscillator.amplitude * Math.sin(oscillator.phase);
			totalAmplitude += oscillator.amplitude;
			count += 1;
		}

		return {
			side,
			count,
			totalAmplitude,
			coherence:
				totalAmplitude > 0
					? Math.hypot(real, imaginary) / totalAmplitude
					: 0,
			phase: count > 0 ? Math.atan2(imaginary, real) : 0,
		};
	});

const sideColor = (side: "bid" | "ask"): string =>
	side === "bid" ? TERMINAL_COLORS.green : TERMINAL_COLORS.red;

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

	context.strokeStyle = "rgba(147, 138, 126, 0.22)";
	context.beginPath();

	for (let degrees = 0; degrees < 360; degrees += 15) {
		const angle = (degrees * Math.PI) / 180;
		const innerRadius = degrees % 45 === 0 ? radius - 7 : radius - 4;
		context.moveTo(
			centerX + Math.cos(angle) * innerRadius,
			centerY - Math.sin(angle) * innerRadius,
		);
		context.lineTo(
			centerX + Math.cos(angle) * radius,
			centerY - Math.sin(angle) * radius,
		);
	}

	context.stroke();
	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "9px JetBrains Mono, monospace";
	context.textAlign = "center";
	context.textBaseline = "middle";

	for (const degrees of [0, 90, 180, 270]) {
		const angle = (degrees * Math.PI) / 180;
		context.fillText(
			`${degrees}°`,
			centerX + Math.cos(angle) * (radius + 12),
			centerY - Math.sin(angle) * (radius + 12),
		);
	}
};

const drawWaveModes = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
	wave: TerminalWaveMode[],
) => {
	const maximumMagnitude = Math.max(
		0,
		...wave.map((mode) => Math.hypot(mode.real, mode.imaginary)),
	);

	if (maximumMagnitude === 0) {
		return;
	}

	context.fillStyle = TERMINAL_COLORS.cyan;
	context.globalAlpha = 0.28;

	for (const mode of wave) {
		const magnitude = Math.hypot(mode.real, mode.imaginary);
		const phase = Math.atan2(mode.imaginary, mode.real);
		const pointRadius = radius * (magnitude / maximumMagnitude);
		context.fillRect(
			centerX + Math.cos(phase) * pointRadius - 1,
			centerY - Math.sin(phase) * pointRadius - 1,
			2,
			2,
		);
	}

	context.globalAlpha = 1;
};

const drawOscillators = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
	oscillators: FluidOscillator[],
) => {
	const maximumAmplitude = Math.max(
		0,
		...oscillators.map((oscillator) => oscillator.amplitude),
	);

	if (maximumAmplitude === 0) {
		return;
	}

	for (const oscillator of oscillators) {
		const pointRadius = radius * (oscillator.amplitude / maximumAmplitude);
		const pointX = centerX + Math.cos(oscillator.phase) * pointRadius;
		const pointY = centerY - Math.sin(oscillator.phase) * pointRadius;
		context.fillStyle = sideColor(oscillator.side);
		context.beginPath();
		context.arc(pointX, pointY, 2.5, 0, Math.PI * 2);
		context.fill();
	}
};

const drawResultants = (
	context: CanvasRenderingContext2D,
	centerX: number,
	centerY: number,
	radius: number,
	resultants: PhaseChannelResultant[],
) => {
	for (const resultant of resultants) {
		if (resultant.count === 0) {
			continue;
		}

		const endX =
			centerX + Math.cos(resultant.phase) * radius * resultant.coherence;
		const endY =
			centerY - Math.sin(resultant.phase) * radius * resultant.coherence;
		context.strokeStyle = sideColor(resultant.side);
		context.lineWidth = 2;
		context.beginPath();
		context.moveTo(centerX, centerY);
		context.lineTo(endX, endY);
		context.stroke();
		context.fillStyle = sideColor(resultant.side);
		context.beginPath();
		context.arc(endX, endY, 3.5, 0, Math.PI * 2);
		context.fill();
		context.font = "9px JetBrains Mono, monospace";
		context.textAlign = "center";
		context.fillText(resultant.side.toUpperCase(), endX, endY - 9);
	}
};

const drawReadout = (
	context: CanvasRenderingContext2D,
	centerX: number,
	baseline: number,
	resultants: PhaseChannelResultant[],
	status: TerminalPhaseStatus,
) => {
	context.textAlign = "center";
	context.font = "9px JetBrains Mono, monospace";

	if (!status.ready) {
		context.fillStyle = TERMINAL_COLORS.muted;
		context.fillText(status.reason, centerX, baseline);
		return;
	}

	for (const [index, resultant] of resultants.entries()) {
		const degrees = (resultant.phase * 180) / Math.PI;
		context.fillStyle = sideColor(resultant.side);
		context.fillText(
			`${resultant.side} n=${resultant.count} · ΣA ${resultant.totalAmplitude.toPrecision(3)} · R ${resultant.coherence.toFixed(3)} · θ ${degrees.toFixed(0)}°`,
			centerX,
			baseline + index * 13,
		);
	}
};

const drawPhaseDial = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	state: PhaseDialState,
) => {
	clearCanvas(context, width, height);
	const centerX = width / 2;
	const top = 46;
	const readoutHeight = 42;
	const bottom = height - readoutHeight;
	const centerY = (top + bottom) / 2;
	const radius = Math.max(
		18,
		Math.min((bottom - top) / 2 - 14, width / 2 - 34),
	);
	const resultants = phaseChannelResultants(state.oscillators);

	context.save();
	drawPhaseAxes(context, centerX, centerY, radius);
	drawWaveModes(context, centerX, centerY, radius, state.wave);
	drawOscillators(context, centerX, centerY, radius, state.oscillators);
	drawResultants(context, centerX, centerY, radius, resultants);
	drawReadout(context, centerX, height - 31, resultants, state.status);
	context.restore();
};

const repaint = () => {
	const canvas = phaseDialCanvasRef.current;

	if (canvas === null) {
		return;
	}

	const context = resizeCanvas(canvas);

	if (context === null) {
		return;
	}

	drawPhaseDial(
		context,
		canvas.clientWidth,
		canvas.clientHeight,
		phaseDialStore.state,
	);
};

/*
paintPhaseDial retains the latest resident cut so resize and remount events
repaint the same oscillator geometry instead of blanking the chart.
*/
export const paintPhaseDial = (state: PhaseDialState) => {
	phaseDialStore.setState(() => state);
	repaint();
};

export const TerminalPhaseDialChart = () => {
	useEffect(() => {
		const canvas = phaseDialCanvasRef.current;

		if (canvas === null) {
			return;
		}

		repaint();
		const observer = new ResizeObserver(repaint);
		observer.observe(canvas);
		const subscription = phaseDialStore.subscribe(repaint);

		return () => {
			observer.disconnect();
			subscription.unsubscribe();
		};
	}, []);

	return <canvas ref={phaseDialCanvasRef} className="block size-full" />;
};
