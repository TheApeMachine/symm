import { useSelector } from "@tanstack/react-store";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import { terminalStore } from "#/collections/terminal";
import {
	clearCanvas,
	drawGrid,
	drawMatrix,
	heatColor,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	hawkesWireMetrics,
	terminalFluidMatrix,
	terminalManifoldCarriers,
	terminalManifoldMatrix,
	terminalResonanceFrame,
} from "#/components/terminal/chart-data";
import { drawCognitiveTree } from "#/components/terminal/cognitive-viz";
import {
	drawTmpHawkes,
	drawTmpManifold,
	drawTmpPrediction,
} from "#/components/terminal/tmp-draw";
import type {
	HawkesSim,
	ManifoldPoint,
	PredBuffer,
} from "#/components/terminal/tmp-sim";

type Draw = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
) => void;

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
	context.fillText(message, 18, 28);
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
	const matrix = useSelector(measurementsStore, (state) => {
		const fluid = state.readings.fluid ?? {};
		const symbols = Object.entries(fluid).map(([symbol, frame]) => ({
			symbol,
			...frame,
		}));

		return symbols.length === 0 ? [] : terminalFluidMatrix({ symbols });
	});

	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (matrix.length === 0) {
				drawWaiting(context, width, height, "waiting for fluid field frame");

				return;
			}

			drawMatrix(context, width, height, matrix, contour);
		},
		[matrix, contour],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalPredictionChart = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const predictionFrame = useSelector(
		measurementsStore,
		(state) => state.readings.prediction?.[focusSymbol] ?? null,
	);

	const buffer = useMemo<PredBuffer>(
		() => ({
			actual: (predictionFrame?.actual as number[]) ?? [],
			pred:
				(predictionFrame?.prediction as number[]) ??
				(predictionFrame?.pred as number[]) ??
				[],
		}),
		[predictionFrame],
	);

	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (buffer.actual.length < 2) {
				drawWaiting(context, width, height, "waiting for prediction frames");

				return;
			}

			drawTmpPrediction(context, width, height, buffer);
		},
		[buffer],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalHawkesChart = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const hawkesFrame = useSelector(
		measurementsStore,
		(state) => state.readings.hawkes?.[focusSymbol] ?? null,
	);
	const metrics = useMemo(() => hawkesWireMetrics(hawkesFrame), [hawkesFrame]);
	const buf = (hawkesFrame?.history ?? hawkesFrame?.buf ?? []) as number[];

	const hawkes = useMemo<HawkesSim>(
		() => ({
			mu: (metrics?.baseline as number) ?? 0.2,
			alpha: (metrics?.alpha as number) ?? 0.6,
			beta: (metrics?.beta as number) ?? 1.25,
			lam: (metrics?.intensity as number) ?? 0,
			events: [],
			buf,
		}),
		[buf, metrics],
	);

	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (hawkes.buf.length < 2) {
				drawWaiting(context, width, height, "waiting for hawkes intensity");

				return;
			}

			drawTmpHawkes(context, width, height, hawkes);
		},
		[hawkes],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalManifoldChart = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const manifoldFrame = useSelector(
		measurementsStore,
		(state) => state.readings.manifold?.[focusSymbol] ?? null,
	);
	const points = useMemo<ManifoldPoint[]>(() => {
		const carriers = terminalManifoldCarriers(manifoldFrame, focusSymbol);

		if (carriers.length > 0) {
			return carriers;
		}

		const matrix = terminalManifoldMatrix(manifoldFrame);

		if (matrix.length === 0) {
			return [];
		}

		return matrix.flatMap((row, rowIndex) =>
			row.map((value, columnIndex) => ({
				symbol: `${rowIndex}${columnIndex}`,
				lx: (columnIndex / Math.max(row.length - 1, 1)) * 2 - 1,
				ly: (rowIndex / Math.max(matrix.length - 1, 1)) * 2 - 1,
				vol: value,
				cluster: (rowIndex + columnIndex) % 4,
			})),
		);
	}, [manifoldFrame, focusSymbol]);

	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (points.length === 0) {
				drawWaiting(context, width, height, "waiting for manifold snapshot");

				return;
			}

			drawTmpManifold(context, width, height, points, focusSymbol);
		},
		[focusSymbol, points],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalResonanceChart = () => {
	const frame = useSelector(resonanceStore, (state) => state.frame);
	const xray = useMemo(() => terminalResonanceFrame(frame), [frame]);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			clearCanvas(context, width, height);
			drawGrid(context, width, height, 20);

			if (xray === null) {
				drawWaiting(context, width, height, "waiting for resonance layers");

				return;
			}

			const rowHeight = (height - 42) / Math.max(xray.layers.length, 1);
			context.font = "10px JetBrains Mono, monospace";

			xray.layers.forEach((layer, layerIndex) => {
				const y = 24 + layerIndex * rowHeight;
				context.fillStyle = TERMINAL_COLORS.muted;
				context.fillText(`L${layerIndex} ${layer.errorNorm.toFixed(3)}`, 12, y);

				layer.state.forEach((value, index) => {
					const normalized = Math.max(0, Math.min(1, (value + 1) / 2));
					context.fillStyle = heatColor(normalized);
					context.fillRect(76 + index * 18, y - 10, 14, rowHeight * 0.55);
				});
			});
		},
		[xray],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalCognitiveChart = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const reading = useSelector(
		measurementsStore,
		(state) =>
			(state.readings.cognitive?.[focusSymbol] ?? null) as Record<
				string,
				unknown
			> | null,
	);

	const draw = useCallback<Draw>(
		(context, width, height) => {
			drawCognitiveTree(context, width, height, reading);
		},
		[reading],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalSignalHeatmap = ({
	kind,
}: {
	kind: "confidence" | "surprise";
}) => {
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const matrix = useMemo(
		() =>
			Object.values(readings).flatMap((scopes) =>
				Object.values(scopes).map((frame) => {
					const output = (frame.output ?? {}) as Record<string, unknown>;
					const confidence = (output.confidence as number) ?? 0;
					const surprise = (output.surprise as number) ?? 0;
					const value = kind === "confidence" ? confidence : surprise;

					return Array.from({ length: 32 }, (_, index) =>
						Math.max(0, Math.min(1, value * (0.4 + index / 48))),
					);
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

export const terminalFluidMatrixFromFrame = terminalFluidMatrix;
