import { useSelector } from "@tanstack/react-store";
import { type ReactNode, useCallback, useEffect, useRef } from "react";
import { categoriesStore } from "#/collections/categories";
import { decisionStore } from "#/collections/decisions";
import { findingsStore } from "#/collections/findings";
import { forecastsStore } from "#/collections/forecasts";
import { graphsStore, latestGraphFrame } from "#/collections/graphs";
import { hypothesesStore } from "#/collections/hypotheses";
import { lifecycleStore } from "#/collections/lifecycle";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import { tickStore } from "#/collections/tick";
import { tradeJournalStore } from "#/collections/trade-journal";
import { resizeCanvas } from "#/components/terminal/canvas";
import { fixed } from "#/components/terminal/decision-format";
import {
	drawEvidenceGraph,
	graphTopologyKey,
} from "#/components/terminal/evidence-graph-viz";
import { LifecycleTrack } from "#/components/terminal/lifecycle-track";
import {
	accumulateThesisSnapshot,
	type ThesisSnapshot,
	thesisSnapshotFor,
} from "#/components/terminal/thesis-snapshot";
import { cn } from "#/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Flex } from "@/components/ui/flex";
import { Panel } from "@/components/ui/panel";

const formatTime = (value: string): string =>
	value.length >= 19 ? value.slice(11, 19) : value;

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

const ThesisEvidenceCanvas = ({ symbol }: { symbol: string }) => {
	const graph = useSelector(
		graphsStore,
		(state) => latestGraphFrame(state.graphs, symbol),
		{
			compare: (left, right) =>
				graphTopologyKey(left) === graphTopologyKey(right),
		},
	);
	const canvasRef = useRef<HTMLCanvasElement | null>(null);

	const draw = useCallback(
		(context: CanvasRenderingContext2D, width: number, height: number) => {
			drawEvidenceGraph(context, width, height, graph);
		},
		[graph],
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

			draw(context, canvas.clientWidth, canvas.clientHeight);
		};

		render();
		const observer = new ResizeObserver(render);
		observer.observe(canvas);

		return () => observer.disconnect();
	}, [draw]);

	return (
		<canvas
			ref={canvasRef}
			className="absolute inset-0 block size-full bg-(--sunken)"
		/>
	);
};

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
						notional{" "}
						<span className="text-(--acc)">
							{fixed(snapshot.decision.proposedNotional)}
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
							className="flex items-center justify-between gap-2 font-mono text-[10px]"
						>
							<span className="text-(--f1)">{category.type}</span>
							<span className="text-(--acc)">
								{category.confidence.toFixed(3)}
							</span>
						</div>
					))}
				</Flex.Column>
			)}
		</Section>

		<Section
			title="Trade journal"
			meta={`${snapshot.observations.length} events`}
		>
			{snapshot.observations.length === 0 ? (
				<div className="font-mono text-[10px] text-(--f4)">no observations</div>
			) : (
				snapshot.observations.slice(-6).map((observation) => (
					<div
						key={`${observation.symbol}:${observation.kind}:${observation.at}:${observation.orderId ?? ""}:${observation.executionId ?? ""}:${observation.decision}`}
						className="border-(--line) border-t py-1.5 font-mono text-[10px] first:border-t-0 first:pt-0"
					>
						<div className="flex items-center justify-between gap-2">
							<span className="text-(--f1)">{observation.kind}</span>
							<span className="text-(--f4)">{formatTime(observation.at)}</span>
						</div>
						<div className="text-(--f3)">
							{[
								observation.action,
								observation.status,
								observation.side,
								observation.quantity && observation.price
									? `${observation.quantity} @ ${observation.price}`
									: "",
							]
								.filter((part) => typeof part === "string" && part.length > 0)
								.join(" · ")}
						</div>
					</div>
				))
			)}
		</Section>

		<Section title="Findings" meta={`${snapshot.findings.length} rows`}>
			{snapshot.findings.length === 0 ? (
				<div className="font-mono text-[10px] text-(--f4)">none retained</div>
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

const useThesisRefresh = (symbol: string | null): void => {
	useSelector(measurementsStore, (state) =>
		symbol === null ? 0 : state.measurements[symbol],
	);
	useSelector(forecastsStore, (state) => state.version);
	useSelector(hypothesesStore, (state) => state.version);
	useSelector(categoriesStore, (state) => state.version);
	useSelector(decisionStore, (state) => state.decisions[symbol ?? ""]);
	useSelector(lifecycleStore, (state) => state.lifecycle[symbol ?? ""]);
	useSelector(tradeJournalStore, (state) => state.version);
	useSelector(findingsStore, (state) => state.findings.length());
	useSelector(tickStore, (state) => state.frame);
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
	useThesisRefresh(thesisSymbol);
	const retainedRef = useRef<ThesisSnapshot | null>(null);

	if (thesisSymbol === null) {
		retainedRef.current = null;

		return null;
	}

	const liveSnapshot = thesisSnapshotFor(thesisSymbol);
	const snapshot = accumulateThesisSnapshot(retainedRef.current, liveSnapshot);
	retainedRef.current = snapshot;
	const { closeThesis } = terminalStore.actions;

	return (
		<div className="absolute inset-0 z-20 flex animate-[symFade_0.18s_ease] items-center justify-center bg-[color-mix(in_srgb,var(--sunken)_52%,transparent)] p-6 backdrop-blur-[4px]">
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
						<ThesisEvidenceCanvas symbol={thesisSymbol} />
						<div className="pointer-events-none absolute top-3.5 left-4">
							<div className="font-semibold text-[10px] text-(--f2) uppercase tracking-[0.13em]">
								Evidence graph
							</div>
							<div className="mt-0.5 font-mono text-[9.5px] text-(--f4)">
								measurement nodes · typed Gonum relationships
							</div>
						</div>
					</div>
					<div className="min-h-0 overflow-hidden p-3.5">
						<ThesisDetailRail snapshot={snapshot} />
					</div>
				</div>
			</div>
		</div>
	);
};
