import {
	DEFAULT_FOCUS_SYMBOL,
	focusAtom,
	positionCountAtom,
	type RingBuffer,
	signals,
} from "#/collections/app";
import { RingCursor } from "#/collections/ring";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Tabs } from "#/components/ui/tabs";
import { Typography } from "#/components/ui/typography";
import { hubBaseUrl } from "#/lib/hub";
import { cn, memoizedQuery, renderValue } from "#/lib/utils";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef, useState } from "react";
import { CandidatePanel, ImpulsePanel, InfluencePanel } from "./decision-panel";
import { Explain } from "./explain";
import { action, basis, clock, outcome, percent, prediction } from "./format";
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
];

const getTrainingRing = (
	records: Record<string, RingBuffer<MeasurementT>> | undefined,
	focus: string,
): RingBuffer<MeasurementT> | null => {
	if (!records) return null;
	return (
		records[focus] ??
		records.learner ??
		records[""] ??
		Object.values(records)[0] ??
		null
	);
};

export const LearningDashboard = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);
	const [tab, setTab] = useState<Tab>("forward");
	const containerRef = useRef<HTMLDivElement>(null);

	const [openPositionsCount, setOpenPositionsCount] = useState(0);

	// Staged training state
	const [frozenPrediction, setFrozenPrediction] = useState("ABSTAIN");
	const [delayedOutcome, setDelayedOutcome] = useState("RESOLVING");
	const [precursorTokens, setPrecursorTokens] = useState<string[]>([
		"unavailable",
	]);
	const [abcMarkers, setAbcMarkers] = useState({ a: 0, b: 0, c: 0 });

	// Impulse Map real nodes & regions
	const knownNodesRef = useRef<Map<string, ImpulseNode>>(new Map());
	const [impulseNodes, setImpulseNodes] = useState<ImpulseNode[]>([]);
	const [gridTotals, setGridTotals] = useState<{
		cells?: number;
		regions?: number;
	}>({});
	const [impulseRegions, setImpulseRegions] = useState<
		Array<{
			id: number;
			strength: number;
			authority: number;
			members: number;
		}>
	>([]);
	const [activePrecursors, setActivePrecursors] = useState<
		Array<{ id: number; label: string; activity: number; members?: number }>
	>([]);

	// Real Radix Tree data from backend
	const [treeResponse, setTreeResponse] =
		useState<CognitionTreeResponse | null>(null);

	// Listen to open positions count
	useEffect(() => {
		const unsubPositions = positionCountAtom.subscribe((count) => {
			setOpenPositionsCount(count);
		});
		setOpenPositionsCount(positionCountAtom.get());

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

		const setText = (el: HTMLElement, val: string) => {
			el.textContent = val;
			el.innerText = val;
		};

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
				for (const metric of measurement.metrics ?? []) {
					if (metric?.name) metricMap[String(metric.name)] = metric.raw ?? 0;
				}

				for (const m of measurement.metrics ?? []) {
					if (!m?.name) continue;
					const name = String(m.name);
					const raw = m.raw ?? 0;

					const els = root.querySelectorAll(`[data-metric="${name}"]`);
					for (let j = 0; j < els.length; j++) {
						const el = els[j] as HTMLElement;
						const fmt = el.getAttribute("data-format");

						switch (fmt) {
							case "percent":
								setText(el, percent(raw));
								break;
							case "basis":
								setText(el, basis(raw));
								break;
							case "integer":
								setText(el, Math.floor(raw).toLocaleString());
								break;
							case "bits":
								setText(el, `${raw.toFixed(2)} bits`);
								break;
							case "spread":
								setText(el, `Spread ${raw.toFixed(3)}`);
								break;
							case "surprisal":
								setText(el, `Surprisal ${raw.toFixed(2)} nat`);
								break;
							case "insufficient_if_zero": {
								// Missed opportunities are not predicted returns. A measured
								// zero is valid only when actual predictions were resolved.
								let ready = false;
								if (
									name === "hist_mean_return" ||
									name === "hist_lower_bound" ||
									name === "hist_return_se"
								) {
									const resolved =
										(metricMap.hist_correct_enter ?? 0) +
										(metricMap.hist_false_enter ?? 0);
									ready = resolved > (name === "hist_mean_return" ? 0 : 1);
								} else if (
									name === "fwd_paper_mean_return" ||
									name === "fwd_paper_lower_bound"
								) {
									ready = (metricMap.fwd_paper_trades ?? 0) > 0;
								} else {
									ready = raw !== 0;
								}
								setText(el, ready ? basis(raw) : "—");
								break;
							}
							case "action":
								if (raw === 1) {
									setText(el, "ENTER");
									break;
								}
								if (raw === 2) {
									setText(el, "EXIT");
									break;
								}
								setText(el, "ABSTAIN");
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
				const edgeReady = (metricMap.edge_sample_count ?? 0) > 0;
				const evaluated = metricMap.evaluated ?? 0;
				const tradingActive = (metricMap.trading ?? 0) > 0;

				// win_rate = skill accuracy %; edge = ProfitFraction mean → bp.
				// Never format skill±1 as bp; never pretend 0 when absent.
				if (resolved <= 0) {
					for (const name of ["win_rate", "accuracy"] as const) {
						const els = root.querySelectorAll(`[data-metric="${name}"]`);
						for (let j = 0; j < els.length; j++) {
							setText(els[j] as HTMLElement, "—");
						}
					}
				}
				if (!edgeReady) {
					const els = root.querySelectorAll('[data-metric="edge"]');
					for (let j = 0; j < els.length; j++) {
						setText(els[j] as HTMLElement, "—");
					}
				}

				if (metaEl) {
					metaEl.innerText = `${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations · ${Math.floor(resolved).toLocaleString()} resolved`;
				}

				if (statusMetaEl) {
					const edgeLabel = edgeReady
						? `edge: ${(edge * 10000).toFixed(1)} bp`
						: "edge: —";
					statusMetaEl.innerText = `conf: ${(confidence * 100).toFixed(1)}% · contrast: ${contrast.toFixed(2)} bits · ${edgeLabel}`;
				}

				// Update backend-owned canonical stage and blocker (Section 33)
				const sCode = metricMap.stage_code ?? 0;
				let stageStr = "MODEL DEVELOPMENT";
				if (sCode === 1) stageStr = "HISTORICAL VALIDATION";
				if (sCode === 2) stageStr = "FORWARD PAPER LEARNING";
				if (sCode === 3) stageStr = "FORWARD SKILL DEMONSTRATED";

				let blockerStr = "";
				if (measurement.provenance) {
					for (const p of measurement.provenance) {
						if (p?.name === "stage" && p.value) {
							stageStr = String(p.value);
						}
						if (p?.name === "stage_blocker" && p.value !== undefined) {
							blockerStr = String(p.value);
						}
					}
				}

				// Frozen pre-outcome prediction & delayed label
				const actStr = prediction(metricMap.action);

				const extVal = metricMap.excursion_type ?? 0;
				const precLen = Math.floor(metricMap.precursor_length ?? 0);
				const markA = Math.floor(metricMap.mark_a ?? 0);
				const markB = Math.floor(metricMap.mark_b ?? 0);
				const markC = Math.floor(metricMap.mark_c ?? 0);

				let frozenPredStr = actStr;
				const fwdRaw = metricMap.frozen_prediction;

				if (fwdRaw !== undefined) {
					frozenPredStr = prediction(fwdRaw);
				}

				let parsedTokens: string[] = [];
				let direction = "";
				if (measurement.provenance) {
					for (const p of measurement.provenance) {
						if (p?.name === "precursor_tokens" && p.value) {
							parsedTokens = String(p.value).split(",").filter(Boolean);
						}
						if (p?.name === "excursion_direction" && p.value) {
							direction = String(p.value);
						}
					}
				}
				if (measurement.metadata) {
					for (const m of measurement.metadata) {
						if (
							parsedTokens.length === 0 &&
							m?.name === "precursor_tokens" &&
							m.value
						) {
							parsedTokens = String(m.value).split(",").filter(Boolean);
						}
						if (m?.name === "excursion_direction" && m.value) {
							direction = String(m.value);
						}
					}
				}

				const namedOutcome = outcome(direction, extVal);
				let delayedTargetStr = namedOutcome || "RESOLVING";
				const targetRaw = metricMap.delayed_target;

				if (targetRaw === 1) {
					delayedTargetStr = "ENTER";
				}

				if (targetRaw === 2) {
					delayedTargetStr = "EXIT";
				}

				if (targetRaw !== undefined && targetRaw !== 1 && targetRaw !== 2) {
					delayedTargetStr = "ABSTAIN";
				}

				let displayTokens: string[];
				if (parsedTokens.length > 0) {
					displayTokens = parsedTokens.map((t) => `[${t}]`);
				} else if (precLen === 0) {
					displayTokens = ["―"];
				} else {
					displayTokens = ["unavailable"];
				}

				const blockerText = blockerStr || "—";
				setFrozenPrediction(frozenPredStr);
				setDelayedOutcome(delayedTargetStr);
				setPrecursorTokens(displayTokens);
				setAbcMarkers({ a: markA, b: markB, c: markC });

				const stageEls = root.querySelectorAll('[data-l="training-stage"]');
				stageEls.forEach((el) => {
					setText(el as HTMLElement, stageStr);
				});

				const blockerEls = root.querySelectorAll('[data-l="stage-blocker"]');
				blockerEls.forEach((el) => {
					setText(el as HTMLElement, blockerText);
				});

				if (gateCountEl) {
					setText(gateCountEl, stageStr);
				}

				const frozenEls = root.querySelectorAll('[data-l="frozen-prediction"]');
				frozenEls.forEach((el) => {
					setText(el as HTMLElement, frozenPredStr);
				});

				const delayedEls = root.querySelectorAll('[data-l="delayed-label"]');
				delayedEls.forEach((el) => {
					setText(el as HTMLElement, delayedTargetStr);
				});

				// Temporal precursor sequence & ABC boundaries (Section 36)
				const precEls = root.querySelectorAll('[data-l="temporal-precursor"]');
				precEls.forEach((el) => {
					setText(el as HTMLElement, displayTokens.join(" → "));
				});

				const abcEls = root.querySelectorAll('[data-l="abc-markers"]');
				abcEls.forEach((el) => {
					setText(el as HTMLElement, `A: ${markA} · B: ${markB} · C: ${markC}`);
				});

				const posEls = root.querySelectorAll('[data-l="paper-position"]');
				posEls.forEach((el) => {
					setText(
						el as HTMLElement,
						openPositionsCount > 0
							? `${openPositionsCount} positions active`
							: "None",
					);
				});

				if (metaEl) {
					setText(
						metaEl,
						`${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations · ${Math.floor(resolved).toLocaleString()} resolved`,
					);
				}

				if (statusMetaEl) {
					const edgeLabel =
						(metricMap.edge_sample_count ?? 0) > 0
							? `edge: ${(edge * 10000).toFixed(1)} bp`
							: "edge: —";
					setText(
						statusMetaEl,
						`conf: ${(confidence * 100).toFixed(1)}% · contrast: ${contrast.toFixed(2)} bits · ${edgeLabel}`,
					);
				}

				if (forwardMetaEl) {
					setText(
						forwardMetaEl,
						`${Math.floor(evaluated).toLocaleString()} completed evaluations`,
					);
				}

				if (recogMetaEl) {
					setText(
						recogMetaEl,
						`${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations`,
					);
				}

				if (recogStatusEl) {
					setText(
						recogStatusEl,
						tradingActive
							? "Execution active · Paper trading enabled"
							: "Training precursor associations · Quoted returns, orders gated",
					);
				}

				if (skillMetaEl) {
					setText(
						skillMetaEl,
						`${Math.floor(evaluated).toLocaleString()} forward evaluations · ${tradingActive ? "trading" : "learning"}`,
					);
				}

				if (activityListEl && !seen.has(measurement)) {
					seen.add(measurement);

					const actionVal = metricMap.action;
					const edgeVal = metricMap.edge ?? 0;
					const atNs = measurement.at ?? 0n;
					const timeStr =
						atNs > 0n
							? clock(new Date(Number(atNs / 1_000_000n)).toISOString())
							: clock("");
					const actStr =
						actionVal === 1
							? action("enter", 1, false)
							: actionVal === 2
								? action("exit", 1, false)
								: "ABSTAIN";
					const edgeStr =
						(metricMap.edge_sample_count ?? 0) > 0 ? basis(edgeVal) : "—";

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

				if (metricMap.grid_cells !== undefined || metricMap.grid_regions !== undefined) {
					const cells = metricMap.grid_cells;
					const regions = metricMap.grid_regions;
					setGridTotals((held) =>
						held.cells === cells && held.regions === regions
							? held
							: { cells, regions },
					);
				}

				const grid = measurement.grid;
				const isGridSymbolMatch =
					!grid?.symbol ||
					grid.symbol === focusSymbol ||
					grid.symbol === "learner" ||
					focusSymbol === "";
				const quantities = isGridSymbolMatch ? (grid?.quantities ?? []) : [];
				let normalizedRegions: Array<{
					id: number;
					strength: number;
					authority: number;
					members: number;
				}> =
					isGridSymbolMatch && grid?.regions
						? grid.regions.map((r) => ({
								id: Number(r.id),
								strength: r.strength ?? 0,
								authority: r.authority ?? 1.0,
								members: r.members ?? 0,
							}))
						: [];

				let mappedNodes: ImpulseNode[] = [];
				if (quantities.length > 0) {
					mappedNodes = quantities.map((cell, index) => ({
						id: `${grid?.symbol || focusSymbol || "cell"}:${String(cell.id)}:${index}`,
						label: String(cell.label ?? cell.source ?? `#${cell.id}`),
						cluster: Number(cell.basin || 0),
						snr: cell.quality ?? 0,
						activation: cell.activity || 0,
						value: cell.value ?? 0,
						x: cell.x,
						y: cell.y,
						present: cell.present ?? true,
					}));
				} else {
					// Mirror the backend grid: only sensory peers feed cells.
					// Training's own report metrics and the resonance/manifold
					// solvers never become grid cells.
					const allPeers = (measurement.peers ?? []).filter((peer) => {
						const source = String(peer?.source || "");
						return (
							source !== "resonance" &&
							source !== "manifold" &&
							source !== String(measurement.source || "")
						);
					});

					if (allPeers.length > 0) {
						const regionActivity: Record<
							number,
							{ activity: number; members: number }
						> = {};
						for (const peer of allPeers) {
							if (!peer) continue;
							for (const metric of peer.metrics ?? []) {
								if (!metric || !metric.name) continue;
								const nameStr = String(metric.name);
								const x = Number(metric.x || 0n);
								const y = Number(metric.y || 0n);
								const regionId = metric.region || 0;
								const raw = metric.raw || 0;
								let activity = 0;
								if (metric.hasNormalized === true) {
									activity = Math.min(1, Math.abs(metric.normalized || 0));
								} else if (raw !== 0) {
									activity = Math.abs(raw) < 1 ? Math.abs(raw) : 1.0;
								}
								// Node identity is the grid CellKey: the metric label with
								// any "@<peer symbol>" pair qualifier dropped. Source and
								// symbol are not part of a cell's identity.
								const cellKey = nameStr.split("@")[0] ?? nameStr;
								mappedNodes.push({
									id: cellKey,
									label: cellKey,
									cluster: regionId,
									snr: 1,
									activation: activity,
									value: raw,
									x: x,
									y: y,
									present: true,
								});
								if (regionId > 0) {
									if (!regionActivity[regionId]) {
										regionActivity[regionId] = { activity: 0, members: 0 };
									}
									regionActivity[regionId].activity += activity;
									regionActivity[regionId].members += 1;
								}
							}
						}
						if (normalizedRegions.length === 0) {
							normalizedRegions = Object.entries(regionActivity).map(
								([idStr, data]) => ({
									id: Number(idStr),
									strength: data.members > 0 ? data.activity / data.members : 0,
									authority: 1.0,
									members: data.members,
								}),
							);
						}
					}
				}

				if (mappedNodes.length > 0) {
					for (const node of mappedNodes) {
						knownNodesRef.current.set(node.id, node);
					}
					const currentNodes = Array.from(knownNodesRef.current.values());

					if (normalizedRegions.length === 0) {
						const regionActivity: Record<
							number,
							{ activity: number; members: number }
						> = {};
						for (const node of currentNodes) {
							if (node.cluster > 0) {
								if (!regionActivity[node.cluster]) {
									regionActivity[node.cluster] = { activity: 0, members: 0 };
								}
								regionActivity[node.cluster].activity += node.activation;
								regionActivity[node.cluster].members += 1;
							}
						}
						normalizedRegions = Object.entries(regionActivity).map(
							([idStr, data]) => ({
								id: Number(idStr),
								strength: data.members > 0 ? data.activity / data.members : 0,
								authority: 1.0,
								members: data.members,
							}),
						);
					}

					setImpulseRegions(normalizedRegions);

					// LitRegions publishes precursor_tokens — authoritative lit set.
					const litFromTokens = new Set<number>();
					for (const tok of parsedTokens) {
						const n = Number(tok.replace(/^R/i, ""));
						if (Number.isFinite(n) && n > 0) litFromTokens.add(n);
					}

					const regionById = new Map(
						normalizedRegions.map((r) => [Number(r.id), r] as const),
					);

					let litRegions =
						litFromTokens.size > 0
							? [...litFromTokens].map((id) => {
									const known = regionById.get(id);
									return {
										id,
										strength: known?.strength ?? 0,
										authority: known?.authority ?? 1,
										members:
											known?.members ??
											currentNodes.filter((c) => Number(c.cluster) === id)
												.length,
									};
								})
							: [...normalizedRegions].filter((r) => (r.strength || 0) > 0);

					litRegions = litRegions.sort(
						(a, b) => (b.strength || 0) - (a.strength || 0),
					);

					const litSet = new Set(litRegions.map((r) => Number(r.id)));

					const displayNodes = currentNodes.map((node) => {
						const lit = litSet.size === 0 || litSet.has(Number(node.cluster));
						const base = node.activation || 0;
						return {
							...node,
							activation: lit
								? Math.max(base, litSet.size > 0 ? 0.85 : base)
								: base * 0.15,
						};
					});
					setImpulseNodes(displayNodes);

					if (litRegions.length > 0) {
						setActivePrecursors(
							litRegions.map((r) => {
								const peakCell = currentNodes.find(
									(c) => Number(c.cluster) === Number(r.id),
								);
								const name = peakCell?.label
									? `Region #${r.id} (${peakCell.label})`
									: `Region #${r.id}`;
								return {
									id: Number(r.id),
									label: name,
									activity: r.strength || 0,
									members: r.members,
								};
							}),
						);
					}
				}

				const activeRegions = normalizedRegions.map((region) => ({
					source: String(
						mappedNodes.find((cell) => cell.cluster === region.id)?.label ??
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
					const points: Array<Point & { basin?: number }> =
						quantities.length > 0
							? quantities.map((cell, index) => ({
									id: `${String(cell.id)}:${index}`,
									source: String(cell.source ?? ""),
									label: String(cell.label ?? ""),
									x: cell.x,
									y: cell.y,
									value: cell.value,
									energy: cell.activity,
									authority: cell.quality,
									present: cell.present,
									basin: Number(cell.basin || 0),
								}))
							: mappedNodes.map((cell, index) => ({
									id: `${cell.id}:${index}`,
									source: String(cell.label),
									label: String(cell.label),
									x: cell.x || 0,
									y: cell.y || 0,
									value: cell.value || 0,
									energy: cell.activation,
									authority: cell.snr,
									present: cell.present || false,
									basin: Number(cell.cluster || 0),
								}));
					const regions: Region[] = normalizedRegions.map((region) => ({
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
					const peakSet = new Set<number>(regions.map((reg) => Number(reg.id)));

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
						const isPeak = peakSet.has(Number(pt.basin ?? -1));
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

		const initial = getTrainingRing(signals.training?.state, focusSymbol);
		if (initial) {
			update(initial);
		}

		const unsubTraining = signals.training.subscribe((state) => {
			const activeRing = getTrainingRing(state, focusSymbol);
			if (activeRing) {
				update(activeRing);
			}
		});

		return () => {
			unsubTraining?.unsubscribe?.();
		};
	}, [focusSymbol, openPositionsCount]);

	return (
		<Flex.Column
			ref={containerRef}
			className="h-full min-h-0 w-full bg-(--bg) text-(--f2) font-mono"
		>
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

			{/* Temporal Precursor & Boundary Bar (Section 36) */}
			<div className="border-(--line) border-b bg-(--sunken) px-3 py-1 flex items-center justify-between text-[10px] text-(--f3) shrink-0 font-mono">
				<div className="flex items-center gap-2">
					<span className="text-(--f4) uppercase font-bold tracking-wider">
						Temporal Precursor:
					</span>
					<span data-l="temporal-precursor" className="text-(--acc) font-bold">
						{precursorTokens.join(" → ")}
					</span>
				</div>
				<div className="flex items-center gap-3">
					<div className="flex items-center gap-1.5">
						<span className="text-(--f4)">Boundaries:</span>
						<span data-l="abc-markers" className="text-(--f2)">
							A: {abcMarkers.a} · B: {abcMarkers.b} · C: {abcMarkers.c}
						</span>
					</div>
					<div className="h-2.5 w-px bg-(--line)" />
					<div className="flex items-center gap-1.5">
						<span className="text-(--f4)">Pre-Outcome Prediction:</span>
						<span data-l="frozen-prediction" className="text-(--acc) font-bold">
							{frozenPrediction}
						</span>
					</div>
					<div className="h-2.5 w-px bg-(--line)" />
					<div className="flex items-center gap-1.5">
						<span className="text-(--f4)">Delayed Label:</span>
						<span data-l="delayed-label" className="text-(--f1) font-bold">
							{delayedOutcome}
						</span>
					</div>
				</div>
			</div>

			{/* View Panels */}
			{tab === "forward" && (
				<div className="flex-1 min-h-0 flex flex-col">
					<ForwardLearningViz symbol={focusSymbol} />
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
						gridCells={gridTotals.cells}
						gridRegions={gridTotals.regions}
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

						<div className={tab === "decision" ? "" : "hidden"}>
							<ImpulsePanel />
							<CandidatePanel />
							<KnowledgePanel />
						</div>
						<div className={tab === "recognition" ? "" : "hidden"}>
							<RecognitionPanel />
						</div>
						<div className={tab === "influence" ? "" : "hidden"}>
							<InfluencePanel />
						</div>
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
