import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { appStore } from "#/collections/app";
import {
	type CognitiveReading,
	cognitiveScopes,
	cognitiveStore,
} from "#/collections/cognitive";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import { terminalStore } from "#/collections/terminal";
import {
	clearCanvas,
	drawGrid,
	resizeCanvas,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import { XrayLayerRows } from "#/components/terminal/xray-layers";

type Draw = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
) => void;

type HawkesMetrics = {
	intensity: number | null;
	branching: number | null;
	radius: number | null;
	asymmetry: number | null;
	buyIntensity: number | null;
	sellIntensity: number | null;
	exo: number | null;
};

type HawkesSample = {
	key: string;
	symbol: string;
	intensity: number;
};

type LatentPoint = {
	key: string;
	symbol: string;
	x: number;
	y: number;
	category: string;
};

const asRecord = (value: unknown): Record<string, unknown> | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: null;

const recordArray = (value: unknown): Record<string, unknown>[] =>
	Array.isArray(value)
		? value.flatMap((item) => {
				const record = asRecord(item);
				return record === null ? [] : [record];
			})
		: [];

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const numberArray = (value: unknown): number[] =>
	Array.isArray(value)
		? value.filter((item): item is number => typeof item === "number")
		: [];

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

const format = (value: number | null, digits = 3): string =>
	value === null ? "—" : value.toFixed(digits);

const signed = (value: number | null, digits = 3): string => {
	if (value === null) {
		return "—";
	}

	return `${value >= 0 ? "+" : "−"}${Math.abs(value).toFixed(digits)}`;
};

const outputOf = (
	frame: Record<string, unknown> | null | undefined,
): Record<string, unknown> =>
	(asRecord(frame?.output) ?? {}) as Record<string, unknown>;

const outputNumber = (
	frame: Record<string, unknown> | null | undefined,
	key: string,
): number | null => finite(outputOf(frame)[key]) ?? finite(frame?.[key]);

const focusFrameForSymbol = (
	frame: Record<string, unknown> | null,
	symbol: string,
): Record<string, unknown> | null => {
	if (frame === null) {
		return null;
	}

	for (const snapshot of recordArray(frame.snapshots)) {
		if (stringValue(snapshot.symbol) === symbol) {
			return snapshot;
		}
	}

	const focus = asRecord(frame.focus);

	if (focus !== null) {
		if (
			symbol === "" ||
			symbol === "stream" ||
			stringValue(focus.symbol) === symbol
		) {
			return focus;
		}
	}

	if (
		stringValue(frame.symbol) === symbol ||
		stringValue(frame.scope) === symbol
	) {
		return frame;
	}

	return focus;
};

const symbolList = (
	resonance: Record<string, unknown> | null,
	readings: Record<string, Record<string, Record<string, unknown>>>,
): string[] => {
	const fromResonance = recordArray(resonance?.symbols)
		.map((entry) => stringValue(entry.symbol))
		.filter(Boolean);

	if (fromResonance.length > 0) {
		return fromResonance;
	}

	const symbols = new Set<string>();

	for (const bySymbol of Object.values(readings)) {
		for (const symbol of Object.keys(bySymbol)) {
			symbols.add(symbol);
		}
	}

	return [...symbols];
};

export const activeSymbolFor = (
	requested: string,
	resonance: Record<string, unknown> | null,
	symbols: string[],
): string => {
	if (
		requested !== "" &&
		requested !== "stream" &&
		symbols.includes(requested)
	) {
		return requested;
	}

	const focusSymbol =
		stringValue(resonance?.focus_symbol) ||
		stringValue(asRecord(resonance?.focus)?.symbol);

	return focusSymbol || symbols[0] || "stream";
};

const hawkesMetrics = (
	frame: Record<string, unknown> | undefined,
): HawkesMetrics => ({
	intensity: outputNumber(frame, "intensity"),
	branching: outputNumber(frame, "branching"),
	radius: outputNumber(frame, "radius"),
	asymmetry: outputNumber(frame, "asymmetry"),
	buyIntensity: outputNumber(frame, "buyIntensity"),
	sellIntensity: outputNumber(frame, "sellIntensity"),
	exo: outputNumber(frame, "exo"),
});

const hawkesSample = (
	frame: Record<string, unknown> | undefined,
	symbol: string,
): HawkesSample | null => {
	const intensity = outputNumber(frame, "intensity");

	if (intensity === null) {
		return null;
	}

	const key = String(
		frame?.timestamp ??
			frame?.ts ??
			frame?.updated_at ??
			`${symbol}:${intensity}`,
	);

	return { key, symbol, intensity };
};

export const hawkesSamplesFromFrame = (
	frame: Record<string, unknown> | undefined,
	symbol: string,
	limit = 120,
): HawkesSample[] => {
	if (frame === undefined) {
		return [];
	}

	const samples = recordArray(frame.history)
		.map((historyFrame) => hawkesSample(historyFrame, symbol))
		.filter((sample): sample is HawkesSample => sample !== null);
	const latest = hawkesSample(frame, symbol);

	if (latest !== null && samples[samples.length - 1]?.key !== latest.key) {
		samples.push(latest);
	}

	return samples.slice(-limit);
};

export const latentPointsFromFrame = (
	frame: Record<string, unknown> | null,
): LatentPoint[] =>
	recordArray(frame?.symbols).flatMap((entry, index) => {
		const latent = numberArray(entry.latent);
		const symbol = stringValue(entry.symbol);

		if (symbol === "" || latent.length < 2) {
			return [];
		}

		return [
			{
				key: `${symbol}:${index}`,
				symbol,
				x: latent[0] ?? 0,
				y: latent[1] ?? 0,
				category: stringValue(entry.category),
			},
		];
	});

const cascadeLabel = (
	branching: number | null,
): { label: string; color: string } => {
	if (branching === null) {
		return { label: "—", color: "var(--f4)" };
	}

	if (branching > 0.85) {
		return { label: "critical", color: "var(--down)" };
	}

	if (branching > 0.6) {
		return { label: "elevated", color: "var(--warn)" };
	}

	return { label: "stable", color: "var(--up)" };
};

const cognitiveForSymbol = (
	readings: Record<string, CognitiveReading>,
	symbol: string,
): CognitiveReading | null => {
	if (readings[symbol] !== undefined) {
		return readings[symbol];
	}

	const [scope] = cognitiveScopes(readings);

	return scope === undefined ? null : readings[scope];
};

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

	return (
		<canvas ref={canvasRef} className="absolute inset-0 block size-full" />
	);
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
	context.fillText(message, 18, Math.max(52, height * 0.34));
};

const categoryColor = (category: string, focus: boolean): string => {
	const normalized = category.toLowerCase();

	if (focus) {
		return TERMINAL_COLORS.amber;
	}

	if (normalized.includes("stress") || normalized.includes("turbulent")) {
		return TERMINAL_COLORS.red;
	}

	if (normalized.includes("flow") || normalized.includes("laminar")) {
		return TERMINAL_COLORS.green;
	}

	if (normalized.includes("coupling") || normalized.includes("equilibrium")) {
		return TERMINAL_COLORS.cyan;
	}

	return TERMINAL_COLORS.muted;
};

const latentRange = (
	points: LatentPoint[],
	key: "x" | "y",
): { min: number; span: number } => {
	const values = points.map((point) => point[key]);
	const min = Math.min(...values);
	const max = Math.max(...values);
	const span = max - min;

	if (!Number.isFinite(min) || !Number.isFinite(span) || span <= 0) {
		return { min: 0, span: 1 };
	}

	return { min, span };
};

const LatentScatter = ({
	frame,
	activeSymbol,
}: {
	frame: Record<string, unknown> | null;
	activeSymbol: string;
}) => {
	const points = useMemo(() => latentPointsFromFrame(frame), [frame]);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (points.length === 0) {
				drawWaiting(context, width, height, "waiting for latent carriers");
				return;
			}

			clearCanvas(context, width, height);

			const pad = 28;
			const xRange = latentRange(points, "x");
			const yRange = latentRange(points, "y");

			context.strokeStyle = TERMINAL_COLORS.line;
			context.lineWidth = 1;

			for (let index = 0; index <= 4; index += 1) {
				const x = pad + index * ((width - pad * 2) / 4);
				const y = pad + index * ((height - pad * 2) / 4);

				context.beginPath();
				context.moveTo(x, pad);
				context.lineTo(x, height - pad);
				context.stroke();
				context.beginPath();
				context.moveTo(pad, y);
				context.lineTo(width - pad, y);
				context.stroke();
			}

			for (const point of points) {
				const focus = point.symbol === activeSymbol;
				const x =
					pad + ((point.x - xRange.min) / xRange.span) * (width - pad * 2);
				const y =
					height -
					pad -
					((point.y - yRange.min) / yRange.span) * (height - pad * 2);
				const color = categoryColor(point.category, focus);

				context.fillStyle = color;
				context.globalAlpha = focus ? 1 : 0.72;
				context.shadowBlur = focus ? 12 : 4;
				context.shadowColor = color;
				context.beginPath();
				context.arc(x, y, focus ? 5 : 3.5, 0, Math.PI * 2);
				context.fill();
				context.shadowBlur = 0;
				context.globalAlpha = 1;

				if (focus) {
					context.strokeStyle = TERMINAL_COLORS.amber;
					context.lineWidth = 1.5;
					context.beginPath();
					context.arc(x, y, 9, 0, Math.PI * 2);
					context.stroke();
					context.fillStyle = TERMINAL_COLORS.foreground;
					context.font = "9px JetBrains Mono, monospace";
					context.fillText(
						point.symbol.split("/")[0] ?? point.symbol,
						x + 11,
						y + 4,
					);
				}
			}
		},
		[activeSymbol, points],
	);

	return <Canvas draw={draw} />;
};

const HawkesIntensityPanel = ({
	frame,
	activeSymbol,
	metrics,
	cascade,
}: {
	frame: Record<string, unknown> | undefined;
	activeSymbol: string;
	metrics: HawkesMetrics;
	cascade: { label: string; color: string };
}) => {
	const samples = useMemo(
		() => hawkesSamplesFromFrame(frame, activeSymbol),
		[activeSymbol, frame],
	);
	const draw = useCallback<Draw>(
		(context, width, height) => {
			if (samples.length === 0) {
				drawWaiting(context, width, height, "waiting for hawkes intensity");
				return;
			}

			clearCanvas(context, width, height);
			drawGrid(context, width, height, 18);

			const padX = 18;
			const padTop = 18;
			const padBottom = 28;
			const innerWidth = Math.max(1, width - padX * 2);
			const innerHeight = Math.max(1, height - padTop - padBottom);
			const maxIntensity = Math.max(
				1,
				...samples.map((sample) => sample.intensity),
			);
			const xFor = (index: number): number =>
				padX +
				(samples.length <= 1
					? innerWidth
					: (index / (samples.length - 1)) * innerWidth);
			const yFor = (intensity: number): number =>
				padTop + (1 - intensity / maxIntensity) * innerHeight;

			context.fillStyle = "rgba(232, 163, 61, 0.18)";
			context.beginPath();
			context.moveTo(padX, height - padBottom);
			samples.forEach((sample, index) => {
				context.lineTo(xFor(index), yFor(sample.intensity));
			});
			context.lineTo(width - padX, height - padBottom);
			context.closePath();
			context.fill();

			context.strokeStyle = TERMINAL_COLORS.amber;
			context.lineWidth = 1.8;
			context.beginPath();
			samples.forEach((sample, index) => {
				const x = xFor(index);
				const y = yFor(sample.intensity);

				if (index === 0) {
					context.moveTo(x, y);
					return;
				}

				context.lineTo(x, y);
			});
			context.stroke();

			const latest = samples[samples.length - 1];
			if (latest !== undefined) {
				const x = xFor(samples.length - 1);
				const y = yFor(latest.intensity);

				context.fillStyle = TERMINAL_COLORS.amber;
				context.shadowBlur = 10;
				context.shadowColor = TERMINAL_COLORS.amber;
				context.beginPath();
				context.arc(x, y, 3.5, 0, Math.PI * 2);
				context.fill();
				context.shadowBlur = 0;
				context.fillStyle = TERMINAL_COLORS.muted;
				context.font = "10px JetBrains Mono, monospace";
				context.fillText(`λ ${latest.intensity.toFixed(2)}`, 18, height - 9);
			}
		},
		[samples],
	);

	return (
		<div className="flex min-h-[300px] flex-1 flex-col border-(--line) border-t">
			<div className="flex items-start justify-between gap-3 px-[18px] pt-3 pb-2">
				<div>
					<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Hawkes self-exciting intensity
					</div>
					<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
						λ(t) = μ + Σ α·e^(-β(t-tᵢ)) · order-flow arrivals
					</div>
				</div>
				<div className="shrink-0 text-right font-mono text-[10px]">
					<div>
						<span className="text-(--f3)">η = α/β = </span>
						<span style={{ color: cascade.color }}>
							{format(metrics.branching)}
						</span>
					</div>
					<div>
						<span className="text-(--f3)">λ now </span>
						<span className="text-(--acc)">{format(metrics.intensity, 2)}</span>
					</div>
					<div style={{ color: cascade.color }}>cascade {cascade.label}</div>
				</div>
			</div>
			<div className="relative min-h-0 flex-1">
				<Canvas draw={draw} />
			</div>
		</div>
	);
};

const RowFact = ({
	label,
	value,
	accent,
}: {
	label: string;
	value: unknown;
	accent?: string;
}) => (
	<div className="flex justify-between gap-3">
		<span className="text-(--f3)">{label}</span>
		<span className="text-right" style={{ color: accent ?? "var(--f1)" }}>
			{value === undefined || value === null || value === ""
				? "—"
				: String(value)}
		</span>
	</div>
);

const RouteComponent = () => {
	const requestedFocus = useSelector(
		terminalStore,
		(state) => state.focusSymbol,
	);
	const readings = useSelector(measurementsStore, (state) => state);
	const resonance = useSelector(resonanceStore, (state) => state.frame);
	const manifold = useSelector(appStore, (state) => state.lastManifoldFrame);
	const cognitiveReadings = useSelector(
		cognitiveStore,
		(state) => state.readings,
	);
	const symbols = symbolList(resonance, readings);
	const activeSymbol = activeSymbolFor(requestedFocus, resonance, symbols);
	const focus = focusFrameForSymbol(resonance, activeSymbol);
	const layers = recordArray(focus?.layers);
	const hawkes = readings.hawkes?.[activeSymbol] as
		| Record<string, unknown>
		| undefined;
	const resonanceMeas = readings.resonance?.[activeSymbol] as
		| Record<string, unknown>
		| undefined;
	const cognitive = cognitiveForSymbol(cognitiveReadings, activeSymbol);
	const hawkesNow = hawkesMetrics(hawkes);
	const cascade = cascadeLabel(hawkesNow.branching);
	const reading = asRecord(manifold?.reading);
	const coherenceMag2 = finite(reading?.coherence_mag2);
	const coherence =
		coherenceMag2 === null
			? "—"
			: coherenceMag2 >= 0.4
				? "laminar"
				: "turbulent";
	const coherenceFg =
		coherence === "laminar"
			? "var(--info)"
			: coherence === "turbulent"
				? "var(--down)"
				: "var(--f4)";
	const freeEnergy =
		finite(focus?.energy) ?? outputNumber(resonanceMeas, "energy");
	const surprise =
		outputNumber(resonanceMeas, "surprise") ?? finite(focus?.surprise);
	const momentumShare = hawkesNow.radius ?? hawkesNow.branching ?? 0;
	const momentumFg = momentumShare >= 0.4 ? "var(--up)" : "var(--f3)";

	return (
		<div className="flex h-full min-w-[1100px] flex-col">
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
				<div className="flex min-h-0 flex-col overflow-auto border-(--line) border-r">
					<div className="shrink-0 px-[18px] py-4">
						<div className="flex items-baseline justify-between gap-3">
							<span className="font-serif font-semibold text-[22px] text-(--f1) leading-[1.1]">
								Predictive-coding hierarchy
							</span>
							<span
								data-symbol={activeSymbol}
								className="shrink-0 cursor-pointer font-mono text-[11px] text-(--f3)"
							>
								{activeSymbol}
							</span>
						</div>
						<div className="mt-1 font-mono text-[10px] text-(--f4)">
							latent state · prediction error ε per layer · macro = abstract
							regime, sensory = raw tape
						</div>
						<div className="mt-4">
							{layers.length > 0 ? (
								<XrayLayerRows layers={layers} />
							) : (
								<div className="font-mono text-[10px] text-(--f4)">
									waiting for resonance layers
								</div>
							)}
						</div>
					</div>
					<HawkesIntensityPanel
						frame={hawkes}
						activeSymbol={activeSymbol}
						metrics={hawkesNow}
						cascade={cascade}
					/>
				</div>

				<div className="flex min-h-0 flex-col overflow-auto bg-(--surface)">
					<div className="shrink-0 px-3.5 pt-3 pb-1.5">
						<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
							Latent manifold
						</div>
						<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
							universe embedding · clustered by regime · focus pulses
						</div>
					</div>
					<div className="relative mx-2 h-[300px] shrink-0">
						<LatentScatter frame={resonance} activeSymbol={activeSymbol} />
						<div className="pointer-events-none absolute bottom-1.5 left-2.5 font-mono text-[8.5px] text-(--f4)">
							latent-1 →
						</div>
						<div className="pointer-events-none absolute top-2.5 left-1.5 font-mono text-[8.5px] text-(--f4) [writing-mode:vertical-rl]">
							latent-2 →
						</div>
					</div>

					<div className="mt-2 flex flex-col gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
						<RowFact
							label="regime class"
							value={cognitive?.regimePrefix || stringValue(focus?.category)}
							accent="var(--acc)"
						/>
						<RowFact label="coherence" value={coherence} accent={coherenceFg} />
						<RowFact label="free energy" value={format(freeEnergy)} />
						<RowFact
							label="surprise"
							value={surprise === null ? "—" : `${surprise.toFixed(2)}× thr`}
						/>
						<RowFact
							label="flow events"
							value={
								hawkesNow.intensity === null
									? "—"
									: Math.round(hawkesNow.intensity)
							}
						/>
						<RowFact
							label="branching η"
							value={format(hawkesNow.branching)}
							accent={cascade.color}
						/>
					</div>

					<div className="flex flex-col gap-2 border-(--line) border-t px-3.5 py-3">
						<div>
							<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
								Manifold reading
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								navier–stokes · ρ projection · oscillator carriers
							</div>
						</div>
						<div className="grid grid-cols-2 gap-x-4 gap-y-2 font-mono text-[11px]">
							<RowFact
								label="∇·u"
								value={signed(finite(reading?.divergence))}
							/>
							<RowFact
								label="|ψ|²"
								value={format(finite(reading?.coherence_mag2))}
							/>
							<RowFact
								label="guide v"
								value={format(finite(reading?.guidance_speed))}
							/>
							<RowFact
								label="viscosity"
								value={format(finite(reading?.viscosity_proxy))}
							/>
						</div>
						<div className="mt-0.5">
							<div className="mb-1 flex justify-between text-[10px]">
								<span className="text-(--f3)">momentum eigenmode</span>
								<span className="font-mono" style={{ color: momentumFg }}>
									{momentumShare.toFixed(2)} / 0.40
								</span>
							</div>
							<div className="relative h-1.5 overflow-hidden rounded-sm bg-(--line)">
								<div
									className="h-full"
									style={{
										width: `${Math.min(100, momentumShare * 100)}%`,
										background: momentumFg,
									}}
								/>
							</div>
							<div className="relative h-0">
								<div className="absolute top-[-9px] left-[40%] h-3 w-0.5 bg-(--acc)" />
							</div>
							<div className="mt-1.5 font-mono text-[8.5px] text-(--f4)">
								drive playbook gate · mode share ≥ 0.40
							</div>
						</div>
					</div>
				</div>
			</div>
		</div>
	);
};

export const Route = createFileRoute("/xray")({
	component: RouteComponent,
});
