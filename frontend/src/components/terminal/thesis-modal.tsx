import { useEffect, useState } from "react";
import { useSelector } from "@tanstack/react-store";
import { terminalStore } from "#/collections/terminal";
import { ModelScope } from "#/components/graph/component";
import {
	paintGraphSurface,
	readGraphSurface,
	subscribeGraphSurface,
} from "#/components/terminal/graph-surface-store";
import { MCTSTreeVisualizer } from "#/components/terminal/mcts-tree-visualizer";
import { ThesisDetailRail } from "#/components/terminal/thesis-detail-rail";
import { Typography } from "@/components/ui/typography";
import { graphStore, strategyStore, tickStore, useSubscribe } from "#/providers/ws-stores";
import { cn } from "#/lib/utils";

export type ModalViewMode = "mcts" | "graph3d";

/*
ThesisModal is the carrier for one symbol's thesis and its full decision breakdown.

It provides a comprehensive inspection window featuring:
- Full visual breakdown of the MCTS/Pearl causal branching process
- The 3D WebGL WebGPU Causal Graph renderer
- Live arbitration & entry decision snapshot rail
*/
const ThesisActionBadge = ({ symbol }: { symbol: string }) => {
	const root = useSubscribe(strategyStore, (state) => {
		const decision = (state?.decisions ?? []).find((entry) => entry.symbol === symbol);
		const el = root.current?.querySelector<HTMLElement>("[data-action]");

		if (el instanceof HTMLElement) {
			el.textContent = decision?.action ?? "";
		}
	}, [symbol]);

	return (
		<span ref={root} className="contents">
			<span data-action className="rounded-full border border-(--line2) px-2.5 py-0.5 font-mono text-[9.5px] uppercase font-semibold" />
		</span>
	);
};

const ThesisTickCounter = () => {
	const root = useSubscribe(tickStore, (state) => {
		const el = root.current?.querySelector<HTMLElement>("[data-tick]");

		if (el instanceof HTMLElement) {
			el.textContent = String(state?.count ?? "—");
		}
	});

	return (
		<div ref={root} className="font-mono text-[10px] text-(--f4)">
			tick <span data-tick />
		</div>
	);
};

export const openThesisShell = (symbol: string) => {
	terminalStore.actions.openThesis(symbol);
};

export const closeThesisShell = () => {
	terminalStore.actions.closeThesis();
};

export const ThesisModal = () => {
	const symbol = useSelector(terminalStore, (state) => state.thesisSymbol);
	const [viewMode, setViewMode] = useState<ModalViewMode>("mcts");
	const [graphState, setGraphState] = useState(readGraphSurface);
	const [selectedNodeName, setSelectedNodeName] = useState<string | null>(null);

	useEffect(() => {
		if (graphStore.state) {
			paintGraphSurface(graphStore.state);
		}

		const { unsubscribe: unregisterStore } = graphStore.subscribe((state) => {
			if (state) {
				paintGraphSurface(state);
			}
		});

		const unregisterSurface = subscribeGraphSurface(() => {
			setGraphState(readGraphSurface());
		});

		return () => {
			unregisterStore();
			unregisterSurface();
		};
	}, []);

	if (symbol === null || symbol === "") {
		return null;
	}

	return (
		<div className="absolute inset-0 z-20 flex animate-[symFade_0.18s_ease] items-center justify-center bg-[color-mix(in_srgb,var(--sunken)_52%,transparent)] p-6 backdrop-blur-sm">
			<button
				type="button"
				aria-label="Close thesis modal"
				className="absolute inset-0 cursor-default"
				onClick={closeThesisShell}
			/>
			<div
				className={cn(
					"pointer-events-auto relative z-10 flex h-[min(90vh,960px)] w-[min(1240px,96vw)] flex-col overflow-hidden",
					"rounded-lg border border-(--line2) bg-(--surface) shadow-[0_28px_72px_-18px_rgba(0,0,0,0.78)]",
				)}
			>
				{/* Modal Top Header */}
				<div className="flex shrink-0 items-center justify-between gap-3 border-(--line) border-b px-5 py-3.5">
					<div className="flex flex-wrap items-center gap-3">
						<Typography.Display size="lg">{symbol}</Typography.Display>
						<ThesisActionBadge symbol={symbol} />

						{/* View Switcher Tabs */}
						<div className="ml-4 flex items-center rounded border border-(--line) bg-(--sunken) p-0.5 font-mono text-[10px]">
							<button
								type="button"
								onClick={() => setViewMode("mcts")}
								className={`cursor-pointer rounded px-2.5 py-1 uppercase transition-all ${
									viewMode === "mcts"
										? "bg-(--raised) text-(--acc) font-semibold shadow-xs"
										: "text-(--f4) hover:text-(--f2)"
								}`}
							>
								MCTS / Pearl Process
							</button>
							<button
								type="button"
								onClick={() => setViewMode("graph3d")}
								className={`cursor-pointer rounded px-2.5 py-1 uppercase transition-all ${
									viewMode === "graph3d"
										? "bg-(--raised) text-(--acc) font-semibold shadow-xs"
										: "text-(--f4) hover:text-(--f2)"
								}`}
							>
								3D Causal Graph
							</button>
						</div>
					</div>

					<div className="flex items-center gap-3">
						<ThesisTickCounter />
						<button
							type="button"
							onClick={closeThesisShell}
							className="flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-[3px] border border-(--line) bg-(--raised) p-0 text-(--f3) hover:border-(--line2) hover:text-(--f1)"
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
								<title>Close</title>
								<path d="M6 6l12 12M18 6L6 18" />
							</svg>
						</button>
					</div>
				</div>

				{/* Main Body Split */}
				<div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1.65fr)_minmax(280px,360px)]">
					<div className="relative min-h-0 overflow-hidden border-(--line) border-r">
						{viewMode === "mcts" && (
							<MCTSTreeVisualizer symbol={symbol} />
						)}

						{viewMode === "graph3d" && (
							<div className="relative h-full w-full bg-(--sunken)">
								{graphState.graph ? (
									<ModelScope
										className="h-full"
										graph={graphState.graph}
										onNodeSelect={(_index: number, name: string) => setSelectedNodeName(name)}
										selectedNodeName={selectedNodeName}
									/>
								) : (
									<div className="flex h-full items-center justify-center font-mono text-[11px] text-(--f4)">
										Waiting for market graph frame...
									</div>
								)}
								<div className="pointer-events-none absolute top-3 left-4 rounded bg-[color-mix(in_srgb,var(--surface)_80%,transparent)] px-2 py-1 backdrop-blur-xs">
									<div className="font-semibold text-[9.5px] text-(--f2) uppercase tracking-[0.13em]">
										3D Causal Graph Renderer
									</div>
									<div className="font-mono text-[9px] text-(--f4)">
										WebGL · force simulation · interactive camera
									</div>
								</div>
							</div>
						)}
					</div>

					<div className="min-h-0 overflow-y-auto p-3.5">
						<ThesisDetailRail symbol={symbol} />
					</div>
				</div>
			</div>
		</div>
	);
};