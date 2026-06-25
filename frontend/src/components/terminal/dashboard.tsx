import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { appStore } from "#/collections/app";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	TerminalFluidChart,
	TerminalPredictionChart,
} from "#/components/terminal/charts";
import {
	AuditRows,
	DecisionRows,
	KernelList,
	PositionRows,
} from "#/components/terminal/rows";
import { KernelInspector } from "#/components/terminal/widgets";

const DashboardPulse = () => {
	const storyTicks = useSelector(appStore, (state) => state.storyTicks);
	const online = useSelector(appStore, (state) => state.online);
	const enginePhase = useSelector(appStore, (state) => state.enginePhase);
	const playbookEvaluations = useSelector(
		appStore,
		(state) => state.playbookEvaluations,
	);
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const fluidScopes = readings.fluid ?? {};
	const fluidSampled = Object.keys(fluidScopes).length;
	const symbolsTotal = Math.max(
		[
			...new Set(
				Object.values(readings).flatMap((scopes) =>
					Object.keys(scopes).filter((scope) => scope.includes("/")),
				),
			),
		].length,
		1,
	);
	const focusFluid = fluidScopes[focusSymbol] as
		| Record<string, unknown>
		| undefined;
	const quotesReady = (focusFluid?.quotes_ready as number) ?? fluidSampled;

	return (
		<div className="flex h-8 shrink-0 items-center gap-4 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)">
			<span className="font-semibold text-(--f1)">#{storyTicks}</span>
			<span>
				phase{" "}
				<span className="text-(--acc)">
					{online ? enginePhase || "stream" : "offline"}
				</span>
			</span>
			<span>meas {playbookEvaluations.toLocaleString()}</span>
			<span>cand 0</span>
			<span>open 0</span>
			<span>
				quotes {quotesReady}/{symbolsTotal}
			</span>
			<span>
				fluid {fluidSampled}/{symbolsTotal}
			</span>
		</div>
	);
};

const FluidLegend = () => (
	<div className="pointer-events-none absolute bottom-2.5 left-3 flex gap-3.5 font-mono text-[9px] text-(--f3)">
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-(--acc) shadow-[0_0_7px_var(--acc)]" />
			whale carrier
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-info" />
			laminar
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-(--down)" />
			turbulent
		</span>
	</div>
);

const DashboardCanvasPanel = ({
	title,
	meta,
	topRight,
	legend,
	footer,
	children,
	className,
}: {
	title: string;
	meta: string;
	topRight?: ReactNode;
	legend?: ReactNode;
	footer?: ReactNode;
	children: ReactNode;
	className: string;
}) => (
	<div className={`relative min-h-0 overflow-hidden bg-[#0a0907] ${className}`}>
		<div className="absolute inset-0">{children}</div>
		<div className="pointer-events-none absolute inset-0 opacity-50 bg-[repeating-linear-gradient(0deg,rgba(0,0,0,0.18)_0px,rgba(0,0,0,0.18)_1px,transparent_1px,transparent_3px)] mix-blend-multiply" />
		<div className="pointer-events-none absolute top-[11px] left-3">
			<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
				{title}
			</div>
			<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">{meta}</div>
		</div>
		{topRight ? (
			<div className="pointer-events-none absolute top-[11px] right-3 text-right font-mono text-[9.5px] text-(--f3) leading-[1.6]">
				{topRight}
			</div>
		) : null}
		{legend}
		{footer ? (
			<div className="pointer-events-none absolute right-3 bottom-2 font-mono text-[9.5px] text-(--f3)">
				{footer}
			</div>
		) : null}
	</div>
);

const ColumnHeader = ({ title, meta }: { title: string; meta?: string }) => (
	<div className="sticky top-0 z-2 flex items-center justify-between border-(--line) border-b bg-(--surface) px-3 py-2">
		<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
			{title}
		</span>
		{meta ? (
			<span className="font-mono text-[10px] text-(--f4)">{meta}</span>
		) : null}
	</div>
);

export const DashboardSurface = () => {
	const readings = useSelector(measurementsStore, (state) => state.readings);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const fieldStyle = useSelector(terminalStore, (state) => state.fieldStyle);
	const predictionFrame = readings.prediction?.[focusSymbol] as
		| Record<string, unknown>
		| undefined;
	const predictionOutput = (predictionFrame?.output ?? {}) as Record<
		string,
		unknown
	>;
	const predictionScope = (predictionFrame?.scope as string) ?? "stream";
	const predictionSurprise = (
		(predictionOutput.surprise as number) ?? 0
	).toFixed(2);
	const predictionConfidence = `${Math.round(((predictionOutput.confidence as number) ?? 0) * 100)}%`;
	const originCount = Object.keys(readings).length;

	return (
		<div className="flex h-full min-w-[1120px] flex-col">
			<DashboardPulse />
			<div className="relative grid min-h-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]">
				<KernelInspector />

				<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
					<ColumnHeader
						title="Signal kernels"
						meta={`${originCount} origins`}
					/>
					<KernelList />
				</div>

				<div className="flex min-h-0 flex-col border-(--line) border-r bg-(--sunken)">
					<DashboardCanvasPanel
						title="Fluid density field"
						meta="navier–stokes · vol-rank × Δ · whale carriers"
						topRight={<div>grid 64×38</div>}
						legend={<FluidLegend />}
						className="flex-[1.45]"
					>
						<TerminalFluidChart contour={fieldStyle === "Contour"} />
					</DashboardCanvasPanel>
					<DashboardCanvasPanel
						title={`Predictive coding · ${predictionScope}`}
						meta="hierarchical error · 8-step horizon"
						footer={`err ${predictionSurprise} · conf ${predictionConfidence}`}
						topRight={
							<div className="flex gap-3 text-left">
								<span className="inline-flex items-center gap-1.5">
									<span className="inline-block h-px w-3 bg-(--f1)" />
									actual
								</span>
								<span className="inline-flex items-center gap-1.5">
									<span className="inline-block h-px w-3 bg-info" />
									prediction
								</span>
								<span className="inline-flex items-center gap-1.5">
									<span className="size-2 bg-[color-mix(in_srgb,var(--acc)_30%,transparent)]" />
									error
								</span>
							</div>
						}
						className="flex-1 border-(--line) border-t"
					>
						<TerminalPredictionChart />
					</DashboardCanvasPanel>
				</div>

				<div className="flex min-h-0 flex-col bg-(--surface)">
					<div className="flex min-h-0 flex-[1.15] flex-col border-(--line) border-b">
						<ColumnHeader title="Decisions" meta="line —" />
						<DecisionRows />
					</div>
					<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
						<ColumnHeader title="Open positions" meta="—" />
						<PositionRows />
					</div>
					<div className="flex min-h-0 flex-1 flex-col">
						<ColumnHeader title="Audit trail" />
						<AuditRows />
					</div>
				</div>
			</div>
		</div>
	);
};
