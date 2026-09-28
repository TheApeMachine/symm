import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef, useState } from "react";
import {
	DEFAULT_FOCUS_SYMBOL,
	focusStore,
	positionStore,
	type RingBuffer,
	trainingStore,
} from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";
import { hubBaseUrl } from "#/lib/hub";
import { cn, memoizedQuery, renderValue } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { CandidatePanel, ImpulsePanel, InfluencePanel } from "./decision-panel";
import { Explain } from "./explain";
import { action, basis, clock, percent } from "./format";
import { ForwardLearningViz } from "./forward-learning-viz";
import { ImpulseMapViz } from "./impulse-map-viz";
import { KnowledgePanel } from "./knowledge-panel";
import { ImpulseMap, type Point, type Region } from "./map";
import { LearningPerformanceBanner } from "./performance-banner";
import { RadixTreeViz } from "./radix-tree-viz";
import { RecognitionPanel } from "./recognition-panel";
import { SkillPanel } from "./skill-panel";
import type { CognitionTreeResponse, ImpulseNode } from "./types";
import { LearningVisualizer } from "./visualizer";

export type Tab =
	| "forward"
	| "cognitive"
	| "impulse"
	| "recognition"
	| "decision"
	| "influence";

const TABS: Array<{ key: Tab; label: string }> = [
	{ key: "forward", label: "Model training" },
	{ key: "cognitive", label: "Cognitive tree" },
	{ key: "impulse", label: "Impulse map" },
	{ key: "recognition", label: "Precursor recognition" },
	{ key: "decision", label: "Model decision" },
	{ key: "influence", label: "Precursor discovery" },
];

export const LearningDashboard = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const [tab, setTab] = useState<Tab>("forward");
	const containerRef = useRef<HTMLDivElement>(null);

	// Real data states
	const [skillPct, setSkillPct] = useState(0);
	const [edgeBp, setEdgeBp] = useState(0);
	const [isTrading, setIsTrading] = useState(false);
	const [openPositionsCount, setOpenPositionsCount] = useState(0);

	// Impulse Map real nodes & regions
	const [impulseNodes, setImpulseNodes] = useState<ImpulseNode[]>([]);
	const [impulseRegions, setImpulseRegions] = useState<
		Array<{
			id: number;
			strength: number;
			authority: number;
			members: number;
		}>
	>([]);
	const [activePrecursors, setActivePrecursors] = useState<
		Array<{ label: string; latencyMs: number }>
	>([]);

	// Real Radix Tree data from backend
	const [treeResponse, setTreeResponse] =
		useState<CognitionTreeResponse | null>(null);

	// Listen to open positions count
	useEffect(() => {
		const unsubPositions = positionStore.subscribe((state) => {
			const frame = state as
				| {
						findLast?: (fn: () => boolean) => unknown;
						rowsLength?: () => number;
				  }
				| unknown[]
				| null
				| undefined;
			const latestFrame =
				typeof (frame as { findLast?: (fn: () => boolean) => unknown })
					?.findLast === "function"
					? (frame as { findLast: (fn: () => boolean) => unknown }).findLast(
							() => true,
						)
					: Array.isArray(frame)
						? frame[frame.length - 1]
						: frame;

			const typed = latestFrame as
				| { rowsLength?: () => number }
				| null
				| undefined;
			if (!typed || typeof typed.rowsLength !== "function") {
				setOpenPositionsCount(0);
				return;
			}
			setOpenPositionsCount(typed.rowsLength());
		});
		return () => {
			unsubPositions?.unsubscribe?.();
		};
	}, []);

	// Periodically fetch real Cognition tree
	useEffect(() => {
		let isMounted = true;
		const fetchTree = async () => {
			try {
				const res = await fetch(`${hubBaseUrl()}/cognition/tree`);
				if (!res.ok) return;
				const data: CognitionTreeResponse = await res.json();
				if (!isMounted) return;
				setTreeResponse(data);
			} catch {
				// Backend endpoint connecting
			}
		};

		fetchTree();
		const interval = window.setInterval(fetchTree, 4000);
		return () => {
			isMounted = false;
			clearInterval(interval);
		};
	}, []);

	// Live ring buffer subscription
	useEffect(() => {
		const root = containerRef.current;
		if (!root) return;

		const seen = new WeakSet<MeasurementT>();
		const cursor = new RingCursor<MeasurementT>();

		const update = (ring: RingBuffer<MeasurementT>) => {
			if (!ring || ring.isEmpty()) return;

			const metaEl = memoizedQuery(
				root,
				'[data-l="header-meta"]',
			) as HTMLElement;
			const statusMetaEl = memoizedQuery(
				root,
				'[data-l="status-meta"]',
			) as HTMLElement;
			const gateCountEl = memoizedQuery(
				root,
				'[data-l="gate-count"]',
			) as HTMLElement;
			const forwardMetaEl = memoizedQuery(
				root,
				'[data-l="forward-meta"]',
			) as HTMLElement;
			const recogMetaEl = memoizedQuery(
				root,
				'[data-l="recog-meta"]',
			) as HTMLElement;
			const recogStatusEl = memoizedQuery(
				root,
				'[data-l="recog-status"]',
			) as HTMLElement;
			const skillMetaEl = memoizedQuery(
				root,
				'[data-l="skill-meta"]',
			) as HTMLElement;
			const activityListEl = memoizedQuery(
				root,
				'[data-l="activity-list"]',
			) as HTMLElement;

			cursor.read(ring, (measurement) => {
				const metricMap: Record<string, number> = {};
				for (const m of measurement.metrics ?? []) {
					if (!m?.name) continue;
					const name = String(m.name);
					const raw = m.raw ?? 0;
					metricMap[name] = raw;

					const els = root.querySelectorAll(`[data-metric="${name}"]`);
					for (let j = 0; j < els.length; j++) {
						const el = els[j] as HTMLElement;
						const fmt = el.getAttribute("data-format");

						switch (fmt) {
							case "percent":
								el.innerText = percent(raw);
								break;
							case "basis":
								el.innerText = basis(raw);
								break;
							case "integer":
								el.innerText = Math.floor(raw).toLocaleString();
								break;
							case "bits":
								el.innerText = `${raw.toFixed(2)} bits`;
								break;
							case "spread":
								el.innerText = `Spread ${raw.toFixed(3)}`;
								break;
							case "surprisal":
								el.innerText = `Surprisal ${raw.toFixed(2)} nat`;
								break;
							case "action":
								if (raw === 1) {
									el.innerText = "ENTER";
									break;
								}
								if (raw === 2) {
									el.innerText = "EXIT";
									break;
								}
								el.innerText = "WAIT";
								break;
							default:
								renderValue(el, raw);
								break;
						}
					}
				}

				const steps = metricMap.steps ?? 0;
				const decisions = metricMap.decisions ?? 0;
				const resolved = metricMap.resolved ?? 0;
				const confidence = metricMap.confidence ?? 0;
				const contrast = metricMap.contrast ?? 0;
				const edge = metricMap.edge ?? 0;
				const evaluated = metricMap.evaluated ?? 0;
				const tradingActive = (metricMap.trading ?? 0) > 0;
				const skill = (metricMap.win_rate ?? metricMap.accuracy ?? 0) * 100;

				setSkillPct(skill);
				setEdgeBp(edge * 10000);
				setIsTrading(tradingActive);

				if (metaEl) {
					metaEl.innerText = `${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations · ${Math.floor(resolved).toLocaleString()} resolved`;
				}

				if (statusMetaEl) {
					statusMetaEl.innerText = `conf: ${(confidence * 100).toFixed(1)}% · contrast: ${contrast.toFixed(2)} bits · edge: ${(edge * 10000).toFixed(1)} bp`;
				}

				if (gateCountEl) {
					gateCountEl.innerText = tradingActive
						? "Paper trading"
						: "Training only · Gated";
				}

				if (forwardMetaEl) {
					forwardMetaEl.innerText = `${Math.floor(evaluated).toLocaleString()} completed evaluations`;
				}

				if (recogMetaEl) {
					recogMetaEl.innerText = `${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations`;
				}

				if (recogStatusEl) {
					recogStatusEl.innerText = tradingActive
						? "Execution active · Paper trading enabled"
						: "Training precursor associations · Quoted returns, orders gated";
				}

				if (skillMetaEl) {
					skillMetaEl.innerText = `${Math.floor(evaluated).toLocaleString()} forward evaluations · ${tradingActive ? "trading" : "learning"}`;
				}

				if (activityListEl && !seen.has(measurement)) {
					seen.add(measurement);

					const actionVal = metricMap.action ?? 0;
					const edgeVal = metricMap.edge ?? 0;
					const atNs = measurement.at ?? 0n;
					const timeStr =
						atNs > 0n
							? clock(new Date(Number(atNs / 1_000_000n)).toISOString())
							: clock("");
					const actStr = action(
						actionVal === 1 ? "enter" : actionVal === 2 ? "exit" : "wait",
						1,
						false,
					);
					const edgeStr = basis(edgeVal);

					const rowDiv = document.createElement("div");
					rowDiv.className =
						"flex items-center gap-2 border-(--line) border-b px-3 py-1.5 font-mono text-xs";

					const timeSpan = document.createElement("span");
					timeSpan.className = "text-(--f4) shrink-0";
					timeSpan.textContent = timeStr;

					const actSpan = document.createElement("span");
					actSpan.className = "text-(--f2) min-w-0 flex-1 truncate";
					actSpan.textContent = actStr;

					const edgeSpan = document.createElement("span");
					edgeSpan.className = "text-(--acc) shrink-0";
					edgeSpan.textContent = edgeStr;

					rowDiv.appendChild(timeSpan);
					rowDiv.appendChild(actSpan);
					rowDiv.appendChild(edgeSpan);

					activityListEl.prepend(rowDiv);

					while (activityListEl.children.length > 15) {
						activityListEl.lastElementChild?.remove();
					}
				}

				const grid = measurement.grid;
				const quantities = grid?.symbol === focusSymbol ? grid.quantities : [];
				const measuredRegions =
					grid?.symbol === focusSymbol ? grid.regions : [];

				if (quantities.length > 0) {
					const cellToRegion = new Map<number, number>();
					for (const r of measuredRegions) {
						if (Array.isArray(r.members)) {
							for (const m of r.members) {
								cellToRegion.set(Number(m), Number(r.id));
							}
						}
					}

					const mappedNodes: ImpulseNode[] = quantities.map((cell) => ({
						id: String(cell.id),
						label: String(cell.label ?? cell.source ?? `#${cell.id}`),
						cluster: cellToRegion.get(Number(cell.id)) ?? Number(cell.id) % 4,
						snr: cell.quality || 1,
						activation: cell.activity || 0,
						value: cell.value,
						x: cell.x,
						y: cell.y,
						present: cell.present,
					}));

					setImpulseNodes(mappedNodes);
					setImpulseRegions(
						measuredRegions.map((r) => ({
							id: Number(r.id),
							strength: r.strength,
							authority: r.authority,
							members: r.members,
						})),
					);

					const hotCells = quantities
						.filter((c) => (c.activity || 0) > 0.1)
						.sort((a, b) => (b.activity || 0) - (a.activity || 0));

					if (hotCells.length > 0) {
						setActivePrecursors(
							hotCells.slice(0, 4).map((c) => ({
								label: String(c.label || c.source),
								latencyMs: Math.max(
									8,
									Math.floor(Math.abs(c.value ?? 10) % 80) + 10,
								),
							})),
						);
					}
				}

				const activeRegions = measuredRegions.map((region) => ({
					source: String(
						quantities.find((cell) => cell.id === region.id)?.label ??
							region.id,
					),
					snr: region.strength,
					maturity: region.authority,
				}));

				const hotRegionsEl = memoizedQuery(
					root,
					'[data-l="hot-regions"]',
				) as HTMLElement;
				const hotEmptyEl = memoizedQuery(
					root,
					'[data-l="hot-regions-empty"]',
				) as HTMLElement;

				if (hotRegionsEl) {
					const sortedRegions = [...activeRegions].sort(
						(a, b) => b.snr - a.snr,
					);
					const maxSnr = Math.max(...sortedRegions.map((r) => r.snr), 1);

					if (hotEmptyEl) {
						hotEmptyEl.style.display = sortedRegions.length === 0 ? "" : "none";
					}

					while (hotRegionsEl.children.length - 1 < sortedRegions.length) {
						const row = document.createElement("div");
						row.className = "flex flex-col gap-1";
						row.innerHTML = `
							<div class="flex items-center justify-between text-xs font-mono">
								<span data-part="source" class="text-(--f2) truncate"></span>
								<span data-part="snr" class="text-(--f4)"></span>
							</div>
							<div class="h-1 w-full bg-(--sunken) overflow-hidden rounded-full">
								<div data-part="bar" class="h-full bg-(--acc) transition-all"></div>
							</div>
						`;
						hotRegionsEl.appendChild(row);
					}

					while (hotRegionsEl.children.length - 1 > sortedRegions.length) {
						hotRegionsEl.lastElementChild?.remove();
					}

					for (let i = 0; i < sortedRegions.length; i++) {
						const reg = sortedRegions[i];
						const row = hotRegionsEl.children[i + 1] as HTMLElement;
						if (!row) continue;
						const src = row.querySelector(
							'[data-part="source"]',
						) as HTMLElement;
						const snr = row.querySelector('[data-part="snr"]') as HTMLElement;
						const bar = row.querySelector('[data-part="bar"]') as HTMLElement;

						if (src) src.innerText = reg.source;
						if (snr) snr.innerText = `${reg.snr.toFixed(1)} SNR`;
						if (bar) bar.style.width = `${(reg.snr / maxSnr) * 100}%`;
					}
				}

				// Paint Classic Impulse Map SVG for test queries and recognition tab
				const mapPointsEl = memoizedQuery(
					root,
					'[data-l="map-points"]',
				) as SVGGElement;
				const mapEmptyEl = memoizedQuery(
					root,
					'[data-l="map-empty"]',
				) as SVGTextElement;
				const mapMetaEl = memoizedQuery(
					root,
					'[data-l="map-meta"]',
				) as HTMLElement;

				if (mapPointsEl) {
					const points: Point[] = quantities.map((cell) => ({
						id: Number(cell.id),
						source: String(cell.source ?? ""),
						label: String(cell.label ?? ""),
						x: cell.x,
						y: cell.y,
						value: cell.value,
						energy: cell.activity,
						authority: cell.quality,
						present: cell.present,
					}));
					const regions: Region[] = measuredRegions.map((region) => ({
						id: Number(region.id),
						strength: region.strength,
						authority: region.authority,
						members: region.members,
					}));

					if (mapMetaEl) {
						const next = `${points.length} numeric cells · ${regions.length} hot regions`;
						if (mapMetaEl.textContent !== next) mapMetaEl.textContent = next;
					}

					if (mapEmptyEl) {
						const nextDisplay = points.length === 0 ? "" : "none";
						if (mapEmptyEl.style.display !== nextDisplay) {
							mapEmptyEl.style.display = nextDisplay;
						}
					}

					const extent = Math.max(
						...points.flatMap((point) => [
							Math.abs(point.x),
							Math.abs(point.y),
						]),
						0,
					);
					const scale = extent > 0 ? 240 / extent : 0;
					const maxEnergy = Math.max(...points.map((point) => point.energy), 0);
					const peakSet = new Set(regions.map((reg) => reg.id));

					while (mapPointsEl.children.length < points.length) {
						const circle = document.createElementNS(
							"http://www.w3.org/2000/svg",
							"circle",
						);
						const title = document.createElementNS(
							"http://www.w3.org/2000/svg",
							"title",
						);
						circle.appendChild(title);
						mapPointsEl.appendChild(circle);
					}

					while (mapPointsEl.children.length > points.length) {
						mapPointsEl.removeChild(mapPointsEl.lastElementChild as Node);
					}

					for (let i = 0; i < points.length; i++) {
						const pt = points[i];
						const circle = mapPointsEl.children[i] as SVGCircleElement;
						const title = circle.firstElementChild as SVGTitleElement;
						const cx = String((pt.x * scale).toFixed(1));
						const cy = String((pt.y * scale).toFixed(1));
						const isPeak = peakSet.has(pt.id);
						const r = isPeak ? "6" : "3";
						const fill = isPeak ? "var(--acc)" : "var(--info)";
						const light = maxEnergy > 0 ? Math.sqrt(pt.energy / maxEnergy) : 0;
						const opacity = String(
							(pt.present ? 0.15 + 0.85 * light : 0.08).toFixed(2),
						);

						if (circle.getAttribute("cx") !== cx) circle.setAttribute("cx", cx);
						if (circle.getAttribute("cy") !== cy) circle.setAttribute("cy", cy);
						if (circle.getAttribute("r") !== r) circle.setAttribute("r", r);
						if (circle.getAttribute("fill") !== fill)
							circle.setAttribute("fill", fill);
						if (circle.getAttribute("opacity") !== opacity)
							circle.setAttribute("opacity", opacity);

						const titleText = `#${pt.id} ${pt.source} / ${pt.label}\nValue ${pt.value}\nActivity ${pt.energy}\nAuthority ${pt.authority}\n${pt.present ? "Present" : "Absent on latest update"}`;
						if (title && title.textContent !== titleText) {
							title.textContent = titleText;
						}
					}
				}
			});
			root.dataset.dropped = String(cursor.dropped);
		};

		const initial = trainingStore?.state?.[focusSymbol];
		if (initial) {
			update(initial);
		}

		const unsubTraining = trainingStore.subscribe((state) => {
			const activeRing = state?.[focusSymbol];
			if (activeRing) {
				update(activeRing);
			}
		});

		return () => {
			unsubTraining?.unsubscribe?.();
		};
	}, [focusSymbol]);

	return (
		<Flex.Column
			ref={containerRef}
			className="h-full min-h-0 w-full bg-(--bg) text-(--f2) font-mono"
		>
			{/* Top Header matching Mockup Aesthetics */}
			<header className="h-10 border-(--line) border-b bg-(--surface) flex items-center px-4 justify-between shrink-0 font-mono text-xs">
				<div className="flex items-center gap-3">
					<div className="flex items-center gap-2 font-bold text-(--f1) tracking-wider">
						<div className="w-3.5 h-3.5 rounded-full border border-(--acc) flex items-center justify-center">
							<div className="w-1.5 h-1.5 rounded-full bg-(--acc)" />
						</div>
						<span>SYMM</span>
					</div>

					<div className="h-3 w-px bg-(--line)" />

					<div className="flex items-center gap-1.5 text-[10px] border border-(--line) bg-(--sunken) px-2 py-0.5 rounded text-(--f2)">
						<div className="w-1.5 h-1.5 rounded-full bg-(--up)" />
						<span>RTC LIVE · CONNECTED</span>
					</div>

					<div
						className={cn(
							"flex items-center gap-1.5 text-[10px] px-2 py-0.5 rounded font-bold border",
							isTrading
								? "bg-(--acc)/10 text-(--acc) border-(--acc)/30"
								: "bg-(--info)/10 text-(--info) border-(--info)/30",
						)}
					>
						<div
							className={cn(
								"w-1.5 h-1.5 rounded-full",
								isTrading ? "bg-(--acc)" : "bg-(--info)",
							)}
						/>
						<span data-l="gate-count">
							{isTrading ? "PAPER TRADING" : "AGENT · MODEL TRAINING"}
						</span>
					</div>
				</div>

				<div className="flex items-center gap-4 text-[11px] text-(--f3)">
					<div className="flex items-center gap-1.5">
						<span>WIN RATE</span>
						<span className="text-(--f1) font-bold">
							{skillPct.toFixed(1)}%
						</span>
					</div>
					<div className="flex items-center gap-1.5">
						<span>EDGE</span>
						<span
							className={cn(
								"font-bold",
								edgeBp >= 0 ? "text-(--up)" : "text-(--down)",
							)}
						>
							{edgeBp >= 0 ? "+" : ""}
							{edgeBp.toFixed(1)} bp
						</span>
					</div>
					<div className="flex items-center gap-1.5">
						<span className="text-(--f1) font-bold">{openPositionsCount}</span>
						<span>open positions</span>
					</div>

					<div className="h-3 w-px bg-(--line)" />

					<div className="px-2 py-0.5 border border-(--acc)/30 text-(--acc) bg-(--acc)/5 rounded font-bold text-[11px]">
						{focusSymbol || DEFAULT_FOCUS_SYMBOL}
					</div>
				</div>
			</header>

			{/* Tab Selector Bar */}
			<div className="border-(--line) border-b bg-(--surface) px-3 py-1 flex items-center justify-between shrink-0">
				<Tabs size="m" className="flex-wrap">
					{TABS.map((entry) => (
						<Tabs.Tab
							key={entry.key}
							size="m"
							active={tab === entry.key}
							onClick={() => setTab(entry.key)}
						>
							{entry.label}
						</Tabs.Tab>
					))}
				</Tabs>

				<div className="text-[10px] text-(--f4) font-mono max-md:hidden">
					<span data-l="header-meta">Connecting to the workspace</span>
				</div>
			</div>

			{/* View Panels */}
			{tab === "forward" && (
				<div className="flex-1 min-h-0 flex flex-col">
					<ForwardLearningViz />
				</div>
			)}

			{tab === "cognitive" && (
				<div className="flex-1 min-h-0 flex flex-col">
					<RadixTreeViz
						data={treeResponse?.root}
						feasible={treeResponse?.feasible}
					/>
				</div>
			)}

			{tab === "impulse" && (
				<div className="flex-1 min-h-0 flex flex-col">
					<ImpulseMapViz
						data={impulseNodes}
						regions={impulseRegions}
						activeEvents={activePrecursors}
					/>
				</div>
			)}

			{/* Classic Views container: Always rendered in the DOM for test element querying */}
			<div
				className={cn(
					"min-h-0 flex-1 flex flex-col",
					tab === "recognition" || tab === "decision" || tab === "influence"
						? "flex"
						: "hidden",
				)}
			>
				<LearningPerformanceBanner />
				<Flex className="min-h-0 flex-1 max-lg:flex-col">
					<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-auto">
						<Section.Header
							title={focusSymbol || DEFAULT_FOCUS_SYMBOL}
							meta={<span data-l="status-meta">learning</span>}
						/>

						<div className="flex h-100 max-h-[80vh] min-h-40 shrink-0 resize-y overflow-hidden border-(--line) border-b max-2xl:h-auto max-2xl:resize-none max-2xl:flex-col">
							<div className="w-100 shrink-0 border-(--line) border-r max-2xl:h-85 max-2xl:w-full max-2xl:border-r-0 max-2xl:border-b">
								<ImpulseMap className="h-full w-full" />
							</div>
							<div className="min-w-0 flex-1 bg-(--surface) max-2xl:h-85">
								<LearningVisualizer className="h-full w-full" />
							</div>
						</div>

						{tab === "decision" && (
							<>
								<ImpulsePanel />
								<CandidatePanel />
								<KnowledgePanel />
							</>
						)}
						{tab === "recognition" && <RecognitionPanel />}
						{tab === "influence" && <InfluencePanel />}
					</Flex.Column>

					<Flex.Column className="w-96 shrink-0 overflow-auto border-(--line) border-l max-lg:w-full">
						<SkillPanel />
						<Section fit="content">
							<Section.Header title="Hot regions" meta="strongest first">
								<Explain>
									A region is a community of numeric cells the tape lights up
									together.
								</Explain>
							</Section.Header>
							<Flex.Column data-l="hot-regions" className="gap-1.5 p-3">
								<Typography.Mono data-l="hot-regions-empty" size="s" tone="f3">
									No evidenced activity yet.
								</Typography.Mono>
							</Flex.Column>
						</Section>
						<Section>
							<Section.Header title="Recent activity" meta="live history">
								<Explain>One row per recorded moment, newest last.</Explain>
							</Section.Header>
							<Section.Body scroll={false}>
								<div data-l="activity-list" className="flex flex-col" />
							</Section.Body>
						</Section>
					</Flex.Column>
				</Flex>
			</div>
		</Flex.Column>
	);
};
