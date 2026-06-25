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
	drawTmpFluid,
	drawTmpHawkes,
	drawTmpManifold,
	drawTmpPrediction,
} from "#/components/terminal/tmp-draw";
import {
	clamp,
	createFluidSim,
	createHawkesSim,
	stepHawkesSim,
	type HawkesSim,
	type ManifoldPoint,
	type PredBuffer,
	type FluidSim,
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
	const canvasRef = useRef<HTMLCanvasElement | null>(null);
	const simRef = useRef<FluidSim | null>(null);

	if (simRef.current === null) {
		simRef.current = createFluidSim();
	}

	useEffect(() => {
		const canvas = canvasRef.current;
		if (canvas === null) {
			return;
		}

		let rafId: number;

		const loop = () => {
			const context = resizeCanvas(canvas);
			if (context !== null && simRef.current !== null) {
				drawTmpFluid(context, canvas.clientWidth, canvas.clientHeight, simRef.current);
			}
			rafId = requestAnimationFrame(loop);
		};

		rafId = requestAnimationFrame(loop);
		return () => cancelAnimationFrame(rafId);
	}, []);

	return <canvas ref={canvasRef} className="block size-full" />;
};

export const TerminalPredictionChart = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const resonanceFrame = useSelector(resonanceStore, (state) => state.frame);
	const resonance = useMemo(
		() => terminalResonanceFrame(resonanceFrame, focusSymbol),
		[resonanceFrame, focusSymbol],
	);

	const canvasRef = useRef<HTMLCanvasElement | null>(null);
	const bufferRef = useRef<PredBuffer>({ actual: [], pred: [] });

	useEffect(() => {
		const layer = resonance?.layers?.[0];
		if (!layer) return;

		const actVal = layer.state[0] !== undefined ? clamp((layer.state[0] + 1) / 2, 0.05, 0.95) : 0.5;
		const predVal = layer.prediction[0] !== undefined ? clamp((layer.prediction[0] + 1) / 2, 0.05, 0.95) : 0.5;

		const buf = bufferRef.current;
		buf.actual.push(actVal);
		buf.pred.push(predVal);

		if (buf.actual.length > 130) {
			buf.actual.shift();
			buf.pred.shift();
		}
	}, [resonance]);

	useEffect(() => {
		const canvas = canvasRef.current;
		if (canvas === null) {
			return;
		}

		let rafId: number;

		const loop = () => {
			const context = resizeCanvas(canvas);
			if (context !== null) {
				drawTmpPrediction(context, canvas.clientWidth, canvas.clientHeight, bufferRef.current);
			}
			rafId = requestAnimationFrame(loop);
		};

		rafId = requestAnimationFrame(loop);
		return () => cancelAnimationFrame(rafId);
	}, []);

	return <canvas ref={canvasRef} className="block size-full" />;
};

export const TerminalHawkesChart = () => {
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const hawkesFrame = useSelector(
		measurementsStore,
		(state) => state.readings.hawkes?.[focusSymbol] ?? null,
	);
	const metrics = useMemo(() => hawkesWireMetrics(hawkesFrame), [hawkesFrame]);
	const buf = (hawkesFrame?.history ?? hawkesFrame?.buf ?? []) as number[];

	const canvasRef = useRef<HTMLCanvasElement | null>(null);
	const hawkesRef = useRef<HawkesSim | null>(null);
	const bufRef = useRef<number[]>([]);

	if (hawkesRef.current === null) {
		hawkesRef.current = {
			mu: (metrics?.baseline as number) ?? 0.2,
			alpha: (metrics?.alpha as number) ?? 0.68,
			beta: (metrics?.beta as number) ?? 1.25,
			lam: (metrics?.intensity as number) ?? 0.2,
			events: [],
			buf: buf.length >= 2 ? [...buf] : Array.from({ length: 100 }, () => 0.2),
		};
	}

	useEffect(() => {
		if (metrics && hawkesRef.current) {
			hawkesRef.current.mu = metrics.baseline;
			hawkesRef.current.alpha = metrics.alpha;
			hawkesRef.current.beta = metrics.beta;
			if (metrics.intensity > 0) {
				hawkesRef.current.lam = metrics.intensity;
				hawkesRef.current.events.push(performance.now());
				if (hawkesRef.current.events.length > 80) {
					hawkesRef.current.events.shift();
				}
			}
		}

		if (metrics?.intensity !== undefined) {
			bufRef.current.push(metrics.intensity);
			if (bufRef.current.length > 220) {
				bufRef.current.shift();
			}
		}
	}, [metrics]);

	// Keep the simulation buffer in sync
	if (hawkesRef.current && buf.length < 2 && bufRef.current.length >= 2) {
		hawkesRef.current.buf = bufRef.current;
	}

	useEffect(() => {
		const canvas = canvasRef.current;
		if (canvas === null) {
			return;
		}

		let rafId: number;
		let lastStep = 0;

		const loop = (ts: number) => {
			if (ts - lastStep > 90) {
				if (hawkesRef.current) {
					stepHawkesSim(hawkesRef.current);
				}
				lastStep = ts;
			}

			const context = resizeCanvas(canvas);
			if (context !== null && hawkesRef.current !== null) {
				drawTmpHawkes(context, canvas.clientWidth, canvas.clientHeight, hawkesRef.current);
			}
			rafId = requestAnimationFrame(loop);
		};

		rafId = requestAnimationFrame(loop);
		return () => cancelAnimationFrame(rafId);
	}, []);

	return <canvas ref={canvasRef} className="block size-full" />;
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

	const canvasRef = useRef<HTMLCanvasElement | null>(null);

	useEffect(() => {
		const canvas = canvasRef.current;
		if (canvas === null) {
			return;
		}

		let rafId: number;

		const loop = () => {
			const context = resizeCanvas(canvas);
			if (context !== null) {
				drawTmpManifold(context, canvas.clientWidth, canvas.clientHeight, points, focusSymbol);
			}
			rafId = requestAnimationFrame(loop);
		};

		rafId = requestAnimationFrame(loop);
		return () => cancelAnimationFrame(rafId);
	}, [points, focusSymbol]);

	return <canvas ref={canvasRef} className="block size-full" />;
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
