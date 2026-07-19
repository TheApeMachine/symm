import { useSelector } from "@tanstack/react-store";
import { type ReactNode, useRef, useState } from "react";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import type { Holding, TickFrame } from "#/collections/types";
import { fixed } from "#/components/terminal/decision-format";
import { LifecycleTrack } from "#/components/terminal/lifecycle-track";
import { ThesisEvidenceCanvas } from "#/components/terminal/thesis-evidence-canvas";
import {
	accumulateThesisSnapshot,
	type ThesisSnapshot,
	thesisSnapshotFor,
} from "#/components/terminal/thesis-snapshot";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { cn } from "#/lib/utils";
import { getWorker } from "#/providers/websocket";
import type { Category, Measurement } from "#/types/measurement";
import type {
	Finding,
	GraphFrame,
	StrategyDecision,
	ThesisForecast,
	ThesisHypothesis,
} from "#/types/thesis";
import { Badge } from "@/components/ui/badge";
import { Flex } from "@/components/ui/flex";
import { Panel } from "@/components/ui/panel";

/*
evidenceLabels turns measurement evidence keys (source/stream/metric/…) into
compact source/metric labels, de-duplicated, capped so the row stays readable.
*/
const evidenceLabels = (keys: string[] | undefined): string[] => {
	if (keys === undefined) {
		return [];
	}

	const labels: string[] = [];
	const seen = new Set<string>();

	for (const key of keys) {
		const parts = key.split("/");
		const source = parts[0] ?? key;
		const metric = parts[2] ?? source;
		const label = metric === source ? source : `${source}/${metric}`;

		if (!seen.has(label)) {
			seen.add(label);
			labels.push(label);
		}
	}

	return labels.slice(0, 4);
};

const Section = ({
	title,
	meta,
	children,
}: {
	title: string;
	meta?: string;
	children: ReactNode;
}) => (
	<Panel variant="surface" size="bare" className="px-3 py-2.5">
		<div className="mb-2 flex items-center justify-between gap-2">
			<span className="font-semibold text-[11px] text-(--f1) uppercase tracking-[0.08em]">
				{title}
			</span>
			{meta ? (
				<span className="font-mono text-[9px] text-(--f4)">{meta}</span>
			) : null}
		</div>
		{children}
	</Panel>
);

const ThesisDetailRail = ({ snapshot }: { snapshot: ThesisSnapshot }) => (
	<Flex.Column className="min-h-0 gap-2 overflow-auto pr-1">
		<Section title="Lifecycle" meta={snapshot.lifecycle ?? "observing"}>
			<LifecycleTrack
				symbol={snapshot.symbol}
				state={snapshot.lifecycle ?? "observing"}
			/>
		</Section>

		<Section title="Decision" meta={snapshot.decision?.action ?? "none"}>
			{snapshot.decision === null ? (
				<div className="font-mono text-[10px] text-(--f4)">
					no strategy decision retained for this symbol
				</div>
			) : (
				<div className="flex flex-col gap-1 font-mono text-[10px] text-(--f3)">
					<div>
						utility{" "}
						<span className="text-(--f1)">
							{fixed(snapshot.decision.utility)}
						</span>
					</div>
					<div>
						proposed{" "}
						<span className="text-(--acc)">
							{fixed(snapshot.decision.proposedNotional)}
						</span>
					</div>
					<div>
						return{" "}
						<span className="text-(--f1)">
							{fixed(snapshot.decision.expectedReturn)}
						</span>
						{" · conf "}
						<span className="text-(--f1)">
							{fixed(snapshot.decision.confidence)}
						</span>
					</div>
					<div className="text-(--f4)">{snapshot.decision.cause}</div>
				</div>
			)}
		</Section>

		<Section title="Forecasts" meta={`${snapshot.forecasts.length} rows`}>
			{snapshot.forecasts.length === 0 ? (
				<div className="font-mono text-[10px] text-(--f4)">none published</div>
			) : (
				snapshot.forecasts.slice(-4).map((forecast) => (
					<div
						key={`${forecast.source}:${forecast.at}:${forecast.target}`}
						className="border-(--line) border-t py-1.5 font-mono text-[10px] first:border-t-0 first:pt-0"
					>
						<div className="text-(--f1)">
							{forecast.source} · {forecast.target}
						</div>
						<div className="text-(--f3)">
							return {forecast.expectedReturn.toFixed(4)} · conf{" "}
							{forecast.confidence.toFixed(3)}
						</div>
					</div>
				))
			)}
		</Section>

		<Section title="Hypotheses" meta={`${snapshot.hypotheses.length} rows`}>
			{snapshot.hypotheses.length === 0 ? (
				<div className="font-mono text-[10px] text-(--f4)">none published</div>
			) : (
				snapshot.hypotheses.slice(-3).map((hypothesis) => (
					<div
						key={`${hypothesis.source}:${hypothesis.at}:${hypothesis.claim}`}
						className="border-(--line) border-t py-1.5 font-mono text-[10px] first:border-t-0 first:pt-0"
					>
						<div className="text-(--f1)">{hypothesis.claim}</div>
						<div className="text-(--f3)">
							do {hypothesis.doExpectation.toFixed(4)} · strength{" "}
							{hypothesis.strength.toFixed(3)}
						</div>
					</div>
				))
			)}
		</Section>

		<Section title="Categories" meta={`${snapshot.categories.length} rows`}>
			{snapshot.categories.length === 0 ? (
				<div className="font-mono text-[10px] text-(--f4)">none published</div>
			) : (
				<Flex.Column className="gap-1.5">
					{snapshot.categories.slice(-6).map((category) => (
						<div
							key={`${category.type}:${category.confidence}`}
							className="font-mono text-[10px]"
						>
							<div className="flex items-center justify-between gap-2">
								<span className="text-(--f1)">{category.type}</span>
								<span className="text-(--acc)">
									{category.confidence.toFixed(3)}
								</span>
							</div>
							{(category.supporting?.length ?? 0) > 0 && (
								<div className="text-(--f3)">
									+ {evidenceLabels(category.supporting).join(" · ")}
								</div>
							)}
							{(category.opposing?.length ?? 0) > 0 && (
								<div className="text-(--f4)">
									− {evidenceLabels(category.opposing).join(" · ")}
								</div>
							)}
						</div>
					))}
				</Flex.Column>
			)}
		</Section>

		<Section title="Holdings" meta={`${snapshot.holdings.length} lots`}>
			{snapshot.holdings.length === 0 ? (
				<div className="font-mono text-[10px] text-(--f4)">no holdings</div>
			) : (
				snapshot.holdings.map((holding) => (
					<div
						key={`${holding.symbol}:${String(holding.status)}`}
						className="border-(--line) border-t py-1.5 font-mono text-[10px] first:border-t-0 first:pt-0"
					>
						<div className="flex items-center justify-between gap-2">
							<span className="text-(--f1)">
								{typeof holding.status === "string"
									? holding.status
									: "unknown"}
							</span>
							<span className="text-(--f4)">qty {fixed(holding.qty)}</span>
						</div>
						<div className="text-(--f3)">
							mark {fixed(holding.mark)} · pnl {fixed(holding.pnl)} · return{" "}
							{fixed(holding.return_pct)}
						</div>
					</div>
				))
			)}
		</Section>

		<Section title="Findings" meta={`${snapshot.findings.length} rows`}>
			{snapshot.findings.length === 0 ? (
				<div className="font-mono text-[10px] text-(--f4)">
					none retained · postmortem after exit
				</div>
			) : (
				snapshot.findings.slice(-3).map((finding) => (
					<div
						key={`${finding.component}:${finding.condition}:${finding.estimatedEffect}`}
						className="border-(--line) border-t py-1.5 first:border-t-0 first:pt-0"
					>
						<Badge label={finding.component} variant="warning" size="xs" />
						<div className="mt-1 font-mono text-[10px] text-(--f1)">
							{finding.condition}
						</div>
					</div>
				))
			)}
		</Section>
	</Flex.Column>
);

const tickCount = (frame: TickFrame | undefined): number | null => {
	const tick = frame?.count;

	return typeof tick === "number" && Number.isFinite(tick) ? tick : null;
};

/*
ThesisModal opens a large inspect surface for one open position's full thesis
carrier, including the evidence graph published from types.Thesis.Graphs.
*/
export const ThesisModal = () => {
	const thesisSymbol = useSelector(
		terminalStore,
		(state) => state.thesisSymbol,
	);
	const online = useSelector(appStore, (state) => state.online);
	const [snapshot, setSnapshot] = useState<ThesisSnapshot | null>(null);
	const retainedRef = useRef<ThesisSnapshot | null>(null);

	useDirectStorePaint(
		getWorker(),
		thesisSymbol === null
			? []
			: [
					{ store: "measurements", key: thesisSymbol },
					{ store: "forecasts", key: thesisSymbol },
					{ store: "hypotheses", key: thesisSymbol },
					{ store: "categories", key: thesisSymbol },
					{ store: "decisions", key: thesisSymbol },
					{ store: "lifecycle", key: thesisSymbol },
					{ store: "holdings", key: "" },
					{ store: "findings", key: thesisSymbol },
					{ store: "graphs", key: thesisSymbol },
					{ store: "tick", key: "" },
				],
		(buffers) => {
			if (thesisSymbol === null) {
				retainedRef.current = null;
				setSnapshot(null);
				return;
			}

			const decisions = (buffers[`decisions:${thesisSymbol}`] ??
				buffers["decisions:"] ??
				[]) as StrategyDecision[];
			const lifecycle = (buffers[`lifecycle:${thesisSymbol}`] ??
				buffers["lifecycle:"] ??
				[]) as Array<{ symbol: string; state: string }>;
			const graphs = (buffers[`graphs:${thesisSymbol}`] ??
				buffers["graphs:"] ??
				[]) as GraphFrame[];
			const live = thesisSnapshotFor({
				symbol: thesisSymbol,
				tick: tickCount((buffers["tick:"] as TickFrame[] | undefined)?.at(-1)),
				lifecycle:
					lifecycle.find((row) => row.symbol === thesisSymbol)?.state ?? null,
				graph: graphs.find((frame) => frame.symbol === thesisSymbol) ?? null,
				measurements: (buffers[`measurements:${thesisSymbol}`] ??
					buffers["measurements:"] ??
					[]) as Measurement[],
				decision:
					decisions.find((decision) => decision.symbol === thesisSymbol) ??
					null,
				forecasts: (buffers[`forecasts:${thesisSymbol}`] ??
					buffers["forecasts:"] ??
					[]) as ThesisForecast[],
				hypotheses: (buffers[`hypotheses:${thesisSymbol}`] ??
					buffers["hypotheses:"] ??
					[]) as ThesisHypothesis[],
				categories: (buffers[`categories:${thesisSymbol}`] ??
					buffers["categories:"] ??
					[]) as Array<Category & { symbol?: string }>,
				holdings: (buffers["holdings:"] ?? []) as Holding[],
				findings: (buffers[`findings:${thesisSymbol}`] ??
					buffers["findings:"] ??
					[]) as Finding[],
			});
			const next = accumulateThesisSnapshot(retainedRef.current, live);
			retainedRef.current = next;
			setSnapshot(next);
		},
		[thesisSymbol, online],
	);

	if (thesisSymbol === null || snapshot === null) {
		return null;
	}

	const { closeThesis } = terminalStore.actions;

	return (
		<div className="absolute inset-0 z-20 flex animate-[symFade_0.18s_ease] items-center justify-center bg-[color-mix(in_srgb,var(--sunken)_52%,transparent)] p-6 backdrop-blur-sm">
			<button
				type="button"
				aria-label="Close thesis modal"
				className="absolute inset-0"
				onClick={closeThesis}
			/>
			<div
				className={cn(
					"pointer-events-auto relative z-10 flex h-[min(88vh,920px)] w-[min(1180px,96vw)] flex-col overflow-hidden",
					"rounded-[8px] border border-(--line2) bg-(--surface) shadow-[0_28px_72px_-18px_rgba(0,0,0,0.78)]",
				)}
			>
				<div className="flex shrink-0 items-start justify-between gap-3 border-(--line) border-b px-5 py-4">
					<div className="min-w-0">
						<div className="flex flex-wrap items-center gap-2">
							<span className="font-serif font-semibold text-[24px] text-(--f1) leading-[1.05]">
								{snapshot.symbol}
							</span>
							<Badge
								label={snapshot.lifecycle ?? "observing"}
								variant="info"
								size="xs"
							/>
						</div>
						<div className="mt-1 font-mono text-[10px] text-(--f4)">
							thesis carrier · tick {snapshot.tick ?? "—"} ·{" "}
							{snapshot.measurements.length} measurements ·{" "}
							{snapshot.graph?.nodes.length ?? 0} graph nodes
						</div>
					</div>
					<button
						type="button"
						onClick={closeThesis}
						className="flex size-[28px] shrink-0 cursor-pointer items-center justify-center rounded-[3px] border border-(--line) bg-(--raised) p-0 text-(--f3) hover:border-(--line2) hover:text-(--f1)"
					>
						<svg
							width="13"
							height="13"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							strokeWidth="2"
							aria-hidden="true"
						>
							<path d="M6 6l12 12M18 6L6 18" />
						</svg>
					</button>
				</div>

				<div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1.55fr)_minmax(280px,360px)]">
					<div className="relative min-h-0 border-(--line) border-r">
						<ThesisEvidenceCanvas symbol={snapshot.symbol} />
						<div className="pointer-events-none absolute top-3.5 left-4">
							<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
								Evidence graph
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								measurement nodes · typed Gonum relationships
							</div>
						</div>
					</div>
					<div className="min-h-0 overflow-y-auto p-3.5">
						<ThesisDetailRail snapshot={snapshot} />
					</div>
				</div>
			</div>
		</div>
	);
};
