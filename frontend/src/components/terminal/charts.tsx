import { useSelector } from "@tanstack/react-store";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import { terminalStore } from "#/collections/terminal";
import {
	clearCanvas,
	drawGrid,
	drawMatrix,
	drawPolyline,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	resolveScopedFrame,
	type ScopedFrameSource,
} from "#/components/terminal/scoped-frame";

type Draw = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
) => void;

const asRecord = (value: unknown): Record<string, unknown> | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: null;

const numberArray = (value: unknown): number[] =>
	Array.isArray(value)
		? value.filter((item): item is number => typeof item === "number")
		: [];

const recordArray = (value: unknown): Record<string, unknown>[] =>
	Array.isArray(value)
		? value.flatMap((item) => {
				const record = asRecord(item);
				return record === null ? [] : [record];
			})
		: [];

const numberMatrix = (value: unknown): number[][] =>
	Array.isArray(value)
		? value.map((row) => numberArray(row)).filter((row) => row.length > 0)
		: [];

const frameOutput = (
	frame: Record<string, unknown> | null | undefined,
): Record<string, unknown> | null => asRecord(frame?.output);

const frameMatrix = (
	frame: Record<string, unknown> | null | undefined,
): number[][] => {
	const output = frameOutput(frame);

	for (const value of [
		frame?.rho,
		output?.rho,
		frame?.matrix,
		output?.matrix,
		frame?.grid,
		output?.grid,
	]) {
		const matrix = numberMatrix(value);

		if (matrix.length > 0) {
			return matrix;
		}
	}

	for (const value of [
		frame?.state,
		output?.state,
		frame?.values,
		output?.values,
	]) {
		const row = numberArray(value);

		if (row.length > 0) {
			return [row];
		}
	}

	return [];
};

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

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

const PREDICTION_HISTORY_LIMIT = 130;

export type TerminalPredictionSample = {
	key: string;
	symbol: string;
	actual: number;
	prediction: number;
	error: number;
};

const firstFinite = (values: number[]): number | null => {
	for (const value of values) {
		if (!Number.isFinite(value)) {
			continue;
		}

		return value;
	}

	return null;
};

const predictionFrameForSymbol = (
	root: Record<string, unknown>,
	focusSymbol?: string,
): Record<string, unknown> | null => {
	return resolveScopedFrame(root, focusSymbol, "resonance").frame;
};

export const terminalPredictionSampleFromFrame = (
	frame: unknown,
	focusSymbol?: string,
): TerminalPredictionSample | null => {
	const root = asRecord(frame);

	if (root === null) {
		return null;
	}

	const focus = predictionFrameForSymbol(root, focusSymbol);

	if (focus === null) {
		return null;
	}

	const symbol =
		stringValue(focus.symbol) ||
		stringValue(root.focus_symbol) ||
		stringValue(root.symbol) ||
		"resonance";
	const timestamp = stringValue(focus.ts) || stringValue(root.ts);

	for (const layer of recordArray(focus.layers)) {
		const actual = firstFinite(numberArray(layer.state));
		const prediction = firstFinite(numberArray(layer.prediction));

		if (actual === null || prediction === null) {
			continue;
		}

		const error =
			finiteNumber(layer.error_norm) ??
			finiteNumber(focus.surprise) ??
			finiteNumber(root.surprise) ??
			Math.abs(actual - prediction);

		return {
			key:
				timestamp === ""
					? `${symbol}:${actual}:${prediction}:${error}`
					: `${symbol}:${timestamp}`,
			symbol,
			actual,
			prediction,
			error,
		};
	}

	return null;
};

const StaticCanvas = ({ draw }: { draw: Draw }) => {
	const canvasRef = useRef<HTMLCanvasElement | null>(null);

	useEffect(() => {
		const canvas = canvasRef.current;

		if (canvas === null) {
			return;
		}

		const render = () => {
			const context = resizeCanvas(canvas);

			if (context === null) {
				return;
			}

			draw(context, canvas.clientWidth, canvas.clientHeight);
		};

		render();
		const observer = new ResizeObserver(render);
		observer.observe(canvas);

		return () => observer.disconnect();
	}, [draw]);

	return <canvas ref={canvasRef} className="block size-full" />;
};

export const TerminalFluidChart = ({
	contour = false,
}: {
	contour?: boolean;
}) => {
	const frame = useSelector(manifoldStore, (state) => state.frame);
	const matrix = useMemo(() => frameMatrix(frame), [frame]);
	const carriers = useMemo(() => recordArray(frame?.carriers), [frame]);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (matrix.length === 0) {
				drawWaiting(context, width, height, "waiting for manifold field");
				return;
			}

			drawMatrix(context, width, height, matrix, contour);

			const columns = matrix[0]?.length ?? 0;
			const rows = matrix.length;

			if (columns === 0 || rows === 0) {
				return;
			}

			for (const carrier of carriers) {
				const cellX = finiteNumber(carrier.cell_x);
				const cellZ = finiteNumber(carrier.cell_z);

				if (cellX === null || cellZ === null) {
					continue;
				}

				const role = String(carrier.role ?? "");
				const x = (cellX / Math.max(columns - 1, 1)) * width;
				const y = (cellZ / Math.max(rows - 1, 1)) * height;
				const radius = role === "whale" ? 4 : 2.5;

				context.fillStyle =
					role === "whale" ? TERMINAL_COLORS.amber : TERMINAL_COLORS.cyan;
				context.shadowColor = context.fillStyle;
				context.shadowBlur = role === "whale" ? 14 : 6;
				context.beginPath();
				context.arc(x, y, radius, 0, Math.PI * 2);
				context.fill();
				context.shadowBlur = 0;
			}
		},
		[matrix, carriers, contour],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalPredictionChart = () => {
	const frame = useSelector(resonanceStore, (state) => state.frame);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const sample = useMemo(
		() => terminalPredictionSampleFromFrame(frame, focusSymbol),
		[frame, focusSymbol],
	);
	const [samples, setSamples] = useState<TerminalPredictionSample[]>([]);

	useEffect(() => {
		setSamples((previous) => {
			if (
				focusSymbol === "stream" ||
				previous.length === 0 ||
				previous[previous.length - 1]?.symbol === focusSymbol
			) {
				return previous;
			}

			return [];
		});
	}, [focusSymbol]);

	useEffect(() => {
		if (sample === null) {
			return;
		}

		setSamples((previous) => {
			const last = previous[previous.length - 1];

			if (last?.symbol !== sample.symbol) {
				return [sample];
			}

			if (last.key === sample.key) {
				return previous;
			}

			return [...previous, sample].slice(-PREDICTION_HISTORY_LIMIT);
		});
	}, [sample]);

	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (samples.length === 0) {
				drawWaiting(context, width, height, "waiting for resonance history");
				return;
			}

			clearCanvas(context, width, height);
			drawGrid(context, width, height, 18);

			const values = samples
				.flatMap((entry) => [entry.actual, entry.prediction])
				.filter(Number.isFinite);

			if (values.length === 0) {
				return;
			}

			let min = Math.min(...values);
			let max = Math.max(...values);
			const span = max > min ? max - min : 1;
			const margin = span * 0.08;
			min -= margin;
			max += margin;
			const paddedSpan = max > min ? max - min : 1;
			const paddingX = 18;
			const plotWidth = Math.max(1, width - paddingX * 2);
			const plotHeight = Math.max(1, height - 46);
			const denominator = Math.max(samples.length - 1, 1);
			const xFor = (index: number) =>
				paddingX + (index / denominator) * plotWidth;
			const yFor = (value: number) =>
				height - 26 - ((value - min) / paddedSpan) * plotHeight;
			const actualPoints = samples.map((entry, index) => ({
				x: xFor(index),
				y: yFor(entry.actual),
			}));
			const predictionPoints = samples.map((entry, index) => ({
				x: xFor(index),
				y: yFor(entry.prediction),
			}));

			context.fillStyle = "rgba(232, 163, 61, 0.18)";
			context.beginPath();
			for (const [index, point] of actualPoints.entries()) {
				if (index === 0) {
					context.moveTo(point.x, point.y);
				} else {
					context.lineTo(point.x, point.y);
				}
			}
			for (let index = predictionPoints.length - 1; index >= 0; index -= 1) {
				const point = predictionPoints[index];
				context.lineTo(point.x, point.y);
			}
			context.closePath();
			context.fill();

			drawPolyline(context, actualPoints, TERMINAL_COLORS.foreground);
			drawPolyline(context, predictionPoints, TERMINAL_COLORS.cyan, true);

			const latest = samples[samples.length - 1];

			if (latest !== undefined) {
				context.fillStyle = TERMINAL_COLORS.amber;
				context.beginPath();
				context.arc(
					xFor(samples.length - 1),
					yFor(latest.actual),
					2.6,
					0,
					Math.PI * 2,
				);
				context.fill();
				context.fillStyle = TERMINAL_COLORS.muted;
				context.font = "10px JetBrains Mono, monospace";
				context.fillText(`ε ${latest.error.toFixed(4)}`, 18, height - 8);
			}
		},
		[samples],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalHawkesChart = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const frame = useSelector(measurementsStore, (state) => {
		if (focusSymbol !== "stream") {
			const frames = state.symbols[focusSymbol] ?? [];

			for (let index = frames.length - 1; index >= 0; index -= 1) {
				const measurement = frames[index];

				if (measurement.source === "hawkes") {
					return measurement;
				}
			}

			return null;
		}

		return state.measurements.hawkes?.values().at(-1) ?? null;
	});
	const output = frameOutput(frame);
	const values = numberArray([
		output?.baseline,
		output?.intensity,
		output?.buyIntensity,
		output?.sellIntensity,
		output?.branching,
		output?.radius,
	]);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (values.length === 0) {
				drawWaiting(context, width, height, "waiting for hawkes output");
				return;
			}

			drawMatrix(context, width, height, [values]);
		},
		[values],
	);

	return <StaticCanvas draw={draw} />;
};

export const terminalResonanceLayerMatrixFromFrame = (
	source: ScopedFrameSource,
	focusSymbol: string,
): number[][] => {
	const frame = resolveScopedFrame(source, focusSymbol, "resonance").frame;

	return recordArray(frame?.layers).flatMap((layer) => {
		const row = numberArray(layer.state);
		return row.length > 0 ? [row] : [];
	});
};

export const TerminalManifoldChart = () => {
	const frame = useSelector(manifoldStore, (state) => state.frame);
	const matrix = frameMatrix(frame);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (matrix.length === 0) {
				drawWaiting(context, width, height, "waiting for manifold rho");
				return;
			}

			drawMatrix(context, width, height, matrix);
		},
		[matrix],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalResonanceChart = () => {
	const state = useSelector(resonanceStore, (storeState) => storeState);
	const focusSymbol = useSelector(
		terminalStore,
		(storeState) => storeState.focusSymbol,
	);
	const matrix = terminalResonanceLayerMatrixFromFrame(state, focusSymbol);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (matrix.length === 0) {
				drawWaiting(context, width, height, "waiting for resonance layers");
				return;
			}

			drawMatrix(context, width, height, matrix);
		},
		[matrix],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalCognitiveChart = () => {
	const draw = useCallback<Draw>((context, width, height) => {
		drawWaiting(context, width, height, "waiting for cognitive frames");
	}, []);

	return <StaticCanvas draw={draw} />;
};

export const TerminalSignalHeatmap = ({
	kind,
}: {
	kind: "confidence" | "surprise";
}) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const matrix = useMemo(
		() =>
			Object.values(readings.measurements).flatMap((history) =>
				history.values().flatMap((frame) => {
					const output = frameOutput(frame);
					const value = frame[kind] ?? output?.[kind];
					return typeof value === "number" ? [[value]] : [];
				}),
			),
		[readings, kind],
	);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (matrix.length === 0) {
				drawWaiting(context, width, height, "waiting for signal readings");
				return;
			}

			drawMatrix(context, width, height, matrix);
		},
		[matrix],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalPositionChart = ({
	positions,
}: {
	positions: Array<{
		key: string;
		symbol: string;
		pnlPercentText: string;
		profitable: boolean;
	}>;
}) => {
	const draw = useCallback<Draw>(
		(context, width, height) => {
			clearCanvas(context, width, height);
			drawGrid(context, width, height);
			const rowHeight = Math.max(
				22,
				(height - 24) / Math.max(positions.length, 1),
			);

			positions.forEach((position, index) => {
				const y = 18 + index * rowHeight;
				const value = Number.parseFloat(position.pnlPercentText) || 0;
				const bar = Math.min(width * 0.42, Math.abs(value) * 18);
				const origin = width * 0.5;

				context.fillStyle = TERMINAL_COLORS.muted;
				context.font = "10px JetBrains Mono, monospace";
				context.fillText(position.symbol, 12, y + 4);
				context.fillStyle = position.profitable
					? TERMINAL_COLORS.green
					: TERMINAL_COLORS.red;
				context.fillRect(value >= 0 ? origin : origin - bar, y - 6, bar, 10);
			});
		},
		[positions],
	);

	return <StaticCanvas draw={draw} />;
};

export const terminalFluidMatrixFromFrame = frameMatrix;
