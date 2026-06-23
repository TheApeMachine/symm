import { useSelector } from "@tanstack/react-store";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { appStore } from "#/collections/app";
import type { CognitiveReading } from "#/collections/cognitive";
import { signalStore } from "#/collections/signals";
import {
	clamp01,
	clearCanvas,
	drawGrid,
	drawPolyline,
	heatColor,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	appendPredictionFrame,
	emptyPredictionSeries,
	type PredictionSeries,
	resetTerminalFluidMatrix,
	terminalFluidMatrix,
	terminalManifoldMatrix,
	terminalResonanceFrame,
} from "#/components/terminal/chart-data";
import type { TerminalPositionRow } from "#/components/terminal/model";

type Draw = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
) => void;

const Canvas = ({ draw }: { draw: Draw }) => {
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

const matrixExtent = (matrix: number[][]) => {
	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;

	for (const row of matrix) {
		for (const value of row) {
			if (!Number.isFinite(value)) {
				continue;
			}

			min = Math.min(min, value);
			max = Math.max(max, value);
		}
	}

	if (!Number.isFinite(min) || !Number.isFinite(max) || max <= min) {
		return { min: 0, max: 1 };
	}

	return { min, max };
};

const drawMatrix = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	matrix: number[][],
) => {
	clearCanvas(context, width, height);

	if (matrix.length === 0 || (matrix[0]?.length ?? 0) === 0) {
		drawGrid(context, width, height);
		return;
	}

	const { min, max } = matrixExtent(matrix);
	const rows = matrix.length;
	const columns = matrix[0]?.length ?? 0;
	const cellWidth = width / columns;
	const cellHeight = height / rows;

	for (let rowIndex = 0; rowIndex < rows; rowIndex += 1) {
		for (let columnIndex = 0; columnIndex < columns; columnIndex += 1) {
			const value = matrix[rowIndex]?.[columnIndex] ?? min;
			const normalized = (value - min) / (max - min);
			context.fillStyle = heatColor(normalized);
			context.fillRect(
				columnIndex * cellWidth,
				rowIndex * cellHeight,
				cellWidth + 1,
				cellHeight + 1,
			);
		}
	}
};

const useFluidMatrix = () => {
	const [matrix, setMatrix] = useState<number[][]>([]);

	useEffect(() => {
		resetTerminalFluidMatrix();

		const update = (frame: Record<string, unknown>) => {
			setMatrix(terminalFluidMatrix(frame));
		};

		appStore.actions.updateFluidUpdater(update);

		return () => appStore.actions.updateFluidUpdater(null);
	}, []);

	return matrix;
};

export const TerminalFluidChart = () => {
	const matrix = useFluidMatrix();
	const draw = useCallback<Draw>(
		(context, width, height) => drawMatrix(context, width, height, matrix),
		[matrix],
	);

	return <Canvas draw={draw} />;
};

const usePredictionSeries = () => {
	const [series, setSeries] = useState<PredictionSeries>(emptyPredictionSeries);

	useEffect(() => {
		const update = (frame: Record<string, unknown>) => {
			setSeries((current) => appendPredictionFrame(current, frame));
		};

		appStore.actions.updatePredictionUpdater(update);

		return () => appStore.actions.updatePredictionUpdater(null);
	}, []);

	return series;
};

const scalePrediction = (
	points: Array<{ x: number; value: number }>,
	width: number,
	height: number,
) => {
	const allValues = points.map((point) => point.value);
	const min = Math.min(...allValues, 0);
	const max = Math.max(...allValues, 1);
	const span = max > min ? max - min : 1;
	const startX = points[0]?.x ?? 0;
	const endX = points.at(-1)?.x ?? startX + 1;
	const xSpan = endX > startX ? endX - startX : 1;
	const pad = 14;

	return points.map((point) => ({
		x: pad + ((point.x - startX) / xSpan) * (width - pad * 2),
		y: height - 26 - ((point.value - min) / span) * (height - 46),
	}));
};

export const TerminalPredictionChart = () => {
	const series = usePredictionSeries();
	const draw = useCallback<Draw>(
		(context, width, height) => {
			clearCanvas(context, width, height);
			drawGrid(context, width, height);
			drawPolyline(
				context,
				scalePrediction(series.prediction, width, height),
				TERMINAL_COLORS.cyan,
				true,
			);
			drawPolyline(
				context,
				scalePrediction(series.error, width, height),
				TERMINAL_COLORS.red,
			);
			drawPolyline(
				context,
				scalePrediction(series.actual, width, height),
				TERMINAL_COLORS.foreground,
			);
		},
		[series],
	);

	return <Canvas draw={draw} />;
};

export const TerminalSignalHeatmap = ({
	kind,
}: {
	kind: "confidence" | "surprise";
}) => {
	const readings = useSelector(signalStore, (state) => state.readings);
	const matrix = useMemo(
		() =>
			Object.values(readings).map((reading) => {
				const value =
					kind === "confidence"
						? reading.confidence
						: reading.surprise / Math.max(reading.surpriseThreshold, 1);

				return Array.from({ length: 32 }, (_, index) =>
					clamp01(value * (0.4 + index / 48)),
				);
			}),
		[readings, kind],
	);
	const draw = useCallback<Draw>(
		(context, width, height) => drawMatrix(context, width, height, matrix),
		[matrix],
	);

	return <Canvas draw={draw} />;
};

export const TerminalManifoldChart = () => {
	const frame = useSelector(appStore, (state) => state.lastManifoldFrame);
	const matrix = useMemo(() => terminalManifoldMatrix(frame), [frame]);
	const draw = useCallback<Draw>(
		(context, width, height) => drawMatrix(context, width, height, matrix),
		[matrix],
	);

	return <Canvas draw={draw} />;
};

export const TerminalResonanceChart = () => {
	const frame = useSelector(appStore, (state) => state.lastResonanceFrame);
	const xray = useMemo(() => terminalResonanceFrame(frame), [frame]);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			clearCanvas(context, width, height);
			drawGrid(context, width, height, 20);

			if (xray === null) {
				return;
			}

			const rowHeight = (height - 42) / Math.max(xray.layers.length, 1);
			context.font = "10px monospace";

			xray.layers.forEach((layer, layerIndex) => {
				const y = 24 + layerIndex * rowHeight;
				context.fillStyle = TERMINAL_COLORS.muted;
				context.fillText(`L${layerIndex} ${layer.errorNorm.toFixed(3)}`, 12, y);

				layer.state.forEach((value, index) => {
					const normalized = clamp01((value + 1) / 2);
					context.fillStyle = heatColor(normalized);
					context.fillRect(76 + index * 18, y - 10, 14, rowHeight * 0.55);
				});
			});
		},
		[xray],
	);

	return <Canvas draw={draw} />;
};

export const TerminalCognitiveChart = ({
	reading,
}: {
	reading: CognitiveReading | null;
}) => {
	const draw = useCallback<Draw>(
		(context, width, height) => {
			clearCanvas(context, width, height);

			const tokens = (reading?.sequence ?? "")
				.split(/[/.>\s-]+/)
				.filter(Boolean)
				.slice(0, 18);
			const centerX = width * 0.5;
			const centerY = height * 0.18;

			context.strokeStyle = TERMINAL_COLORS.lineStrong;
			context.fillStyle = TERMINAL_COLORS.amber;
			context.font = "10px monospace";
			context.beginPath();
			context.arc(centerX, centerY, 8, 0, Math.PI * 2);
			context.fill();

			tokens.forEach((token, index) => {
				const depth = 1 + Math.floor(index / 6);
				const slot = index % 6;
				const x = width * (0.14 + slot * 0.145);
				const y = centerY + depth * 72;

				context.beginPath();
				context.moveTo(centerX, centerY + 8);
				context.lineTo(x, y - 8);
				context.stroke();
				context.fillStyle =
					index % 2 === 0 ? TERMINAL_COLORS.cyan : TERMINAL_COLORS.green;
				context.beginPath();
				context.arc(x, y, 7, 0, Math.PI * 2);
				context.fill();
				context.fillStyle = TERMINAL_COLORS.foreground;
				context.fillText(token.slice(0, 10), x + 10, y + 3);
			});
		},
		[reading],
	);

	return <Canvas draw={draw} />;
};

export const TerminalPositionChart = ({
	positions,
}: {
	positions: TerminalPositionRow[];
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
				context.font = "10px monospace";
				context.fillText(position.symbol, 12, y + 4);
				context.fillStyle = position.profitable
					? TERMINAL_COLORS.green
					: TERMINAL_COLORS.red;
				context.fillRect(value >= 0 ? origin : origin - bar, y - 6, bar, 10);
			});
		},
		[positions],
	);

	return <Canvas draw={draw} />;
};
