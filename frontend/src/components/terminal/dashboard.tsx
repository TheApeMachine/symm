import { useSelector } from "@tanstack/react-store";
import type { ReactNode } from "react";
import { appStore } from "#/collections/app";
import { decisionsStore } from "#/collections/decisions";
import { manifoldStore } from "#/collections/manifold";
import { measurementsStore } from "#/collections/measurements";
import { resonanceStore } from "#/collections/resonance";
import { terminalStore } from "#/collections/terminal";
import { tickStore } from "#/collections/tick";
import {
	TerminalFluidChart,
	TerminalPredictionChart,
} from "#/components/terminal/charts";
import {
	AuditRows,
	DecisionLineMeta,
	DecisionRows,
	KernelList,
	decisionRowsFromFrame,
	kernelHealthSummary,
	PositionLineMeta,
	PositionRows,
} from "#/components/terminal/rows";
import { KernelInspector } from "#/components/terminal/widgets";

const pulseDecisionText = (frame: Record<string, unknown> | null): string => {
	const decisions = decisionRowsFromFrame(frame);
	const admitted = decisions.filter((decision) => decision.verdict === "allow");
	const rejected = decisions.filter((decision) => decision.verdict !== "allow");

	if (rejected.length > 0) {
		const first = rejected[0] ?? {};
		const label = String(first.source ?? first.why ?? "candidate").replace(
			/_/g,
			" ",
		);

		return `reject ${label} ×${rejected.length}`;
	}

	return admitted.length > 0 ? `admit ×${admitted.length}` : "";
};

const DashboardPulse = () => {
	const online = useSelector(appStore, (state) => state.online);
	const tick = useSelector(tickStore, (state) => state.frame);
	const decisionFrame = useSelector(decisionsStore, (state) => state.frame);
	const origins = (tick?.origins ?? {}) as Record<string, unknown>;
	const decisionText = pulseDecisionText(decisionFrame);

	return (
		<div className="flex h-8 shrink-0 items-center gap-4 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)">
			<span className="font-semibold text-(--f1)">
				#{String(tick?.count ?? 0)}
			</span>
			<span>
				phase{" "}
				<span className="text-(--acc)">
					{online ? String(tick?.phase ?? "stream") : "offline"}
				</span>
			</span>
			<span>meas {String(tick?.measurements ?? "—")}</span>
			<span>cand {String(tick?.candidates ?? "—")}</span>
			<span>open {String(tick?.open ?? "—")}</span>
			<span>
				quotes{" "}
				{tick?.quotes_ready !== undefined && tick?.quotes_total !== undefined
					? `${String(tick.quotes_ready)}/${String(tick.quotes_total)}`
					: "—"}
			</span>
			<span>fluid {String(tick?.fluid ?? origins.fluid ?? "—")}</span>
			{decisionText !== "" ? (
				<span className="ml-auto text-(--down)">{decisionText}</span>
			) : null}
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

const ColumnHeader = ({ title, meta }: { title: string; meta?: ReactNode }) => (
	<div className="sticky top-0 z-2 flex items-center justify-between border-(--line) border-b bg-(--surface) px-3 py-2">
		<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
			{title}
		</span>
		{meta ? (
			<span className="font-mono text-[10px] text-(--f4)">{meta}</span>
		) : null}
	</div>
);

const dashboardRecord = (value: unknown): Record<string, unknown> | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: null;

const dashboardString = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

const resonanceSnapshotForSymbol = (
	frame: Record<string, unknown> | null,
	focusSymbol: string,
): Record<string, unknown> | null => {
	const symbol = focusSymbol.trim();

	if (symbol !== "" && symbol !== "stream") {
		const snapshots = Array.isArray(frame?.snapshots) ? frame.snapshots : [];

		for (const snapshot of snapshots) {
			const record = dashboardRecord(snapshot);

			if (dashboardString(record?.symbol) === symbol) {
				return record;
			}
		}

		const focus = dashboardRecord(frame?.focus);

		if (dashboardString(focus?.symbol) === symbol) {
			return focus;
		}

		return null;
	}

	return dashboardRecord(frame?.focus);
};

export const DashboardSurface = () => {
	const readings = useSelector(measurementsStore, (state) => state);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const fieldStyle = useSelector(terminalStore, (state) => state.fieldStyle);
	const manifoldFrame = useSelector(manifoldStore, (state) => state.frame);
	const resonanceFrame = useSelector(resonanceStore, (state) => state.frame);
	const grid = (manifoldFrame?.grid ?? {}) as Record<string, unknown>;
	const gridText =
		grid.x !== undefined && grid.z !== undefined
			? `grid ${String(grid.x)}×${String(grid.z)}`
			: "grid —";
	const peak =
		typeof manifoldFrame?.peak === "number"
			? manifoldFrame.peak.toFixed(2)
			: undefined;
	const carriers = Array.isArray(manifoldFrame?.carriers)
		? manifoldFrame.carriers
		: [];
	const predictionFrame = readings.resonance?.[focusSymbol] as
		| Record<string, unknown>
		| undefined;
	const resonanceFocus = resonanceSnapshotForSymbol(
		resonanceFrame,
		focusSymbol,
	);
	const predictionOutput = (predictionFrame?.output ?? {}) as Record<
		string,
		unknown
	>;
	const predictionScope =
		(focusSymbol !== "stream"
			? focusSymbol
			: dashboardString(resonanceFocus?.symbol)) ||
		dashboardString(predictionFrame?.scope) ||
		"stream";
	const predictionSurprise = (
		(resonanceFocus?.surprise as number) ??
		(predictionFrame?.surprise as number) ??
		(predictionOutput.surprise as number) ??
		0
	).toFixed(2);
	const predictionConfidence = `${Math.round(
		((resonanceFocus?.confidence as number) ??
			(predictionFrame?.confidence as number) ??
			(predictionOutput.confidence as number) ??
			0) * 100,
	)}%`;
	const kernelHealth = kernelHealthSummary(readings, focusSymbol);

	return (
		<div className="flex h-full min-w-[1120px] flex-col">
			<DashboardPulse />
			<div className="relative grid min-h-0 flex-1 grid-cols-[282px_minmax(360px,1fr)_332px]">
				<KernelInspector />

				<div className="min-h-0 overflow-auto border-(--line) border-r bg-(--surface)">
					<ColumnHeader title="Signal kernels" meta={kernelHealth.label} />
					<KernelList />
				</div>

				<div className="flex min-h-0 flex-col border-(--line) border-r bg-(--sunken)">
					<DashboardCanvasPanel
						title="Fluid density field"
						meta="navier–stokes · vol-rank × Δ · whale carriers"
						topRight={
							<div>
								<div>{gridText}</div>
								<div>outliers {carriers.length}</div>
								{peak !== undefined ? <div>peak {peak}</div> : null}
							</div>
						}
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
						<ColumnHeader title="Decisions" meta={<DecisionLineMeta />} />
						<DecisionRows />
					</div>
					<div className="flex min-h-0 flex-1 flex-col border-(--line) border-b">
						<ColumnHeader title="Open positions" meta={<PositionLineMeta />} />
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
