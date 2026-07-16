import { useSelector } from "@tanstack/react-store";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { appStore } from "#/collections/app";
import { manifoldStore } from "#/collections/manifold";
import {
	flattenMeasurementBuffer,
	measurementEpochs,
	measurementRaw,
	measurementsStore,
} from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import {
	clearCanvas,
	drawGrid,
	drawMatrix,
	drawPolyline,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import {
	drawFluidField,
	drawFluidWhaleCarriers,
	isFluidFieldMatrix,
	type TerminalFluidParticle,
} from "#/components/terminal/fluid-field";
import { MockupFluidCanvas } from "#/components/terminal/mockup-fluid-canvas";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";

export type { TerminalFluidParticle };

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

const frameReading = (
	frame: Record<string, unknown> | null | undefined,
): Record<string, unknown> | null =>
	asRecord(frame?.reading) ?? asRecord(frameOutput(frame)?.reading);

const frameMatrix = (
	frame: Record<string, unknown> | null | undefined,
): number[][] => {
	const output = frameOutput(frame);
	const reading = frameReading(frame);

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
		reading?.pressureGradX,
		reading?.pressureGradY,
		reading?.pressureGradZ,
		reading?.divergence,
		reading?.coherenceMag2,
		reading?.guidanceSpeed,
		reading?.viscosityProxy,
	].filter(
		(value): value is number =>
			typeof value === "number" && Number.isFinite(value),
	);

	if (scalarRow.length > 0) {
		return [scalarRow];
	}

	return [];
};

const frameAuxMatrix = (
	frame: Record<string, unknown> | null | undefined,
	field: string,
): number[][] => {
	const output = frameOutput(frame);

	for (const value of [frame?.[field], output?.[field]]) {
		const matrix = numberMatrix(value);

		if (matrix.length > 0) {
			return matrix;
		}
	}

	return [];
};

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

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

export const fluidGridDimensions = (
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
	const draw = useCallback<Draw>(
		(context, width, height) => {
			const focusSymbol = appStore.state.focusSymbol;
			const frame =
				manifoldStore.state.manifold[focusSymbol]?.values().at(-1) ?? null;
			const matrix = frameMatrix(frame);
			const particles = terminalFluidParticlesFromFrame(frame);
			const fieldMatrix = isFluidFieldMatrix(matrix) ? matrix : [];
			const { columns, rows } = fluidGridDimensions(frame, fieldMatrix);
			const reading = frameReading(frame);

			if (fieldMatrix.length === 0 && particles.length === 0) {
				drawWaiting(context, width, height, "waiting for manifold field");
				return;
			}

			if (fieldMatrix.length > 0) {
				drawFluidField(context, width, height, fieldMatrix, contour, {
					particles,
					pressureGradX:
						finiteNumber(frame?.pressureGradX) ??
						finiteNumber(reading?.pressureGradX) ??
						0,
					pressureGradZ:
						finiteNumber(frame?.pressureGradZ) ??
						finiteNumber(reading?.pressureGradZ) ??
						0,
					psiMag2: frameAuxMatrix(frame, "psiMag2"),
					guidanceVelX: frameAuxMatrix(frame, "guidanceVelX"),
					guidanceVelZ: frameAuxMatrix(frame, "guidanceVelZ"),
				});
			} else {
				clearCanvas(context, width, height);
				drawGrid(context, width, height);
			}

			drawFluidWhaleCarriers(context, width, height, particles, columns, rows);
		},
		[contour],
	);

	return (
		<MockupFluidCanvas
			draw={draw}
			stores={[manifoldStore, appStore]}
			deps={[contour]}
		/>
	);
};

/*
paintTerminalPredictionChart reads resonance history from the store and paints
actual versus predicted sensory state without going through React reconciliation.
*/
const paintTerminalPredictionChart = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
): void => {
	const focusSymbol = appStore.state.focusSymbol;
	const frames = resonanceStore.state.resonance[focusSymbol]?.values() ?? [];

	if (frames.length === 0) {
		drawWaiting(context, width, height, "waiting for resonance history");
		return;
	}

	clearCanvas(context, width, height);
	drawGrid(context, width, height, 18);

	const values = frames.flatMap((frame) => {
		const layer = frame.layers?.[0];

		if (layer === undefined) {
			return [];
		}

		return [...layer.state, ...layer.prediction].filter(Number.isFinite);
	});

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
	const denominator = Math.max(frames.length - 1, 1);
	const xFor = (index: number) => paddingX + (index / denominator) * plotWidth;
	const yFor = (value: number) =>
		height - 26 - ((value - min) / paddedSpan) * plotHeight;
	const actualPoints = frames.flatMap((frame, index) => {
		const state = frame.layers?.[0]?.state ?? [];
		const finite = state.filter(Number.isFinite);

		if (finite.length === 0) {
			return [];
		}

		const actual =
			finite.reduce((sum, value) => sum + value, 0) / finite.length;

		return [{ x: xFor(index), y: yFor(actual) }];
	});
	const predictionPoints = frames.flatMap((frame, index) => {
		const prediction = frame.layers?.[0]?.prediction ?? [];
		const finite = prediction.filter(Number.isFinite);

		if (finite.length === 0) {
			return [];
		}

		const value = finite.reduce((sum, entry) => sum + entry, 0) / finite.length;

		return [{ x: xFor(index), y: yFor(value) }];
	});

	if (actualPoints.length === 0 || predictionPoints.length === 0) {
		drawWaiting(context, width, height, "waiting for resonance layers");
		return;
	}

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

	const latest = frames.at(-1);
	const latestActual = actualPoints.at(-1);
	const surprise = latest?.surprise;

	if (
		latestActual !== undefined &&
		typeof surprise === "number" &&
		Number.isFinite(surprise)
	) {
		context.fillStyle = TERMINAL_COLORS.amber;
		context.beginPath();
		context.arc(latestActual.x, latestActual.y, 2.6, 0, Math.PI * 2);
		context.fill();
		context.fillStyle = TERMINAL_COLORS.muted;
		context.font = "10px JetBrains Mono, monospace";
		context.fillText(`ε ${surprise.toFixed(4)}`, 18, height - 8);
	}
};

export const TerminalPredictionChart = () => {
	const canvasRef = useRef<HTMLCanvasElement>(null);

	useDirectStorePaint(
		() => {
			const canvas = canvasRef.current;

			if (canvas === null) {
				return;
			}

			const context = resizeCanvas(canvas);

			if (context === null) {
				return;
			}

			paintTerminalPredictionChart(
				context,
				canvas.clientWidth,
				canvas.clientHeight,
			);
		},
		[resonanceStore, appStore],
		[],
	);

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

			paintTerminalPredictionChart(
				context,
				canvas.clientWidth,
				canvas.clientHeight,
			);
		};

		const observer = new ResizeObserver(render);
		observer.observe(canvas);

		return () => observer.disconnect();
	}, []);

	return <canvas ref={canvasRef} className="block size-full" />;
};

export const TerminalHawkesChart = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const history = useSelector(measurementsStore, (state) =>
		flattenMeasurementBuffer(state.measurements[focusSymbol]?.hawkes),
	);
	const epoch = measurementEpochs(history).at(-1);
	const typedValues =
		epoch === undefined
			? []
			: numberArray([
					measurementRaw(epoch, "baseline_intensity", "buy"),
					measurementRaw(epoch, "baseline_intensity", "sell"),
					measurementRaw(epoch, "conditional_intensity", "buy") ??
						measurementRaw(epoch, "arrival_rate", "buy"),
					measurementRaw(epoch, "conditional_intensity", "sell") ??
						measurementRaw(epoch, "arrival_rate", "sell"),
					measurementRaw(epoch, "spectral_radius"),
				]);
	const output = epoch?.at(-1)?.metrics;
	const values =
		typedValues.length > 0
			? typedValues
			: numberArray([
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
					flattenMeasurementBuffer(history).flatMap((frame) => {
						const category = frame.categories?.at(0);
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
