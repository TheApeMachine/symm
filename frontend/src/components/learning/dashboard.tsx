import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef, useState } from "react";
import { focusStore, type RingBuffer, trainingStore } from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";
import { memoizedQuery, renderValue } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
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

				for (const element of root.querySelectorAll<HTMLElement>(
					"[data-metric]",
				)) {
					const name = element.dataset.metric;
					if (name && !(name in metricMap)) element.innerText = "—";
				}
				const steps = metricMap.steps ?? 0;
				const decisions = metricMap.decisions ?? 0;
				const resolved = metricMap.resolved ?? 0;

				const edge = metricMap.edge;
				const evaluated = metricMap.evaluated ?? 0;

				if (metaEl) {
					metaEl.innerText = `${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations · ${Math.floor(resolved).toLocaleString()} resolved`;
				}

				if (statusMetaEl) {
					statusMetaEl.innerText = `${Math.floor(evaluated).toLocaleString()} evaluations · edge: ${edge === undefined ? "—" : basis(edge)}`;
				}

				if (gateCountEl) {
					gateCountEl.innerText = "Training only";
				}

				if (forwardMetaEl) {
					forwardMetaEl.innerText = `${Math.floor(evaluated).toLocaleString()} completed evaluations`;
				}

				if (recogMetaEl) {
					recogMetaEl.innerText = `${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations`;
				}

				if (recogStatusEl) {
					recogStatusEl.innerText =
						"Training precursor associations · Quoted returns, no orders";
				}

				if (skillMetaEl) {
					skillMetaEl.innerText = `${Math.floor(evaluated).toLocaleString()} forward evaluations · learning`;
				}

				if (activityListEl && !seen.has(measurement)) {
					seen.add(measurement);

					const actionVal = metricMap["action"];
					const edgeVal = metricMap["edge"];
					const atNs = measurement.at ?? 0n;
					const timeStr =
						atNs > 0n
							? clock(new Date(Number(atNs / 1_000_000n)).toISOString())
							: clock("");
					const actStr =
						actionVal === undefined
							? "UNSEEN"
							: action(
									actionVal === 1 ? "enter" : actionVal === 2 ? "exit" : "wait",
									1,
									false,
								);
					const edgeStr = edgeVal === undefined ? "—" : basis(edgeVal);

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

				// Coordinates, membership and activity are the backend's actual readout.
				const grid = measurement.grid;
				const quantities = grid?.symbol === focusSymbol ? grid.quantities : [];
				const measuredRegions =
					grid?.symbol === focusSymbol ? grid.regions : [];
				const activeRegions = measuredRegions.map((region) => ({
					source: String(
						quantities.find((cell) => cell.id === region.id)?.label ??
							region.id,
					),
					snr: region.strength,
					maturity: region.authority,
				}));

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
					const sortedRegions = [...activeRegions].sort(
						(a, b) => b.snr - a.snr,
					);
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
						if (barFill.style.width !== widthPct)
							barFill.style.width = widthPct;
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
