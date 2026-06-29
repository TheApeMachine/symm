import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { decisionsStore } from "#/collections/decisions";
import { measurementsStore } from "#/collections/measurements";
import type { WalkTrace } from "#/collections/playbook";
import { playbookStore } from "#/collections/playbook";
import { tickStore } from "#/collections/tick";
import { entryLineStats, fixed } from "#/components/terminal/decision-format";
import { DecisionSideRail } from "#/components/terminal/decision-side";
import {
	combinedScoreFromKernels,
	decisionRowFromWalk,
	mergeTerminalDecisionRows,
} from "#/components/terminal/decisions-from-walk";
import type {
	TerminalDecisionRow,
	TerminalKernel,
} from "#/components/terminal/model";
import { decisionRowsFromFrame } from "#/components/terminal/rows";

type ReadingsState = Record<string, Record<string, Record<string, unknown>>>;

type CandidateBar = {
	source: string;
	confidence: number;
	pct: number;
	value: string;
	color: string;
};

type AttributionBar = {
	source: string;
	left: number;
	width: number;
	color: string;
	deltaText: string;
};

type CounterfactualProbe = {
	label: string;
	deltaText: string;
	verdict: "ALLOW" | "BELOW";
	bg: string;
	fg: string;
};

type DecisionTreeModel = {
	funnel: Array<{
		label: string;
		value: string | number;
		sub: string;
		accent?: "info" | "acc" | "up";
	}>;
	entry: { line: number; median: number; mad: number };
	rows: TerminalDecisionRow[];
};

const clamp = (value: number, min: number, max: number): number =>
	Math.min(max, Math.max(min, value));

const finite = (value: unknown): number | null => {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value;
	}

	if (typeof value === "string" && value.trim() !== "") {
		const parsed = Number(value);

		if (Number.isFinite(parsed)) {
			return parsed;
		}

		const leading = Number.parseFloat(value);

		return Number.isFinite(leading) ? leading : null;
	}

	return null;
};

const outputOf = (frame: Record<string, unknown> | undefined) =>
	(frame?.output ?? {}) as Record<string, unknown>;

const frameMetric = (
	frame: Record<string, unknown> | undefined,
	key: string,
): number | null => {
	const direct = finite(frame?.[key]);

	if (direct !== null) {
		return direct;
	}

	return finite(outputOf(frame)[key]);
};

const accentColor = (accent?: "info" | "acc" | "up") => {
	if (accent === "info") {
		return "var(--info)";
	}

	if (accent === "up") {
		return "var(--up)";
	}

	if (accent === "acc") {
		return "var(--acc)";
	}

	return "var(--f1)";
};

const verdictStyle = (
	verdict: "allow" | "in-play" | "blocked",
): { label: string; bg: string; fg: string } => {
	if (verdict === "allow") {
		return {
			label: "ALLOW",
			bg: "color-mix(in srgb, var(--up) 16%, transparent)",
			fg: "var(--up)",
		};
	}

	if (verdict === "blocked") {
		return {
			label: "BLOCKED",
			bg: "color-mix(in srgb, var(--down) 14%, transparent)",
			fg: "var(--down)",
		};
	}

	return { label: "BELOW", bg: "var(--line)", fg: "var(--f3)" };
};

export const kernelsForSymbol = (
	readings: ReadingsState,
	symbol: string,
): TerminalKernel[] => {
	const kernels: TerminalKernel[] = [];

	for (const origin of Object.keys(readings)) {
		const frame = readings[origin]?.[symbol];

		if (frame === undefined) {
			continue;
		}

		const confidence = frameMetric(frame, "confidence") ?? 0;
		const surprise = frameMetric(frame, "surprise") ?? 0;

		kernels.push({
			source: origin,
			confidencePercent: Math.round(clamp(confidence, 0, 1) * 100),
			surprisePercent: Math.round(clamp(surprise, 0, 1) * 100),
		});
	}

	return kernels;
};

const rowsFromLiveFrames = (
	readings: ReadingsState,
	evaluations: Record<string, WalkTrace>,
	decisionFrame:
		| Record<string, unknown>
		| Array<Record<string, unknown>>
		| null,
): TerminalDecisionRow[] => {
	const walkInputs = Object.values(evaluations).map((walkTrace) => ({
		walkTrace,
		kernels: kernelsForSymbol(readings, walkTrace.symbol),
	}));
	const walkScores = walkInputs.map(({ kernels, walkTrace }) => {
		const depth = walkTrace.active_path?.length ?? 0;
		const matched = walkTrace.steps.filter(
			(step) => step.outcome === "matched" || step.outcome === "parked",
		).length;

		return Math.min(
			1,
			combinedScoreFromKernels(kernels) + depth * 0.08 + matched * 0.04,
		);
	});
	const { line } = entryLineStats(walkScores);
	const walkRows = walkInputs.map(({ walkTrace, kernels }) =>
		decisionRowFromWalk(walkTrace, kernels, line),
	);
	const traceRows = decisionRowsFromFrame(decisionFrame);

	return mergeTerminalDecisionRows(walkRows, traceRows);
};

export const decisionTreeModel = (
	readings: ReadingsState,
	evaluations: Record<string, WalkTrace>,
	decisionFrame:
		| Record<string, unknown>
		| Array<Record<string, unknown>>
		| null,
	tick: Record<string, unknown> | null,
): DecisionTreeModel => {
	const rows = rowsFromLiveFrames(readings, evaluations, decisionFrame);
	const entry = entryLineStats(rows.map((row) => row.scoreValue));
	const symbols = new Set<string>();

	for (const bySymbol of Object.values(readings)) {
		for (const symbol of Object.keys(bySymbol)) {
			symbols.add(symbol);
		}
	}

	for (const row of rows) {
		symbols.add(row.symbol);
	}

	const scanned = finite(tick?.quotes_total) ?? symbols.size;
	const quoted =
		finite(tick?.quotes_ready) ?? finite(tick?.quotes) ?? symbols.size;
	const inPlay = rows.filter((row) => row.scoreValue >= entry.line).length;
	const allowed = rows.filter((row) => row.verdict === "allow").length;

	return {
		entry,
		rows,
		funnel: [
			{ label: "Scanned", value: Math.round(scanned), sub: "universe" },
			{
				label: "Quoted",
				value: Math.round(quoted),
				sub: "fresh ticks",
				accent: "info",
			},
			{ label: "In play", value: inPlay, sub: "≥ entry line", accent: "acc" },
			{ label: "Allowed", value: allowed, sub: "edge clears", accent: "up" },
		],
	};
};

export const candidateBarsForRow = (
	row: TerminalDecisionRow,
	readings: ReadingsState,
	entryLine: number,
): CandidateBar[] => {
	const bySource = new Map<string, number>();

	for (const signal of row.signals) {
		bySource.set(signal.source, clamp(signal.confidence, 0, 1));
	}

	for (const origin of Object.keys(readings)) {
		const frame = readings[origin]?.[row.symbol];

		if (frame === undefined) {
			continue;
		}

		const confidence = frameMetric(frame, "confidence") ?? 0;
		const surprise = frameMetric(frame, "surprise") ?? 0;
		const score = clamp(Math.max(confidence, surprise * 0.75), 0, 1);
		const previous = bySource.get(origin) ?? 0;

		if (score > previous) {
			bySource.set(origin, score);
		}
	}

	if (bySource.size === 0 && row.scoreValue > 0) {
		bySource.set(row.source, clamp(row.scoreValue, 0, 1));
	}

	return [...bySource.entries()]
		.map(([source, confidence]) => ({
			source,
			confidence,
			pct: Math.round(confidence * 100),
			value: confidence.toFixed(2),
			color: confidence >= entryLine ? "var(--acc)" : "var(--info)",
		}))
		.sort((left, right) => right.confidence - left.confidence)
		.slice(0, 3);
};

export const attributionBars = (
	bars: CandidateBar[],
	entryLine: number,
): AttributionBar[] =>
	bars.map((bar) => {
		const delta = bar.confidence - entryLine;
		const width = Math.max(2, Math.min(50, Math.abs(delta) * 100));
		const positive = delta >= 0;

		return {
			source: bar.source,
			left: positive ? 50 : 50 - width,
			width,
			color: positive ? "var(--up)" : "var(--down)",
			deltaText: `${positive ? "+" : "-"}${fixed(Math.abs(delta))}`,
		};
	});

const firstMetric = (
	frame: Record<string, unknown> | undefined,
	keys: string[],
): number | null => {
	for (const key of keys) {
		const value = frameMetric(frame, key);

		if (value !== null) {
			return value;
		}
	}

	return null;
};

const whaleCarrierRatio = (
	frame: Record<string, unknown> | undefined,
): number | null => {
	const carriers = Array.isArray(frame?.carriers) ? frame.carriers : [];

	if (carriers.length === 0) {
		return null;
	}

	const whales = carriers.filter(
		(carrier) =>
			typeof carrier === "object" &&
			carrier !== null &&
			(carrier as Record<string, unknown>).role === "whale",
	).length;

	return whales / carriers.length;
};

export const counterfactualProbes = (
	readings: ReadingsState,
	row: TerminalDecisionRow,
	entryLine: number,
): CounterfactualProbe[] => {
	const causal = readings.causal?.[row.symbol];
	const manifold = readings.manifold?.[row.symbol];
	const liquidity = readings.liquidity?.[row.symbol];
	const probes = [
		{
			label: "do(vol ↑)",
			value: firstMetric(causal, ["uplift", "alpha"]),
		},
		{
			label: "do(regime = chop)",
			value: firstMetric(causal, ["beta", "shock"]),
		},
		{
			label: "do(liquidity ↓)",
			value: firstMetric(liquidity, ["confidence", "surprise"]),
		},
		{
			label: "do(whale carrier)",
			value:
				whaleCarrierRatio(manifold) ??
				firstMetric(manifold, ["confidence", "shockScore"]),
		},
	];

	return probes.map((probe) => {
		if (probe.value === null) {
			return {
				label: probe.label,
				deltaText: "—",
				verdict: "BELOW",
				bg: "var(--line)",
				fg: "var(--f4)",
			};
		}

		const delta = probe.value - row.scoreValue;
		const clears = row.scoreValue + delta >= entryLine;

		return {
			label: probe.label,
			deltaText: `${delta >= 0 ? "+" : "-"}${fixed(Math.abs(delta))}`,
			verdict: clears ? "ALLOW" : "BELOW",
			bg: clears
				? "color-mix(in srgb, var(--up) 16%, transparent)"
				: "var(--line)",
			fg: clears ? "var(--up)" : "var(--f3)",
		};
	});
};

const FunnelCard = ({
	label,
	value,
	sub,
	accent,
}: DecisionTreeModel["funnel"][number]) => (
	<div className="relative flex-1 rounded border border-(--line) bg-(--surface) px-3 py-2.5">
		<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
			{label}
		</div>
		<div
			className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1]"
			style={{ color: accentColor(accent) }}
		>
			{value}
		</div>
		<div className="mt-px font-mono text-[9.5px] text-(--f4)">{sub}</div>
	</div>
);

const EmptyCandidateList = () => (
	<div className="rounded border border-(--line) bg-(--surface) px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
		waiting for playbook walk frames
	</div>
);

/*
DecisionTreeView renders the single decision point using the tmp terminal's
layout: live universe funnel, adaptive entry line, selectable candidate rows,
score attribution, and causal/cognitive side panels. The surface only projects
backend frames already in the stores; missing inputs render as empty cells.
*/
export const DecisionTreeView = () => {
	const evaluations = useSelector(playbookStore, (state) => state.evaluations);
	const readings = useSelector(measurementsStore, (state) => state);
	const tick = useSelector(tickStore, (state) => state.frame);
	const decisionFrames = useSelector(decisionsStore, (state) => state.frames);
	const [expandedKey, setExpandedKey] = useState<string | null>(null);
	const model = decisionTreeModel(readings, evaluations, decisionFrames, tick);
	const activeKey = model.rows.some((row) => row.key === expandedKey)
		? expandedKey
		: model.rows[0]?.key;
	const leader =
		model.rows.find((row) => row.key === activeKey) ?? model.rows[0];

	return (
		<div className="grid h-full min-h-0 min-w-[1040px] grid-cols-[minmax(640px,1fr)_332px]">
			<div className="min-h-0 overflow-auto px-5 py-[18px]">
				<div className="mb-[18px] flex items-stretch gap-2.5">
					{model.funnel.map((card) => (
						<FunnelCard key={card.label} {...card} />
					))}
				</div>

				<div className="mb-3.5 flex items-center gap-3.5 rounded border border-(--line) bg-(--sunken) px-3 py-2 font-mono text-[11.5px]">
					<span className="text-(--f3)">entry line</span>
					<span className="font-semibold text-(--acc)">
						{fixed(model.entry.line)}
					</span>
					<span className="text-(--f4)">·</span>
					<span className="text-(--f3)">
						median {fixed(model.entry.median)}
					</span>
					<span className="text-(--f4)">·</span>
					<span className="text-(--f3)">mad {fixed(model.entry.mad)}</span>
					<span className="ml-auto text-(--f4)">
						support gate ≥ 2 · edge ≥ req
					</span>
				</div>

				<div className="mb-2 flex items-baseline justify-between">
					<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Candidate evaluation
					</span>
					<span className="font-mono text-[9.5px] text-(--f4)">
						click a row to inspect attribution + counterfactuals
					</span>
				</div>

				<div className="flex flex-col gap-[7px]">
					{model.rows.length === 0 ? <EmptyCandidateList /> : null}
					{model.rows.map((row) => {
						const style = verdictStyle(row.verdict);
						const scorePct = clamp(row.scoreValue * 100, 0, 100);
						const bars = candidateBarsForRow(row, readings, model.entry.line);
						const expanded = row.key === activeKey;
						const attributions = attributionBars(bars, model.entry.line);
						const probes = counterfactualProbes(
							readings,
							row,
							model.entry.line,
						);

						return (
							<div
								key={row.key}
								className="overflow-hidden rounded border bg-(--surface)"
								style={{
									borderColor: expanded ? "var(--line2)" : "var(--line)",
								}}
							>
								<button
									type="button"
									className="grid w-full cursor-pointer grid-cols-[78px_1fr_132px_92px] items-center gap-3 px-3 py-2.5 text-left hover:bg-(--raised)"
									onClick={() => setExpandedKey(row.key)}
								>
									<div>
										<div className="font-mono font-semibold text-[13px] text-(--f1)">
											{row.symbol}
										</div>
										<div className="font-mono text-[9px] text-(--f4)">
											×{bars.length} src
										</div>
									</div>

									<div className="flex flex-col gap-1">
										{bars.map((bar) => (
											<div key={bar.source} className="flex items-center gap-2">
												<span className="w-16 font-mono text-[9px] text-(--f4)">
													{bar.source}
												</span>
												<div className="h-1 flex-1 overflow-hidden rounded-sm bg-(--line)">
													<div
														className="h-full"
														style={{
															width: `${bar.pct}%`,
															background: bar.color,
														}}
													/>
												</div>
												<span className="w-[30px] text-right font-mono text-[9px] text-(--f3)">
													{bar.value}
												</span>
											</div>
										))}
									</div>

									<div>
										<div className="mb-0.5 flex items-center justify-between font-mono text-[9.5px] text-(--f4)">
											<span>combined</span>
											<span className="text-(--f1)">{row.scoreText}</span>
										</div>
										<div className="relative h-1.5 overflow-hidden rounded-sm bg-(--line)">
											<div
												className="h-full"
												style={{
													width: `${scorePct}%`,
													background: row.edgePositive
														? "var(--up)"
														: "var(--down)",
												}}
											/>
										</div>
										<div className="relative h-0">
											<div
												className="absolute top-[-9px] h-3 w-0.5 bg-(--acc)"
												style={{
													left: `${clamp(model.entry.line * 100, 0, 100)}%`,
												}}
											/>
										</div>
										<div
											className="mt-1.5 font-mono text-[9px]"
											style={{
												color: row.edgePositive ? "var(--up)" : "var(--down)",
											}}
										>
											edge {row.edgeText}
										</div>
									</div>

									<div className="text-right">
										<span
											className="inline-block rounded-sm px-2.5 py-1 font-semibold text-[10px] uppercase"
											style={{ background: style.bg, color: style.fg }}
										>
											{style.label}
										</span>
										<div className="mt-1.5 font-mono text-[9px] text-(--f4)">
											{row.why}
										</div>
									</div>
								</button>

								{expanded ? (
									<div className="grid grid-cols-2 gap-5 border-(--line) border-t bg-(--sunken) px-3.5 py-3">
										<div>
											<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.1em]">
												Score attribution
											</div>
											<div className="flex flex-col gap-1.5">
												{attributions.map((bar) => (
													<div
														key={bar.source}
														className="flex items-center gap-2"
													>
														<span className="w-[60px] font-mono text-[9.5px] text-(--f4)">
															{bar.source}
														</span>
														<div className="relative h-3 flex-1 rounded-sm bg-(--line)">
															<div className="absolute inset-y-0 left-1/2 w-px bg-(--f4)" />
															<div
																className="absolute top-px bottom-px rounded-[1px]"
																style={{
																	left: `${bar.left}%`,
																	width: `${bar.width}%`,
																	background: bar.color,
																}}
															/>
														</div>
														<span
															className="w-[50px] text-right font-mono text-[9.5px]"
															style={{ color: bar.color }}
														>
															{bar.deltaText}
														</span>
													</div>
												))}
											</div>
											<div className="mt-2 font-mono text-[9px] text-(--f4)">
												signals ·{" "}
												{bars
													.map((bar) => `${bar.source} ${bar.value}`)
													.join(" · ") || "—"}
											</div>
										</div>

										<div>
											<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-[0.1em]">
												Counterfactual probes · do(·)
											</div>
											<div className="flex flex-col gap-1.5">
												{probes.map((probe) => (
													<div
														key={probe.label}
														className="flex items-center justify-between gap-2 rounded-sm border border-(--line) bg-(--surface) px-2.5 py-1.5"
													>
														<span className="font-mono text-[10px] text-(--f2)">
															{probe.label}
														</span>
														<span className="flex items-center gap-2">
															<span className="font-mono text-[9.5px] text-(--f4)">
																Δ {probe.deltaText}
															</span>
															<span
																className="rounded-sm px-2 py-px font-semibold text-[9px]"
																style={{
																	background: probe.bg,
																	color: probe.fg,
																}}
															>
																{probe.verdict}
															</span>
														</span>
													</div>
												))}
											</div>
										</div>
									</div>
								) : null}
							</div>
						);
					})}
				</div>
			</div>

			<div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
				<DecisionSideRail symbol={leader?.symbol} />
			</div>
		</div>
	);
};
