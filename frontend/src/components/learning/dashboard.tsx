import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef, useState } from "react";
import {
	focusStore,
	type RingBuffer,
	signals,
	trainingStore,
} from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";
import { memoizedQuery, renderValue } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import type { MetricT } from "../hindsight/hindsight-types";
import { CandidateReview } from "./candidate-review";
import {
	CandidatePanel,
	ForwardPanel,
	ImpulsePanel,
	InfluencePanel,
} from "./decision-panel";
import { Explain } from "./explain";
import { action, basis, clock, percent } from "./format";
import { KnowledgePanel } from "./knowledge-panel";
import { ImpulseMap, type Point, type Region } from "./map";
import { LearningPerformanceBanner } from "./performance-banner";
import { RecognitionPanel } from "./recognition-panel";
import { SkillPanel } from "./skill-panel";
import { LearningVisualizer } from "./visualizer";

type Tab = "decision" | "recognition" | "influence" | "forward";

const TABS: Array<{ key: Tab; label: string }> = [
	{ key: "decision", label: "Model decision" },
	{ key: "recognition", label: "Precursor recognition" },
	{ key: "influence", label: "Precursor discovery" },
	{ key: "forward", label: "Forward test" },
];

export const LearningDashboard = () => {
	const focusSymbol = useSelector(focusStore, (state) => state);
	const [tab, setTab] = useState<Tab>("recognition");
	const containerRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const root = containerRef.current;
		if (!root) return;

		const seen = new WeakSet<MeasurementT>();

		const update = (ring: RingBuffer<MeasurementT>) => {
			if (!ring || ring.isEmpty()) return;

			const len = ring.getBufferLength();
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

			for (let i = 0; i < len; i++) {
				const measurement = ring.get(i);
				if (measurement === undefined) {
					continue;
				}

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
				const support = metricMap.support ?? 0;
				const isTrading = confidence >= 0.7 && contrast > 0.5;

				if (metaEl) {
					metaEl.innerText = `${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations · ${Math.floor(resolved).toLocaleString()} resolved`;
				}

				if (statusMetaEl) {
					statusMetaEl.innerText = `conf: ${(confidence * 100).toFixed(1)}% · contrast: ${contrast.toFixed(2)} bits · edge: ${(edge * 10000).toFixed(1)} bp`;
				}

				if (gateCountEl) {
					const gates = [
						support >= 10,
						edge > 0,
						confidence >= 0.7,
						contrast > 0.5,
					].filter(Boolean).length;
					gateCountEl.innerText = `${gates}/4 criteria`;
				}

				if (forwardMetaEl) {
					forwardMetaEl.innerText = `${Math.floor(resolved).toLocaleString()} completed evaluations`;
				}

				if (recogMetaEl) {
					recogMetaEl.innerText = `${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations`;
				}

				if (recogStatusEl) {
					recogStatusEl.innerText = isTrading
						? "Execution active · Model meets confidence and contrast criteria"
						: "Training precursor associations · Execution remains inert until confident";
				}

				if (skillMetaEl) {
					skillMetaEl.innerText = `${Math.floor(resolved).toLocaleString()} forward evaluations · ${isTrading ? "trading" : "learning"}`;
				}

				if (activityListEl && !seen.has(measurement)) {
					seen.add(measurement);

					const actionVal = metricMap["action"] ?? 0;
					const edgeVal = metricMap["edge"] ?? 0;
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
			}

			// 4. Update Hot Regions and Impulse Map from live signals
			const activeSources = Object.keys(signals).filter(
				(s) => s !== "training",
			);
			const activeRegions: Array<{
				source: string;
				snr: number;
				maturity: number;
				metrics: MetricT[];
			}> = [];

			for (const source of activeSources) {
				const sRing =
					signals[source]?.state[focusSymbol] ?? signals[source]?.state[""];
				if (!sRing || sRing.isEmpty()) continue;
				const lastMeas = sRing.getLast();
				if (!lastMeas) continue;

				activeRegions.push({
					source,
					snr: lastMeas.snr ?? 0,
					maturity: lastMeas.maturity ?? 0,
					metrics: lastMeas.metrics ?? [],
				});
			}

			// Paint Hot Regions
			const hotRegionsEl = memoizedQuery(
				root,
				'[data-l="hot-regions"]',
			) as HTMLElement;
			const hotEmptyEl = memoizedQuery(
				root,
				'[data-l="hot-regions-empty"]',
			) as HTMLElement;

			if (hotRegionsEl) {
				const sortedRegions = [...activeRegions].sort((a, b) => b.snr - a.snr);
				const maxSnr = Math.max(...sortedRegions.map((r) => r.snr), 1);

				if (hotEmptyEl) {
					const nextDisplay = sortedRegions.length === 0 ? "" : "none";
					if (hotEmptyEl.style.display !== nextDisplay) {
						hotEmptyEl.style.display = nextDisplay;
					}
				}

				while (hotRegionsEl.children.length - 1 < sortedRegions.length) {
					const rowDiv = document.createElement("div");
					rowDiv.className = "flex items-center gap-2 font-mono text-xs";
					const nameSpan = document.createElement("span");
					nameSpan.className = "w-24 shrink-0 truncate uppercase text-(--f2)";
					const barTrack = document.createElement("div");
					barTrack.className =
						"flex-1 h-1.5 rounded-full bg-(--surface) overflow-hidden";
					const barFill = document.createElement("div");
					barFill.className = "h-full bg-(--acc) transition-all";
					barTrack.appendChild(barFill);
					const snrSpan = document.createElement("span");
					snrSpan.className = "w-14 shrink-0 text-right text-(--f4)";

					rowDiv.appendChild(nameSpan);
					rowDiv.appendChild(barTrack);
					rowDiv.appendChild(snrSpan);
					hotRegionsEl.appendChild(rowDiv);
				}

				while (hotRegionsEl.children.length - 1 > sortedRegions.length) {
					hotRegionsEl.removeChild(hotRegionsEl.lastElementChild as Node);
				}

				for (let i = 0; i < sortedRegions.length; i++) {
					const r = sortedRegions[i];
					const rowDiv = hotRegionsEl.children[i + 1] as HTMLElement;
					const nameSpan = rowDiv.children[0] as HTMLElement;
					const barFill = (rowDiv.children[1] as HTMLElement)
						.children[0] as HTMLElement;
					const snrSpan = rowDiv.children[2] as HTMLElement;

					const nameText = r.source.toUpperCase();
					const snrText = `${r.snr.toFixed(2)}`;
					const widthPct = `${Math.min(100, Math.max(5, (r.snr / maxSnr) * 100)).toFixed(0)}%`;

					if (nameSpan.textContent !== nameText)
						nameSpan.textContent = nameText;
					if (snrSpan.textContent !== snrText) snrSpan.textContent = snrText;
					if (barFill.style.width !== widthPct) barFill.style.width = widthPct;
				}
			}

			// Paint Impulse Map SVG
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
				const points: Point[] = [];
				const regions: Region[] = [];
				let pointId = 0;

				for (let sIdx = 0; sIdx < activeRegions.length; sIdx++) {
					const r = activeRegions[sIdx];
					const angle =
						(sIdx / Math.max(1, activeRegions.length)) * 2 * Math.PI;
					const radius = 120 + ((r.snr * 15) % 80);
					const sx = Math.cos(angle) * radius;
					const sy = Math.sin(angle) * radius;

					regions.push({
						id: pointId,
						strength: r.snr,
						authority: r.maturity,
						members: r.metrics.length || 1,
					});

					for (const m of r.metrics) {
						if (!m?.name) continue;
						points.push({
							id: pointId++,
							source: r.source,
							label: String(m.name),
							x: sx + ((pointId * 19) % 40) - 20,
							y: sy + ((pointId * 29) % 40) - 20,
							value: m.raw ?? 0,
							energy: r.snr,
							authority: r.maturity,
							present: true,
						});
					}
				}

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
					...points.flatMap((point) => [Math.abs(point.x), Math.abs(point.y)]),
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
		};

		const getTrainingRing = (
			records?: Record<string, RingBuffer<MeasurementT>>,
		): RingBuffer<MeasurementT> | null => {
			if (!records) {
				return null;
			}

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

		const unsubTraining = trainingStore.subscribe((state) => {
			const activeRing = getTrainingRing(state);
			if (activeRing) {
				update(activeRing);
			}
		});

		return () => {
			unsubTraining?.unsubscribe?.();
		};
	}, [focusSymbol]);

	return (
		<Flex.Column ref={containerRef} className="h-full min-h-0 w-full">
			<Section.Header
				title="Precursor recognition"
				meta={<span data-l="header-meta">Connecting to the workspace</span>}
			/>
			<LearningPerformanceBanner />
			<Flex className="min-h-0 flex-1 max-lg:flex-col">
				<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-auto">
					<Section.Header
						title={focusSymbol || "BTC/USD"}
						meta={<span data-l="status-meta">learning</span>}
					>
						<Badge label="learning" variant="info" dot />
					</Section.Header>

					<div className="flex h-100 max-h-[80vh] min-h-40 shrink-0 resize-y overflow-hidden border-(--line) border-b max-2xl:h-auto max-2xl:resize-none max-2xl:flex-col">
						<div className="w-100 shrink-0 border-(--line) border-r max-2xl:h-85 max-2xl:w-full max-2xl:border-r-0 max-2xl:border-b">
							<ImpulseMap className="h-full w-full" />
						</div>
						<div className="min-w-0 flex-1 bg-(--surface) max-2xl:h-85">
							<LearningVisualizer className="h-full w-full" />
						</div>
					</div>

					<Flex.Row
						gap={2}
						className="sticky top-0 z-2 shrink-0 border-(--line) border-b bg-(--surface) px-3 py-2"
					>
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
					</Flex.Row>

					{tab === "decision" && (
						<>
							<ImpulsePanel />
							<CandidatePanel />
							<KnowledgePanel />
						</>
					)}
					{tab === "recognition" && <RecognitionPanel />}
					{tab === "influence" && <InfluencePanel />}
					{tab === "forward" && (
						<>
							<ForwardPanel />
							<CandidateReview />
						</>
					)}
				</Flex.Column>

				<Flex.Column className="w-96 shrink-0 overflow-auto border-(--line) border-l max-lg:w-full">
					<SkillPanel />
					<Section fit="content">
						<Section.Header title="Hot regions" meta="strongest first">
							<Explain>
								A region is a community of numeric cells the tape lights up
								together. Bar length is its energy against the strongest region
								currently lit; authority is how much of the map's evidence
								stands behind it.
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
							<Explain>
								One row per recorded moment, newest last: the time, the call the
								model made, and what it came to — a tape benefit in basis points
								once the record has answered it.
							</Explain>
						</Section.Header>
						<Section.Body scroll={false}>
							<div data-l="activity-list" className="flex flex-col" />
						</Section.Body>
					</Section>
				</Flex.Column>
			</Flex>
		</Flex.Column>
	);
};
