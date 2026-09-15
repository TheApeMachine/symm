import { useEffect, useRef, useState } from "react";
import { useSelector } from "@tanstack/react-store";
import { focusStore, type RingBuffer, trainingStore } from "#/collections/app";
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
import { ImpulseMap } from "./map";
import { LearningPerformanceBanner } from "./performance-banner";
import { RecognitionPanel } from "./recognition-panel";
import { SkillPanel } from "./skill-panel";
import { LearningVisualizer } from "./visualizer";

type Tab =
	| "decision"
	| "recognition"
	| "influence"
	| "forward";

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

		const update = (ring: RingBuffer<MeasurementT>) => {
			if (!ring || ring.isEmpty()) return;

			const len = ring.getBufferLength();
			const metaEl = memoizedQuery(root, '[data-l="header-meta"]') as HTMLElement;
			const statusMetaEl = memoizedQuery(root, '[data-l="status-meta"]') as HTMLElement;
			const gateCountEl = memoizedQuery(root, '[data-l="gate-count"]') as HTMLElement;
			const forwardMetaEl = memoizedQuery(root, '[data-l="forward-meta"]') as HTMLElement;
			const recogMetaEl = memoizedQuery(root, '[data-l="recog-meta"]') as HTMLElement;
			const recogStatusEl = memoizedQuery(root, '[data-l="recog-status"]') as HTMLElement;
			const skillMetaEl = memoizedQuery(root, '[data-l="skill-meta"]') as HTMLElement;

			for (let i = 0; i < len; i++) {
				const measurement = ring.get(i);
				if (!measurement) continue;

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

				const steps = metricMap["steps"] ?? 0;
				const decisions = metricMap["decisions"] ?? 0;
				const resolved = metricMap["resolved"] ?? 0;
				const confidence = metricMap["confidence"] ?? 0;
				const contrast = metricMap["contrast"] ?? 0;
				const edge = metricMap["edge"] ?? 0;
				const support = metricMap["support"] ?? 0;
				const isTrading = confidence >= 0.7 && contrast > 0.5;

				if (metaEl) {
					metaEl.innerText = `${Math.floor(steps).toLocaleString()} frames · ${Math.floor(decisions).toLocaleString()} learned situations · ${Math.floor(resolved).toLocaleString()} resolved`;
				}

				if (statusMetaEl) {
					statusMetaEl.innerText = `conf: ${(confidence * 100).toFixed(1)}% · contrast: ${contrast.toFixed(2)} bits · edge: ${(edge * 10000).toFixed(1)} bp`;
				}

				if (gateCountEl) {
					const gates = [support >= 10, edge > 0, confidence >= 0.7, contrast > 0.5].filter(Boolean).length;
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
			}

			const activityListEl = memoizedQuery(root, '[data-l="activity-list"]') as HTMLElement;
			if (activityListEl) {
				const rows = ring.toArray();
				const items = rows.slice(-15);
				activityListEl.innerHTML = items.map((row) => {
					const mmap: Record<string, number> = {};
					for (const m of row.metrics ?? []) {
						if (m?.name) mmap[String(m.name)] = m.raw ?? 0;
					}
					const actionVal = mmap["action"] ?? 0;
					const edgeVal = mmap["edge"] ?? 0;
					const atNs = row.at ?? 0n;
					const atIso = atNs ? new Date(Number(atNs / 1_000_000n)).toISOString() : new Date().toISOString();
					const actStr = action(actionVal === 1 ? "enter" : actionVal === 2 ? "exit" : "wait", 1, false);
					const edgeStr = basis(edgeVal);
					return `<div class="flex items-center gap-2 border-(--line) border-b px-3 py-1.5 font-mono text-xs">
						<span class="text-(--f4) shrink-0">${clock(atIso)}</span>
						<span class="text-(--f2) min-w-0 flex-1 truncate">${actStr}</span>
						<span class="text-(--acc) shrink-0">${edgeStr}</span>
					</div>`;
				}).join("");
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
	}, [focusSymbol, tab]);

	return (
		<Flex.Column ref={containerRef} className="h-full min-h-0 w-full">
			<Section.Header
				title="Precursor recognition"
				meta={
					<span data-l="header-meta">
						Connecting to the workspace
					</span>
				}
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
						<Flex.Column className="gap-1.5 p-3">
							<Typography.Mono size="s" tone="f3">
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
