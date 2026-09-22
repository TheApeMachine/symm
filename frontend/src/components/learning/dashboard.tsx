import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef, useState } from "react";
import { focusAtom, type RingBuffer, signals } from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { Tabs } from "#/components/ui/tabs";
import { renderValue } from "#/lib/utils";
import type { WireMeasurement } from "#/types/capnp/measurement";
import { basis, percent } from "./format";
import { ForwardView } from "./forward-view";
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
	const focusSymbol = useSelector(focusAtom, (state) => state);
	const [tab, setTab] = useState<Tab>("forward");
	const containerRef = useRef<HTMLDivElement>(null);

	const [liveMetricMap, setLiveMetricMap] = useState<Record<string, number>>(
		{},
	);
	const [recentActivity, setRecentActivity] = useState<ActivityRow[]>([]);

	useEffect(() => {
		const root = containerRef.current;
		if (!root) return;

		const seen = new WeakSet<WireMeasurement>();
		const cursor = new RingCursor<WireMeasurement>();

		const update = (ring: RingBuffer<WireMeasurement>) => {
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
			records?: Record<string, RingBuffer<WireMeasurement>>,
		): RingBuffer<WireMeasurement> | null => {
			if (!records) return null;
			return (
				records[focusSymbol] ??
				records["learner"] ??
				records[""] ??
				Object.values(records)[0] ??
				null
			);
		};

		const initial = getTrainingRing(signals.training?.state);
		if (initial) {
			update(initial);
		}

		const unsub = signals.training.subscribe((state: any) => {
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
					<Flex.Column className="p-6" gap={3}>
						<Typography.Label>Impulse map unavailable</Typography.Label>
						<Typography.Label>
							The current feed does not provide grid coordinates or regions.
						</Typography.Label>
					</Flex.Column>
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
