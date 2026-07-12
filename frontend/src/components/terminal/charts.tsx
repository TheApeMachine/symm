import { useSelector } from "@tanstack/react-store";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { appStore } from "#/collections/app";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import {
	clearCanvas,
	drawGrid,
	drawMatrix,
	drawPolyline,
	heatColor,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";

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

	const scalarRow = [
		frame?.bidTouchDensity,
		frame?.askTouchDensity,
		frame?.pressureGradX,
		frame?.pressureGradY,
		frame?.pressureGradZ,
		frame?.divergence,
		frame?.coherenceMag2,
		frame?.guidanceSpeed,
		frame?.stressAnisotropy,
	].filter((value): value is number =>
		typeof value === "number" && Number.isFinite(value),
	);

	if (scalarRow.length > 0) {
		return [scalarRow];
	}

	return [];
};

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

export type TerminalFluidParticle = {
	source: string;
	role: string;
	cellX: number;
	cellY: number;
	cellZ: number;
	phase: number;
	omega: number;
	amplitude: number;
	heat: number;
	velX: number;
	velY: number;
	velZ: number;
	speed: number;
};

const terminalFluidParticleFromRecord = (
	record: Record<string, unknown>,
): TerminalFluidParticle | null => {
	const cellX = finiteNumber(record.cell_x);
	const cellY = finiteNumber(record.cell_y);
	const cellZ = finiteNumber(record.cell_z);
	const phase = finiteNumber(record.phase);
	const omega = finiteNumber(record.omega);
	const amplitude = finiteNumber(record.amplitude);
	const heat = finiteNumber(record.heat);
	const velX = finiteNumber(record.vel_x);
	const velY = finiteNumber(record.vel_y);
	const velZ = finiteNumber(record.vel_z);

	if (
		cellX === null ||
		cellY === null ||
		cellZ === null ||
		phase === null ||
		omega === null ||
		amplitude === null ||
		heat === null ||
		velX === null ||
		velY === null ||
		velZ === null
	) {
		return null;
	}

	return {
		source: stringValue(record.source),
		role: stringValue(record.role),
		cellX,
		cellY,
		cellZ,
		phase,
		omega,
		amplitude,
		heat,
		velX,
		velY,
		velZ,
		speed: finiteNumber(record.speed) ?? Math.hypot(velX, velY, velZ),
	};
};

export const terminalFluidParticlesFromFrame = (
	frame: Record<string, unknown> | null | undefined,
): TerminalFluidParticle[] =>
	recordArray(frame?.particles).flatMap((record) => {
		const particle = terminalFluidParticleFromRecord(record);
		return particle === null ? [] : [particle];
	});

const matrixExtent = (matrix: number[][]): { min: number; max: number } => {
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

const matrixSample = (matrix: number[][], x: number, y: number): number => {
	const rows = matrix.length;
	const columns = matrix[0]?.length ?? 0;

	if (rows === 0 || columns === 0) {
		return 0;
	}

	const x0 = Math.max(0, Math.min(columns - 1, Math.floor(x)));
	const y0 = Math.max(0, Math.min(rows - 1, Math.floor(y)));
	const x1 = Math.max(0, Math.min(columns - 1, x0 + 1));
	const y1 = Math.max(0, Math.min(rows - 1, y0 + 1));
	const tx = x - x0;
	const ty = y - y0;
	const a = matrix[y0]?.[x0] ?? 0;
	const b = matrix[y0]?.[x1] ?? a;
	const c = matrix[y1]?.[x0] ?? a;
	const d = matrix[y1]?.[x1] ?? c;
	const top = a + (b - a) * tx;
	const bottom = c + (d - c) * tx;

	return top + (bottom - top) * ty;
};

const gridDimensions = (
	frame: Record<string, unknown> | null | undefined,
	matrix: number[][],
): { columns: number; rows: number } => {
	const grid = asRecord(frame?.grid);
	const gridX = finiteNumber(grid?.x);
	const gridZ = finiteNumber(grid?.z);
	const columns =
		gridX !== null && gridX > 0 ? gridX : (matrix[0]?.length ?? 0);
	const rows = gridZ !== null && gridZ > 0 ? gridZ : matrix.length;

	return { columns, rows };
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

const PREDICTION_HISTORY_LIMIT = 130;

export type TerminalPredictionSample = {
	key: string;
	symbol: string;
	actual: number;
	prediction: number;
	error: number;
};

export const terminalPredictionSampleFromFrame = (
	frame: unknown,
): TerminalPredictionSample | null => {
	const root = asRecord(frame);

	if (root === null) {
		return null;
	}

	const sensoryLayer = recordArray(root.layers).at(0);
	const actualValues = numberArray(sensoryLayer?.state ?? sensoryLayer?.State);
	const predictionValues = numberArray(
		sensoryLayer?.prediction ?? sensoryLayer?.Prediction,
	);
	const actual =
		actualValues.length > 0
			? actualValues.reduce((sum, value) => sum + value, 0) /
				actualValues.length
			: finiteNumber(root.flow);
	const prediction =
		predictionValues.length > 0
			? predictionValues.reduce((sum, value) => sum + value, 0) /
				predictionValues.length
			: finiteNumber(root.baseline);

	if (actual === null || prediction === null) {
		return null;
	}

	const symbol = stringValue(root.symbol) || "resonance";
	const timestamp = stringValue(root.at);
	const error = finiteNumber(root.surprise) ?? Math.abs(actual - prediction);

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
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const frame = useSelector(
		manifoldStore,
		(state) => state.manifold[focusSymbol]?.values().at(-1) ?? null,
	);
	const matrix = useMemo(() => frameMatrix(frame), [frame]);
	const particles = useMemo(
		() => terminalFluidParticlesFromFrame(frame),
		[frame],
	);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (matrix.length === 0 && particles.length === 0) {
				drawWaiting(context, width, height, "waiting for manifold field");
				return;
			}

			clearCanvas(context, width, height);

			const { columns, rows } = gridDimensions(frame, matrix);

			if (columns === 0 || rows === 0) {
				return;
			}

			if (matrix.length > 0) {
				const matrixColumns = matrix[0]?.length ?? 0;
				const matrixRows = matrix.length;
				const { min, max } = matrixExtent(matrix);
				const span = max - min || 1;
				// Sample the density field at a fine, fixed cadence rather than
				// scaling the block size with the canvas. The Rho matrix is coarse
				// (grid-resolution), so bilinear upsampling at ~2px reads the field
				// as the continuous fluid surface the solver actually computes,
				// instead of grid-sized blocks that look static and flat.
				const sampleSize = 2;

				for (let y = 0; y < height; y += sampleSize) {
					for (let x = 0; x < width; x += sampleSize) {
						const sourceX = (x / Math.max(width - 1, 1)) * (matrixColumns - 1);
						const sourceY = (y / Math.max(height - 1, 1)) * (matrixRows - 1);
						const value = matrixSample(matrix, sourceX, sourceY);
						let normalized = (value - min) / span;

						normalized = Math.max(0, Math.min(1, Math.sqrt(normalized)));

						if (contour) {
							normalized = Math.floor(normalized / 0.12) * 0.12;
						}

						context.fillStyle = heatColor(normalized);
						context.fillRect(x, y, sampleSize + 1, sampleSize + 1);
					}
				}
			} else {
				drawGrid(context, width, height);
			}

			const maxAmplitude = Math.max(
				1,
				...particles.map((particle) => Math.abs(particle.amplitude)),
			);
			const maxHeat = Math.max(
				1,
				...particles.map((particle) => Math.abs(particle.heat)),
			);
			const maxSpeed = Math.max(
				1,
				...particles.map((particle) => Math.abs(particle.speed)),
			);
			const cellWidth = width / Math.max(columns, 1);
			const cellHeight = height / Math.max(rows, 1);
			const cellSpan = Math.hypot(cellWidth, cellHeight);
			const occupancy = Math.max(1, particles.length);

			for (const particle of particles) {
				const x = (particle.cellX / Math.max(columns - 1, 1)) * width;
				const y = (particle.cellZ / Math.max(rows - 1, 1)) * height;
				const velocity = Math.hypot(particle.velX, particle.velZ);
				const directionX =
					velocity > 0 ? particle.velX / velocity : Math.cos(particle.phase);
				const directionY =
					velocity > 0 ? particle.velZ / velocity : Math.sin(particle.phase);
				const role = particle.role.toLowerCase();
				const whale = role.includes("whale");
				const amplitude = Math.abs(particle.amplitude) / maxAmplitude;
				const heat = Math.abs(particle.heat) / maxHeat;
				const speed = Math.abs(particle.speed) / maxSpeed;
				const length =
					cellSpan * (1 + (speed * Math.max(columns, rows)) / occupancy);
				const radius = cellSpan * (0.75 + amplitude + heat);
				const core = Math.max(
					1.6,
					Math.min(3.2, cellSpan * (0.08 + amplitude)),
				);
				const color = whale ? TERMINAL_COLORS.amber : TERMINAL_COLORS.cyan;
				const trail = context.createLinearGradient(
					x - directionX * length,
					y - directionY * length,
					x + directionX * length,
					y + directionY * length,
				);

				trail.addColorStop(0, "rgba(0,0,0,0)");
				trail.addColorStop(
					0.5,
					whale ? "rgba(232, 163, 61, 0.62)" : "rgba(127, 186, 203, 0.56)",
				);
				trail.addColorStop(1, "rgba(0,0,0,0)");

				context.save();
				context.globalCompositeOperation = "lighter";
				context.strokeStyle = trail;
				context.lineWidth = Math.max(1.4, cellSpan * (0.1 + amplitude));
				context.beginPath();
				context.moveTo(x - directionX * length, y - directionY * length);
				context.lineTo(x + directionX * length, y + directionY * length);
				context.stroke();

				const glow = context.createRadialGradient(x, y, 0, x, y, radius);

				glow.addColorStop(0, color);
				glow.addColorStop(1, "rgba(0,0,0,0)");
				context.fillStyle = glow;
				context.beginPath();
				context.arc(x, y, radius, 0, Math.PI * 2);
				context.fill();
				context.restore();

				context.fillStyle = TERMINAL_COLORS.foreground;
				context.beginPath();
				context.arc(x, y, core, 0, Math.PI * 2);
				context.fill();
			}
		},
		[frame, matrix, particles, contour],
	);

	return <StaticCanvas draw={draw} />;
};

export const TerminalPredictionChart = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const frame = useSelector(
		resonanceStore,
		(state) => state.resonance[focusSymbol]?.values().at(-1) ?? null,
	);
	const sample = useMemo(() => terminalPredictionSampleFromFrame(frame), [frame]);
	const [samples, setSamples] = useState<TerminalPredictionSample[]>([]);

	useEffect(() => {
		setSamples((previous) => {
			if (
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
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const frame = useSelector(
		measurementsStore,
		(state) => state.measurements[focusSymbol]?.hawkes?.values().at(-1) ?? null,
	);
	const output = frame?.metrics;
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
	frame: Record<string, unknown> | null,
): number[][] => {
	const latent = numberArray(frame?.latent);
	const modes = numberArray([frame?.flow, frame?.stress, frame?.coupling]);
	const energy = numberArray([frame?.baseline, frame?.energy, frame?.surprise]);

	return [latent, modes, energy].filter((row) => row.length > 0);
};

export const TerminalManifoldChart = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const frame = useSelector(
		manifoldStore,
		(state) => state.manifold[focusSymbol]?.values().at(-1) ?? null,
	);
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
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const frame = useSelector(
		resonanceStore,
		(state) => state.resonance[focusSymbol]?.values().at(-1) ?? null,
	);
	const matrix = terminalResonanceLayerMatrixFromFrame(frame);
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
			Object.values(readings.measurements).flatMap((sources) =>
				Object.values(sources).flatMap((history) =>
					history.values().flatMap((frame) => {
						const category = frame.categories.at(0);
						const value =
							kind === "confidence"
								? category?.confidence
								: category?.surprisal;

					return typeof value === "number" ? [[value]] : [];
					}),
				),
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
