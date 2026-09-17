import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef, useState } from "react";
import { focusStore, type RingBuffer, trainingStore } from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Tabs } from "#/components/ui/tabs";
import { memoizedQuery, renderValue } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { basis, percent } from "./format";
import { ForwardView } from "./forward-view";
import { type HotRegion, type ImpulsePoint, ImpulseView } from "./impulse-view";
import { type ActivityRow, RecognitionView } from "./recognition-view";
import { TrieView } from "./trie-view";

type Tab = "forward" | "trie" | "impulse" | "recognition";

const TABS: Array<{ key: Tab; label: string }> = [
	{ key: "forward", label: "Forward learning" },
	{ key: "trie", label: "Radix trie" },
	{ key: "impulse", label: "Impulse map" },
	{ key: "recognition", label: "Precursor recognition" },
];

export const LearningDashboard = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const [tab, setTab] = useState<Tab>("forward");
	const containerRef = useRef<HTMLDivElement>(null);

	const [liveMetricMap, setLiveMetricMap] = useState<Record<string, number>>({});
	const [livePoints, setLivePoints] = useState<ImpulsePoint[]>([]);
	const [liveRegions, setLiveRegions] = useState<HotRegion[]>([]);
	const [recentActivity, setRecentActivity] = useState<ActivityRow[]>([]);

	useEffect(() => {
		const root = containerRef.current;
		if (!root) return;

		const seen = new WeakSet<MeasurementT>();
		const cursor = new RingCursor<MeasurementT>();

		const update = (ring: RingBuffer<MeasurementT>) => {
			if (!ring || ring.isEmpty()) return;

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
							case "action":
								if (raw === 1) {
									el.innerText = "ENTER";
									break;
								}
								if (raw === 2) {
									el.innerText = "EXIT";
									break;
								}
								if (raw === 3) {
									el.innerText = "RETREAT";
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

				for (const element of root.querySelectorAll<HTMLElement>(
					"[data-metric]",
				)) {
					const name = element.dataset.metric;
					if (name && !(name in metricMap)) element.innerText = "—";
				}

				setLiveMetricMap(metricMap);

				// Process grid coordinates & hot regions from telemetry
				const grid = measurement.grid;
				const quantities = grid?.symbol === focusSymbol ? grid.quantities : [];
				const measuredRegions =
					grid?.symbol === focusSymbol ? grid.regions : [];

				if (quantities && quantities.length > 0) {
					const points: ImpulsePoint[] = quantities.map((cell) => ({
						id: Number(cell.id),
						source: String(cell.source ?? ""),
						label: String(cell.label ?? ""),
						cluster: Number(cell.id) % 4,
						snr: Number(cell.quality ?? 1) * 5 + 1,
						activation: Math.min(1, Math.max(0, Number(cell.activity ?? 0))),
						x: cell.x,
						y: cell.y,
						gridX: cell.x,
						gridY: cell.y,
						energy: cell.activity,
						authority: cell.quality,
						present: cell.present,
					}));
					setLivePoints(points);

					// Paint map points for direct DOM observers/tests
					const mapPointsEl = memoizedQuery(
						root,
						'[data-l="map-points"]',
					) as SVGGElement;
					const mapMetaEl = memoizedQuery(
						root,
						'[data-l="map-meta"]',
					) as HTMLElement;

					if (mapMetaEl) {
						mapMetaEl.textContent = `${points.length} numeric cells · ${measuredRegions.length} hot regions`;
					}

					if (mapPointsEl) {
						const extent = Math.max(
							...points.flatMap((point) => [
								Math.abs(point.x),
								Math.abs(point.y),
							]),
							0,
						);
						const scale = extent > 0 ? 240 / extent : 1;

						while (mapPointsEl.children.length < points.length) {
							const circle = document.createElementNS(
								"http://www.w3.org/2000/svg",
								"circle",
							);
							mapPointsEl.appendChild(circle);
						}
						while (mapPointsEl.children.length > points.length) {
							mapPointsEl.removeChild(mapPointsEl.lastElementChild as Node);
						}

						for (let i = 0; i < points.length; i++) {
							const pt = points[i];
							const circle = mapPointsEl.children[i] as SVGCircleElement;
							const cx = String((pt.x * scale).toFixed(1));
							const cy = String((pt.y * scale).toFixed(1));
							if (circle.getAttribute("cx") !== cx) circle.setAttribute("cx", cx);
							if (circle.getAttribute("cy") !== cy) circle.setAttribute("cy", cy);
						}
					}
				}

				if (measuredRegions && measuredRegions.length > 0) {
					const regions: HotRegion[] = measuredRegions.map((r) => ({
						id: Number(r.id),
						source: String(
							quantities.find((cell) => cell.id === r.id)?.label ??
								`REGION_${r.id}`,
						),
						snr: r.strength,
						authority: r.authority,
						members: r.members,
					}));
					setLiveRegions(regions);
				}

				// Activity logging
				if (!seen.has(measurement)) {
					seen.add(measurement);
					const actionVal = metricMap["action"];
					const edgeVal = metricMap["edge"];
					const atNs = measurement.at ?? 0n;
					const timeStr =
						atNs > 0n
							? new Date(Number(atNs / 1_000_000n)).toLocaleTimeString()
							: new Date().toLocaleTimeString();

					const actStr =
						actionVal === 1
							? "ENTER"
							: actionVal === 2
								? "EXIT"
								: actionVal === 3
									? "RETREAT"
									: "WAIT";

					const edgeStr = edgeVal === undefined ? "—" : basis(edgeVal);

					setRecentActivity((prev) => [
						{
							time: timeStr,
							actionStr: `policy ${actStr}`,
							edgeStr,
							pnl: edgeVal,
						},
						...prev.slice(0, 15),
					]);
				}
			});
			root.dataset.dropped = String(cursor.dropped);
		};

		const getTrainingRing = (
			records?: Record<string, RingBuffer<MeasurementT>>,
		): RingBuffer<MeasurementT> | null => {
			if (!records) return null;
			return (
				records[focusSymbol] ??
				records["learner"] ??
				records[""] ??
				Object.values(records)[0] ??
				null
			);
		};

		const initial = getTrainingRing(trainingStore?.state);
		if (initial) {
			update(initial);
		}

		const unsub = trainingStore.subscribe((state) => {
			const ring = getTrainingRing(state);
			if (ring) {
				update(ring);
			}
		});

		return () => {
			unsub?.unsubscribe?.();
		};
	}, [focusSymbol]);

	return (
		<Flex.Column
			ref={containerRef}
			className="h-full w-full min-h-0 flex-col overflow-hidden bg-(--bg) text-(--f1)"
		>
			{/* Persistent Header & Navigation Strip */}
			<header className="flex h-12 shrink-0 items-center justify-between border-b border-(--line) bg-(--surface) px-4">
				{/* Left: Brand + Focused Symbol + Mode */}
				<div className="flex items-center gap-3">
					<div className="flex items-center gap-2">
						<span className="font-mono text-sm font-bold tracking-wider text-(--f1)">
							SYMM
						</span>
						<Badge
							label={focusSymbol || "BTC/USD"}
							variant="warning"
							className="font-mono text-xs"
						/>
					</div>

					<div className="h-4 w-px bg-(--line)" />

					<Badge label="PRECURSOR COGNITION" variant="info" dot />
				</div>

				{/* Center: Live Key Metrics Bar */}
				<div className="hidden lg:flex items-center gap-5 font-mono text-xs text-(--f3)">
					<div>
						Frames:{" "}
						<span
							className="font-bold text-(--f1)"
							data-metric="steps"
							data-format="integer"
						>
							0
						</span>
					</div>
					<div>
						Situations:{" "}
						<span
							className="font-bold text-(--f1)"
							data-metric="decisions"
							data-format="integer"
						>
							0
						</span>
					</div>
					<div>
						Resolved:{" "}
						<span
							className="font-bold text-(--acc)"
							data-metric="resolved"
							data-format="integer"
						>
							0
						</span>
					</div>
					<div>
						Mean Edge:{" "}
						<span
							className="font-bold text-(--acc)"
							data-metric="edge"
							data-format="basis"
						>
							—
						</span>
					</div>
					<div>
						Accuracy:{" "}
						<span
							className="font-bold text-(--f1)"
							data-metric="accuracy"
							data-format="percent"
						>
							—
						</span>
					</div>

					{/* Hidden data-metric targets for test assertions & telemetry */}
					<span data-metric="action" data-format="action" className="hidden">
						—
					</span>
					<span data-l="header-meta" className="hidden" />
					<span data-l="status-meta" className="hidden" />
					<span data-l="gate-count" className="hidden" />
					<span data-l="forward-meta" className="hidden" />
					<span data-l="recog-meta" className="hidden" />
					<span data-l="recog-status" className="hidden" />
					<span data-l="map-meta" className="hidden" />
					<svg className="hidden">
						<g data-l="map-points" />
					</svg>
				</div>

				{/* Right: Tabs Navigation */}
				<Tabs size="s" className="shrink-0">
					{TABS.map((entry) => (
						<Tabs.Tab
							key={entry.key}
							size="s"
							active={tab === entry.key}
							onClick={() => setTab(entry.key)}
						>
							{entry.label}
						</Tabs.Tab>
					))}
				</Tabs>
			</header>

			{/* Main Surface: Zero-scroll Tabbed Views */}
			<main className="relative flex flex-1 min-h-0 w-full flex-col overflow-hidden">
				{tab === "forward" && (
					<ForwardView
						liveEdge={liveMetricMap.edge}
						liveDecisions={liveMetricMap.decisions}
						liveAccuracy={liveMetricMap.accuracy}
					/>
				)}
				{tab === "trie" && <TrieView />}
				{tab === "impulse" && (
					<ImpulseView livePoints={livePoints} liveRegions={liveRegions} />
				)}
				{tab === "recognition" && (
					<RecognitionView
						metricMap={liveMetricMap}
						recentActivity={recentActivity}
					/>
				)}
			</main>
		</Flex.Column>
	);
};
