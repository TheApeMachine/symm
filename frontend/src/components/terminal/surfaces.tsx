import { useSelector } from "@tanstack/react-store";
import { useMemo } from "react";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import { terminalStore } from "#/collections/terminal";
import { AllocationSidePanel } from "#/components/terminal/allocation-side";
import {
	hawkesWireMetrics,
	terminalManifoldReading,
	terminalResonanceFrame,
} from "#/components/terminal/chart-data";
import {
	TerminalCognitiveChart,
	TerminalHawkesChart,
	TerminalManifoldChart,
} from "#/components/terminal/charts";
import {
	CortexBeamList,
	CortexSidePanels,
} from "#/components/terminal/cortex-panels";
import { DashboardSurface } from "#/components/terminal/dashboard";
import { DecisionTreeView } from "#/components/terminal/decision";
import { HealthPanel, RadarPanel } from "#/components/terminal/health";
import type { TerminalSurface } from "#/components/terminal/model";
import { KernelList } from "#/components/terminal/rows";
import { AllocationView, SignalDetail } from "#/components/terminal/widgets";
import { XrayLayerRows } from "#/components/terminal/xray-layers";

const INSIGHT_FEATURED_SOURCES = [
	"fluid",
	"prediction",
	"hawkes",
	"causal",
	"manifold",
	"correlation",
	"pumpdump",
	"liquidity",
] as const;

const ContextStrip = ({
	label,
	symbols,
	meta,
	activeSymbol,
	onSelect,
}: {
	label: string;
	symbols: string[];
	meta?: string;
	activeSymbol?: string;
	onSelect?: (symbol: string) => void;
}) => (
	<div className="flex h-[46px] shrink-0 items-center gap-2 overflow-x-auto border-(--line) border-b bg-(--surface) px-3.5">
		<span className="mr-1 shrink-0 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
			{label}
		</span>
		{symbols.map((symbol) => {
			const active = activeSymbol === symbol;

			return (
				<button
					key={symbol}
					type="button"
					onClick={() => onSelect?.(symbol)}
					className="shrink-0 cursor-pointer rounded-[3px] border px-[11px] py-1 font-mono text-[11px] font-medium"
					style={{
						borderColor: active ? "var(--acc)" : "var(--line)",
						background: active
							? "color-mix(in srgb, var(--acc) 14%, transparent)"
							: "transparent",
						color: active ? "var(--acc)" : "var(--f3)",
					}}
				>
					{symbol.split("/")[0] ?? symbol}
				</button>
			);
		})}
		{meta ? (
			<span className="ml-auto shrink-0 font-mono text-[10px] text-(--f4)">
				{meta}
			</span>
		) : null}
	</div>
);

const SignalSurface = () => (
	<div className="grid h-full min-w-[1080px] grid-cols-[230px_minmax(420px,1fr)_320px]">
		<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
			<div className="sticky top-0 border-(--line) border-b bg-(--surface) px-3 py-2.5 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
				Kernels
			</div>
			<KernelList origins={[...INSIGHT_FEATURED_SOURCES]} compact />
		</div>
		<div className="min-h-0 overflow-auto bg-(--bg)">
			<SignalDetail />
		</div>
		<div className="min-h-0 space-y-3.5 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
			<HealthPanel />
			<RadarPanel />
		</div>
	</div>
);

const XraySurface = () => {
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const { selectFocusSymbol } = terminalStore.actions;
	const playbookEvaluations = useSelector(
		appStore,
		(state) => state.playbookEvaluations,
	);
	const scopeSymbols = [
		...new Set(
			Object.values(readings).flatMap((scopes) =>
				Object.keys(scopes).filter((scope) => scope.includes("/")),
			),
		),
	];
	const resonanceFrame = useSelector(resonanceStore, (state) => state.frame);
	const hawkesFrame = readings.hawkes?.[focusSymbol] as
		| Record<string, unknown>
		| undefined;
	const manifoldFrame = readings.manifold?.[focusSymbol] as
		| Record<string, unknown>
		| undefined;
	const resonanceReading = readings.resonance?.[focusSymbol] as
		| Record<string, unknown>
		| undefined;
	const hawkesOutput = hawkesFrame?.output as
		| Record<string, unknown>
		| undefined;
	const resonanceOutput = resonanceReading?.output as
		| Record<string, unknown>
		| undefined;
	const hawkesSurprise = (hawkesOutput?.surprise as number) ?? 0;
	const resonanceSurprise = (resonanceOutput?.surprise as number) ?? 0;
	const resonance = useMemo(
		() => terminalResonanceFrame(resonanceFrame, focusSymbol),
		[resonanceFrame, focusSymbol],
	);
	const totalErr = (resonance?.layers ?? []).reduce(
		(sum, layer) => sum + layer.errorNorm,
		0,
	);
	const coherenceLabel = totalErr < 1.2 ? "laminar" : "turbulent";
	const coherenceTone = totalErr < 1.2 ? "var(--info)" : "var(--down)";
	const hawkesMetrics = useMemo(
		() =>
			hawkesWireMetrics(
				hawkesFrame as Record<string, unknown> | null | undefined,
			),
		[hawkesFrame],
	);
	const etaValue = (hawkesMetrics?.branching ??
		hawkesMetrics?.eta ??
		0) as number;
	const eta = etaValue.toFixed(3);
	const cascadeTone =
		etaValue > 0.85
			? "var(--down)"
			: etaValue > 0.6
				? "var(--warn)"
				: "var(--up)";
	const cascadeLabel =
		etaValue > 0.85 ? "critical" : etaValue > 0.6 ? "elevated" : "stable";
	const enginePhase = useSelector(appStore, (state) => state.enginePhase);
	const cognitiveFrame = readings.cognitive?.[focusSymbol] as
		| Record<string, unknown>
		| undefined;
	const regimeClass =
		(cognitiveFrame?.regimePrefix as string) || enginePhase || "stream";
	const reading =
		terminalManifoldReading(manifoldFrame ?? null) ??
		({
			divergence: "—",
			coherence: "—",
			guidance: "—",
			viscosity: "—",
			momentumShare: "0.00",
			momentumPct: 0,
			momentumGate: null,
			momentumGatePct: null,
			momentumFg: "var(--f3)",
		} as const);

	return (
		<div className="flex h-full min-w-[1100px] flex-col">
			<ContextStrip
				label="Inspect symbol"
				symbols={
					scopeSymbols.length > 0 ? scopeSymbols.slice(0, 10) : ["stream"]
				}
				activeSymbol={focusSymbol}
				onSelect={selectFocusSymbol}
			/>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(520px,1fr)_352px]">
				<div className="flex min-h-0 flex-col overflow-auto border-(--line) border-r">
					<div className="px-[18px] py-4">
						<div className="flex items-baseline justify-between gap-3">
							<span className="font-serif font-semibold text-[22px] text-(--f1) leading-[1.1]">
								Predictive-coding hierarchy
							</span>
							<span className="font-mono text-[11px] text-(--f3)">
								{focusSymbol}
							</span>
						</div>
						<div className="mt-1 font-mono text-[10px] text-(--f4)">
							latent state · prediction error ε per layer · macro = abstract
							regime, sensory = raw tape
						</div>
						<div className="mt-4">
							{resonance ? (
								<XrayLayerRows layers={resonance.layers} />
							) : (
								<div className="font-mono text-[10px] text-(--f4)">
									waiting for resonance layers
								</div>
							)}
						</div>
					</div>
					<div className="relative min-h-[210px] flex-1 border-(--line) border-t">
						<TerminalHawkesChart />
						<div className="pointer-events-none absolute top-3 left-3.5">
							<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
								Hawkes self-exciting intensity
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								λ(t) = μ + Σ α·e^(−β(t−tᵢ)) · order-flow arrivals
							</div>
						</div>
						<div className="pointer-events-none absolute top-3 right-3.5 text-right font-mono text-[9.5px] text-(--f3) leading-[1.7]">
							<div>
								η = α/β = <span style={{ color: cascadeTone }}>{eta}</span>
							</div>
							<div>
								λ now {(hawkesMetrics?.intensity ?? hawkesSurprise).toFixed(2)}
							</div>
							<div>
								cascade{" "}
								<span style={{ color: cascadeTone }}>{cascadeLabel}</span>
							</div>
						</div>
					</div>
				</div>
				<div className="flex min-h-0 flex-col overflow-auto bg-(--surface)">
					<div className="px-3.5 pt-3 pb-1.5">
						<div className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
							Latent manifold
						</div>
						<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
							universe embedding · clustered by regime · focus pulses
						</div>
					</div>
					<div className="relative mx-2 h-[300px]">
						<TerminalManifoldChart />
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
							value={regimeClass}
							accent="var(--acc)"
						/>
						<RowFact
							label="coherence"
							value={coherenceLabel}
							accent={coherenceTone}
						/>
						<RowFact label="free energy" value={totalErr.toFixed(3)} />
						<RowFact
							label="surprise"
							value={`${resonanceSurprise.toFixed(2)}× thr`}
						/>
						<RowFact
							label="flow events"
							value={playbookEvaluations.toString()}
						/>
						<RowFact label="branching η" value={eta} accent={cascadeTone} />
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
							<RowFact label="∇·u" value={reading.divergence} />
							<RowFact label="|ψ|²" value={reading.coherence} />
							<RowFact label="guide v" value={reading.guidance} />
							<RowFact label="viscosity" value={reading.viscosity} />
						</div>
						<div>
							<div className="mb-1 flex justify-between text-[10px]">
								<span className="text-(--f3)">momentum eigenmode</span>
								<span
									className="font-mono"
									style={{ color: reading.momentumFg }}
								>
									{reading.momentumGate === null
										? reading.momentumShare
										: `${reading.momentumShare} / ${reading.momentumGate}`}
								</span>
							</div>
							<div className="relative h-1.5 overflow-hidden rounded-[3px] bg-(--line)">
								<div
									className="h-full"
									style={{
										width: `${reading.momentumPct}%`,
										background: reading.momentumFg,
									}}
								/>
							</div>
							{reading.momentumGatePct !== null ? (
								<div className="relative h-0">
									<div
										className="absolute top-[-9px] h-3 w-0.5 bg-(--acc)"
										style={{ left: `${reading.momentumGatePct}%` }}
									/>
								</div>
							) : null}
							{reading.momentumGate !== null ? (
								<div className="mt-1.5 font-mono text-[8.5px] text-(--f4)">
									drive playbook gate · mode share ≥ {reading.momentumGate}
								</div>
							) : null}
						</div>
					</div>
				</div>
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
	value: string;
	accent?: string;
}) => (
	<div className="flex justify-between gap-3">
		<span className="text-(--f3)">{label}</span>
		<span style={{ color: accent ?? "var(--f1)" }}>{value}</span>
	</div>
);

const CortexSurface = () => {
	const readings = useSelector(
		measurementsStore,
		(state) => state.readings.cognitive ?? {},
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const { selectFocusSymbol } = terminalStore.actions;
	const scopeSymbols = Object.keys(readings).filter((scope) =>
		scope.includes("/"),
	);
	const reading = (readings[focusSymbol] ?? null) as Record<
		string,
		unknown
	> | null;
	const tokenCount =
		typeof reading?.sequence === "string"
			? reading.sequence.split(/[-_]+/).filter((token) => token !== "").length
			: 0;

	return (
		<div className="flex h-full min-w-[1140px] flex-col">
			<ContextStrip
				label="Sensory context"
				symbols={
					scopeSymbols.length > 0 ? scopeSymbols.slice(0, 10) : ["stream"]
				}
				activeSymbol={focusSymbol}
				onSelect={selectFocusSymbol}
				meta={`${tokenCount} tokens · ${String(reading?.regimePrefix ?? "waiting")} · ${focusSymbol.split("/")[0] ?? focusSymbol}`}
			/>
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_364px]">
				<div className="flex min-h-0 flex-col border-(--line) border-r">
					<div className="relative min-h-0 flex-[1.55] overflow-hidden bg-(--sunken)">
						<TerminalCognitiveChart />
						<div className="pointer-events-none absolute top-3 left-3.5">
							<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
								Sensory prefix tree · s/[sequence]
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								edge = P(next token | prefix) · amber = MAP beam path
							</div>
						</div>
						<div className="pointer-events-none absolute top-3 right-3.5 flex gap-3 font-mono text-[9px] text-(--f3)">
							<span className="inline-flex items-center gap-1.5">
								<span className="inline-block h-0.5 w-2.5 bg-(--acc)" />
								beam
							</span>
							<span className="inline-flex items-center gap-1.5">
								<span className="inline-block h-0.5 w-2.5 bg-(--line2)" />
								branch
							</span>
						</div>
					</div>
					<div className="flex min-h-0 flex-1 flex-col border-(--line) border-t bg-(--surface)">
						<div className="flex shrink-0 items-center justify-between border-(--line) border-b px-3 py-2">
							<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
								Beam search lookahead
							</span>
							<span className="font-mono text-[9.5px] text-(--f4)">
								width 4 · {tokenCount} hops · log-prob
							</span>
						</div>
						<CortexBeamList reading={reading} />
					</div>
				</div>
				<div className="min-h-0 overflow-auto bg-(--surface) p-3.5">
					<CortexSidePanels reading={reading} />
				</div>
			</div>
		</div>
	);
};

const AllocationSurface = () => (
	<div className="flex h-full min-w-[1080px] flex-col">
		<div className="flex shrink-0 items-center gap-[22px] border-(--line) border-b bg-(--surface) px-[18px] py-3">
			<div>
				<div className="font-serif font-semibold text-[18px] text-(--f1) leading-[1.1]">
					Edge-proportional sizing
				</div>
				<div className="mt-[3px] font-mono text-[10px] text-(--f4)">
					edge = thesis − median − mad · share = edge / (thesis + Σ positive) ·
					notional = free × share
				</div>
			</div>
			<div className="ml-auto flex items-center gap-5">
				<AllocMetric label="Deployable" value="—" />
				<AllocMetric label="Deployed" value="—" accent />
				<AllocMetric label="Positions" value="0" />
			</div>
		</div>
		<div className="grid min-h-0 flex-1 grid-cols-[minmax(560px,1fr)_320px]">
			<AllocationView />
			<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
				<AllocationSidePanel />
			</div>
		</div>
	</div>
);

const AllocMetric = ({
	label,
	value,
	accent = false,
}: {
	label: string;
	value: string;
	accent?: boolean;
}) => (
	<div className="flex flex-col items-end gap-px">
		<span className="text-[9px] text-(--f4) uppercase tracking-widest">
			{label}
		</span>
		<span
			className="font-mono text-[13px] font-semibold"
			style={{ color: accent ? "var(--acc)" : "var(--f1)" }}
		>
			{value}
		</span>
	</div>
);

export const SurfaceBody = ({ surface }: { surface: TerminalSurface }) => {
	if (surface === "signals") {
		return <SignalSurface />;
	}

	if (surface === "decisions") {
		return <DecisionTreeView />;
	}

	if (surface === "xray") {
		return <XraySurface />;
	}

	if (surface === "cortex") {
		return <CortexSurface />;
	}

	if (surface === "allocation") {
		return <AllocationSurface />;
	}

	return <DashboardSurface />;
};
