import { useSelector } from "@tanstack/react-store";
import { decisionsStore } from "#/collections/decisions";
import { executionsStore } from "#/collections/executions";
import type { MeasurementHistorySample } from "#/collections/measurements";
import { measurementsStore } from "#/collections/measurements";
import { playbookStore, type WalkTrace } from "#/collections/playbook";
import { positionsStore } from "#/collections/positions";
import { terminalStore } from "#/collections/terminal";
import {
	entryLineStats,
	fixed,
	whyLabel,
} from "#/components/terminal/decision-format";
import {
	mergeTerminalDecisionRows,
	terminalDecisionsFromWalk,
} from "#/components/terminal/decisions-from-walk";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
	orderedKernelSources,
	type SignalHealthStatus,
} from "#/components/terminal/kernel-meta";
import { kernelsForFocus } from "#/components/terminal/kernels";
import type { TerminalDecisionRow } from "#/components/terminal/model";

const KERNEL_SPARK_HISTORY_LIMIT = 40;

export type ReadingsState = Record<
	string,
	Record<string, Record<string, unknown>>
>;

export type KernelSparkHistory = {
	scope: string;
	stamp: string;
	values: number[];
};

export const kernelReadingSource = (source: string): string =>
	source === "prediction" ? "resonance" : source;

export const kernelFrameForSource = (
	readings: ReadingsState,
	source: string,
	focusSymbol: string,
): Record<string, unknown> | undefined => {
	const bySymbol = readings[kernelReadingSource(source)];

	if (bySymbol === undefined) {
		return undefined;
	}

	if (
		focusSymbol !== "" &&
		focusSymbol !== "stream" &&
		bySymbol[focusSymbol] !== undefined
	) {
		return bySymbol[focusSymbol] as Record<string, unknown>;
	}

	const fallbackSymbol =
		Object.keys(bySymbol).find((symbol) => symbol.includes("/")) ??
		Object.keys(bySymbol)[0];

	return fallbackSymbol === undefined
		? undefined
		: (bySymbol[fallbackSymbol] as Record<string, unknown>);
};

export const getHealthStatus = (
	origin: string,
	confidence: number,
	surprise: number,
): SignalHealthStatus => {
	if (confidence <= 0) return "waiting";
	if (surprise >= 2.0) return "stale";
	if (surprise >= 1.4) return "flat";
	if (origin === "causal") return "calibrating";
	return "healthy";
};

const finiteScore = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? Math.min(1, Math.max(0, number)) : 0;
};

const numericValue = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const readingOutput = (
	frame: Record<string, unknown> | undefined,
): Record<string, unknown> => (frame?.output ?? {}) as Record<string, unknown>;

const readingNumber = (
	frame: Record<string, unknown> | undefined,
	output: Record<string, unknown>,
	key: string,
): number => {
	const direct = frame?.[key];

	if (direct !== undefined) {
		return numericValue(direct);
	}

	return numericValue(output[key]);
};

const kernelStamp = (
	frame: Record<string, unknown> | undefined,
	output: Record<string, unknown>,
	confidence: number,
	surprise: number,
): string => {
	const stamp =
		frame?.observed_at ??
		frame?.timestamp_unix_nano ??
		frame?.timestamp ??
		frame?.ts ??
		output.observed_at ??
		output.timestamp ??
		output.ts;

	return stamp === undefined ? `${confidence}:${surprise}` : String(stamp);
};

export const kernelReadout = (frame: Record<string, unknown> | undefined) => {
	const output = readingOutput(frame);
	const confidence = finiteScore(frame?.confidence ?? output.confidence);
	const surprise = Math.max(0, readingNumber(frame, output, "surprise"));

	return {
		output,
		confidence,
		surprise,
		stamp: kernelStamp(frame, output, confidence, surprise),
	};
};

const kernelHistory = (
	frame: Record<string, unknown> | undefined,
): MeasurementHistorySample[] =>
	Array.isArray(frame?.history)
		? (frame.history as MeasurementHistorySample[])
		: [];

export const kernelHistoryValues = (
	frame: Record<string, unknown> | undefined,
	fallback: number,
): number[] => {
	const values = kernelHistory(frame).flatMap((sample) =>
		sample.confidence === undefined ? [] : [finiteScore(sample.confidence)],
	);

	return values.length > 0 ? values : [fallback];
};

export const kernelHistoryCount = (
	frame: Record<string, unknown> | undefined,
): number => Math.max(1, kernelHistory(frame).length);

export const appendKernelSparkHistory = (
	history: KernelSparkHistory,
	scope: string,
	stamp: string,
	sample: unknown,
	limit = KERNEL_SPARK_HISTORY_LIMIT,
): KernelSparkHistory => {
	if (history.scope === scope && history.stamp === stamp) {
		return history;
	}

	const maxLength = Math.max(1, limit);
	const prior = history.scope === scope ? history.values : [];
	const values = [...prior, finiteScore(sample)].slice(-maxLength);

	return { scope, stamp, values };
};

export const kernelHealthSummary = (
	readings: ReadingsState,
	focusSymbol: string,
	origins?: string[],
) => {
	const sources = orderedKernelSources(origins ?? Object.keys(readings));
	let healthy = 0;

	for (const origin of sources) {
		const frame = kernelFrameForSource(readings, origin, focusSymbol);
		const { confidence, surprise } = kernelReadout(frame);

		if (getHealthStatus(origin, confidence, surprise) === "healthy") {
			healthy += 1;
		}
	}

	return {
		healthy,
		total: sources.length,
		label: `${healthy}/${sources.length} ok`,
	};
};

const decisionList = (
	frame: Record<string, unknown> | null,
): Array<Record<string, unknown>> => {
	if (Array.isArray(frame?.decisions)) {
		return frame.decisions as Array<Record<string, unknown>>;
	}

	if (frame?.role === "decision") {
		return [frame];
	}

	return [];
};

export const decisionRowsFromFrame = (
	frame: Record<string, unknown> | null,
): TerminalDecisionRow[] => {
	const list = decisionList(frame).filter(
		(decision) => typeof decision.symbol === "string" && decision.symbol !== "",
	);
	const scores = list.map((decision) =>
		finiteScore(decision.score ?? decision.combined ?? decision.confidence),
	);
	const { line } = entryLineStats(scores);

	return list.map((decision, index) => {
		const scoreValue = scores[index] ?? 0;
		const rawVerdict = String(decision.verdict ?? "").toLowerCase();
		const verdict: TerminalDecisionRow["verdict"] =
			rawVerdict === "allow"
				? "allow"
				: rawVerdict === "below" || rawVerdict === "in-play"
					? "in-play"
					: "blocked";
		const edge = scoreValue - line;
		const edgePositive = edge > 0;
		const source = String(decision.source ?? decision.type ?? "decision");
		const symbol = String(decision.symbol);
		const side = String(decision.side ?? "");
		const type = String(decision.type ?? "");
		const id = String(decision.action_id ?? decision.decision_id ?? "");

		return {
			key: id || `${symbol}:${side}:${type}:${rawVerdict}:${fixed(scoreValue)}`,
			symbol,
			source,
			scoreText: fixed(scoreValue),
			scoreValue,
			verdict,
			why: whyLabel(String(decision.why ?? decision.reason ?? rawVerdict)),
			signals:
				source === "decision" ? [] : [{ source, confidence: scoreValue }],
			edgeText: `${edgePositive ? "+" : "−"}${fixed(Math.abs(edge))} / ${fixed(Math.abs(line))}`,
			edgePositive,
		};
	});
};

export const dashboardDecisionRows = (
	readings: Record<string, Record<string, Record<string, unknown>>>,
	focusSymbol: string,
	walkEvaluations: Record<string, WalkTrace>,
	decisionFrame: Record<string, unknown> | null,
) => {
	const walkRows = terminalDecisionsFromWalk(walkEvaluations, (symbol) =>
		kernelsForFocus(readings, symbol),
	);
	const traceRows = decisionRowsFromFrame(decisionFrame);
	const rows = mergeTerminalDecisionRows(walkRows, traceRows);
	const scores = rows.map((row) => row.scoreValue);
	const { line } = entryLineStats(scores);

	return { rows, line };
};

const verdictMeta = (verdict: TerminalDecisionRow["verdict"]) => {
	if (verdict === "allow") {
		return {
			label: "ALLOW",
			bg: "color-mix(in srgb, var(--up) 16%, transparent)",
			fg: "var(--up)",
		};
	}

	if (verdict === "blocked") {
		return {
			label: "DENY",
			bg: "color-mix(in srgb, var(--down) 16%, transparent)",
			fg: "var(--down)",
		};
	}

	return {
		label: "BELOW",
		bg: "var(--line)",
		fg: "var(--f3)",
	};
};

const KernelRow = ({
	compact,
	frame,
	inspecting,
	origin,
	selected,
}: {
	compact: boolean;
	frame: Record<string, unknown> | undefined;
	inspecting: boolean;
	origin: string;
	selected: boolean;
}) => {
	const { inspectSource, selectSource } = terminalStore.actions;
	const { output, confidence, surprise } = kernelReadout(frame);
	const copy = kernelCopy(origin, String(output.category ?? origin));
	const confidenceText = `${Math.round(confidence * 100)}%`;
	const healthStatus = getHealthStatus(origin, confidence, surprise);
	const statusMeta = kernelStatusMeta(healthStatus);
	const sparkValues = kernelHistoryValues(frame, confidence);
	const spark = kernelSparkPaths(sparkValues, surprise);
	const surpColor = spark.firing ? "var(--acc)" : "var(--f4)";

	return (
		<button
			type="button"
			onClick={() => (compact ? selectSource(origin) : inspectSource(origin))}
			className="block w-full cursor-pointer border-(--line) border-b border-l-2 px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
			style={{
				borderLeftColor: inspecting || selected ? "var(--acc)" : "transparent",
				background: inspecting || selected ? "var(--raised)" : "transparent",
			}}
		>
			<div className="flex items-center justify-between gap-2">
				<span
					className={`truncate font-semibold text-(--f1) ${compact ? "text-xs" : "text-[12.5px]"}`}
				>
					{copy.name}
				</span>
				{compact ? (
					<span
						className="size-[7px] shrink-0 rounded-full"
						style={{ backgroundColor: statusMeta.fg }}
					/>
				) : (
					<span
						className="shrink-0 rounded-[2px] border px-1.5 py-0.5 text-[9px] font-semibold tracking-wider uppercase"
						style={{
							borderColor: statusMeta.bd,
							backgroundColor: statusMeta.bg,
							color: statusMeta.fg,
						}}
					>
						{statusMeta.label}
					</span>
				)}
			</div>
			<div className="mt-0.5 truncate font-mono text-[9.5px] text-(--f4)">
				{compact ? `${confidenceText} conf` : copy.sub}
			</div>
			{compact ? null : (
				<>
					<svg
						viewBox="0 0 150 30"
						preserveAspectRatio="none"
						className="mt-1.5 block h-[26px] w-full"
					>
						<title>Signal sparkline</title>
						<polyline points={spark.area} fill={spark.fill} stroke="none" />
						<polyline
							points={spark.spark}
							fill="none"
							stroke={spark.line}
							strokeWidth="1.4"
							vectorEffect="non-scaling-stroke"
						/>
					</svg>
					<div className="mt-1.5 flex items-center gap-2">
						<div className="h-1 flex-1 overflow-hidden rounded-[2px] bg-(--line)">
							<div
								className="h-full transition-all duration-500 ease-out"
								style={{
									width: `${Math.round(confidence * 100)}%`,
									backgroundColor: spark.line,
								}}
							/>
						</div>
						<span className="w-7 text-right font-mono text-[10px] text-(--f2)">
							{confidenceText}
						</span>
						<span
							className="w-[62px] text-right font-mono text-[9.5px]"
							style={{ color: surpColor }}
						>
							{surprise.toFixed(2)}× thr
						</span>
					</div>
				</>
			)}
		</button>
	);
};

export const KernelList = ({
	compact = false,
	origins,
}: {
	compact?: boolean;
	origins?: string[];
}) => {
	const readings = useSelector(measurementsStore, (state) => state);
	const inspectorSource = useSelector(
		terminalStore,
		(state) => state.inspectorSource,
	);
	const selectedSource = useSelector(
		terminalStore,
		(state) => state.selectedSource,
	);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const sources = orderedKernelSources(origins ?? Object.keys(readings));

	return (
		<div className="min-h-0 overflow-auto">
			{sources.map((origin) => {
				const frame = kernelFrameForSource(readings, origin, focusSymbol);
				const inspecting = inspectorSource === origin;
				const selected = selectedSource === origin;

				return (
					<KernelRow
						key={origin}
						compact={compact}
						frame={frame}
						inspecting={inspecting}
						origin={origin}
						selected={selected}
					/>
				);
			})}
		</div>
	);
};

export const DecisionLineMeta = () => {
	const readings = useSelector(measurementsStore, (state) => state);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const walkEvaluations = useSelector(
		playbookStore,
		(state) => state.evaluations,
	);
	const decisionFrame = useSelector(decisionsStore, (state) => state.frame);
	const { line, rows } = dashboardDecisionRows(
		readings,
		focusSymbol,
		walkEvaluations,
		decisionFrame,
	);

	return <>{rows.length > 0 ? `line ${fixed(line)}` : "line —"}</>;
};

export const DecisionRows = () => {
	const readings = useSelector(measurementsStore, (state) => state);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const walkEvaluations = useSelector(
		playbookStore,
		(state) => state.evaluations,
	);
	const decisionFrame = useSelector(decisionsStore, (state) => state.frame);
	const { rows } = dashboardDecisionRows(
		readings,
		focusSymbol,
		walkEvaluations,
		decisionFrame,
	);

	return (
		<div className="min-h-0 flex-1 overflow-auto">
			<table className="w-full border-collapse text-[11.5px]">
				<thead>
					<tr className="sticky top-0 bg-(--surface) text-[9.5px] text-(--f4) uppercase tracking-[0.06em]">
						<th className="px-3 py-1.5 text-left font-medium">Symbol</th>
						<th className="px-1.5 py-1.5 text-right font-medium">Comb</th>
						<th className="px-1.5 py-1.5 text-right font-medium">Edge</th>
						<th className="px-3 py-1.5 text-left font-medium">Verdict</th>
					</tr>
				</thead>
				<tbody className="divide-y divide-(--line)">
					{rows.length === 0 ? (
						<tr>
							<td
								colSpan={4}
								className="px-3 py-8 text-center font-mono text-(--f4) text-xs"
							>
								Waiting for playbook decisions...
							</td>
						</tr>
					) : (
						rows.map((decision) => {
							const meta = verdictMeta(decision.verdict);
							return (
								<tr key={decision.key} className="hover:bg-(--raised)">
									<td className="px-3 py-1.5 font-mono text-[11px] font-semibold text-(--f1)">
										{decision.symbol}
									</td>
									<td className="px-1.5 py-1.5 text-right font-mono text-(--f2)">
										{decision.scoreText}
									</td>
									<td
										className="px-1.5 py-1.5 text-right font-mono text-[10px]"
										style={{
											color: decision.edgePositive
												? "var(--up)"
												: "var(--down)",
										}}
									>
										{decision.edgeText}
									</td>
									<td className="px-3 py-1.5">
										<span
											className="rounded-[2px] px-1.5 py-0.5 text-[9px] font-semibold tracking-wider"
											style={{ backgroundColor: meta.bg, color: meta.fg }}
										>
											{meta.label}
										</span>
									</td>
								</tr>
							);
						})
					)}
				</tbody>
			</table>
		</div>
	);
};

type AuditItem = {
	key: string;
	reason: string;
	meta: string;
	time: string;
};

const observedAtMilliseconds = (value: unknown): number | undefined => {
	if (typeof value === "number" && Number.isFinite(value)) {
		return value;
	}

	if (typeof value !== "string" || value.trim() === "") {
		return undefined;
	}

	const parsed = Date.parse(value);

	return Number.isFinite(parsed) ? parsed : undefined;
};

export const auditRowsFromDecisionFrame = (
	frame: Record<string, unknown> | null,
): AuditItem[] => {
	const observedAt = observedAtMilliseconds(frame?.observed_at);
	const clock =
		observedAt === undefined
			? ""
			: new Date(observedAt).toLocaleTimeString("en-US", { hour12: false });
	const seq =
		typeof frame?.seq === "number" || typeof frame?.seq === "string"
			? `#${String(frame.seq)}`
			: "";
	const time = [seq, clock].filter(Boolean).join(" · ");

	return decisionList(frame).flatMap((decision) => {
		const symbol = String(decision.symbol ?? "");

		if (symbol === "") {
			return [];
		}

		const score = finiteScore(
			decision.score ?? decision.combined ?? decision.confidence,
		);
		const verdict = String(decision.verdict ?? "blocked").toLowerCase();
		const reason =
			verdict === "allow"
				? `candidate scored ${fixed(score)}`
				: whyLabel(String(decision.why ?? decision.reason ?? verdict));
		const metaVerb =
			verdict === "allow"
				? "score"
				: verdict === "blocked"
					? "reject"
					: verdict;
		const meta = [
			metaVerb,
			symbol,
			decision.source ?? decision.type ?? decision.side ?? "decision",
		]
			.filter(Boolean)
			.join(" · ");

		return [
			{
				key: `decision:${symbol}:${verdict}:${fixed(score)}:${meta}`,
				reason,
				meta,
				time,
			},
		];
	});
};

const currencySymbolFor = (quoteCurrency: string): string =>
	quoteCurrency === "EUR" ? "€" : "$";

const signedCurrency = (
	value: number,
	currencySymbol: string,
	decimals: number,
): string => {
	const sign = value >= 0 ? "+" : "-";

	return `${sign}${currencySymbol}${Math.abs(value).toFixed(decimals)}`;
};

type PositionRow = {
	key: string;
	symbol: string;
	detail: string;
	pctText: string;
	plText: string;
	pnl: number;
	pnlPositive: boolean;
};

export const positionRowsFromFrames = (
	positionsFrame: Record<string, unknown> | null,
) => {
	const positions =
		(positionsFrame?.positions as Array<Record<string, unknown>> | undefined) ??
		[];
	const quoteCurrency = String(
		positions[0]?.quote ?? positionsFrame?.quote ?? "EUR",
	).toUpperCase();
	const currencySymbol = currencySymbolFor(quoteCurrency);
	const rows = positions.map((position): PositionRow => {
		const symbol = String(position.symbol ?? "");
		const entry = numericValue(position.entry);
		const mark = numericValue(position.mark);
		const pnl = numericValue(position.unrealizedPnl ?? position.unrealized_pnl);
		const pct = numericValue(position.changePct ?? position.change_pct);
		const pnlPositive = pnl >= 0;
		const pctSign = pct >= 0 ? "+" : "-";
		const stop = numericValue(position.stop ?? position.stopPrice);
		const peak = numericValue(position.peak ?? position.peakPrice);
		const stopDetail = stop > 0 ? ` · stop ${stop.toFixed(2)}` : "";
		const peakDetail = peak > 0 ? ` · peak ${peak.toFixed(2)}` : "";

		return {
			key: String(position.id ?? position.asset ?? symbol),
			symbol,
			detail: `entry ${entry > 0 ? entry.toFixed(2) : "—"} · mark ${mark > 0 ? mark.toFixed(2) : "—"}${stopDetail}${peakDetail}`,
			pctText: `${pctSign}${Math.abs(pct).toFixed(2)}%`,
			plText: `P/L ${signedCurrency(pnl, currencySymbol, 4)}`,
			pnl,
			pnlPositive,
		};
	});

	const netPnl = rows.reduce((sum, row) => sum + row.pnl, 0);

	return {
		currencySymbol,
		netPnl,
		netPositive: netPnl >= 0,
		netText: `net ${signedCurrency(netPnl, currencySymbol, 2)}`,
		quoteCurrency,
		rows,
	};
};

export const PositionLineMeta = () => {
	const positions = useSelector(positionsStore, (state) => state.frame);
	const summary = positionRowsFromFrames(positions);

	if (summary.rows.length === 0) {
		return <>—</>;
	}

	return (
		<span style={{ color: summary.netPositive ? "var(--up)" : "var(--down)" }}>
			{summary.netText}
		</span>
	);
};

export const PositionRows = () => {
	const positions = useSelector(positionsStore, (state) => state.frame);
	const summary = positionRowsFromFrames(positions);

	if (summary.rows.length === 0) {
		return (
			<div className="min-h-0 flex-1 overflow-auto p-1.5">
				<div className="px-2 py-8 text-center font-mono text-(--f4) text-xs">
					No open positions
				</div>
			</div>
		);
	}

	return (
		<div className="min-h-0 flex-1 overflow-auto p-1.5 space-y-1.5">
			{summary.rows.map((row) => {
				const plFg = row.pnlPositive ? "var(--up)" : "var(--down)";
				return (
					<div
						key={row.key}
						className="rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-1.5 font-mono text-xs hover:bg-(--raised)"
					>
						<div className="flex items-center justify-between">
							<span className="font-semibold text-(--f1)">{row.symbol}</span>
							<span className="font-semibold" style={{ color: plFg }}>
								{row.plText}
							</span>
						</div>
						<div className="mt-1 flex items-center justify-between text-[9.5px] text-(--f4)">
							<span>{row.detail}</span>
							<span style={{ color: plFg }}>{row.pctText}</span>
						</div>
					</div>
				);
			})}
		</div>
	);
};

export const AuditRows = () => {
	const history = useSelector(executionsStore, (state) => state.history);
	const decisionFrame = useSelector(decisionsStore, (state) => state.frame);
	const decisionAudit = auditRowsFromDecisionFrame(decisionFrame);

	if (history.length === 0 && decisionAudit.length === 0) {
		return (
			<div className="min-h-0 flex-1 overflow-auto py-0.5">
				<div className="px-3 py-8 text-center font-mono text-(--f4) text-xs">
					No audit events
				</div>
			</div>
		);
	}

	return (
		<div className="min-h-0 flex-1 overflow-auto divide-y divide-(--line)">
			{decisionAudit.map((item) => (
				<div key={item.key} className="px-3 py-1.5 hover:bg-(--raised)">
					<div className="flex items-start justify-between gap-2">
						<span className="font-sans font-medium text-[11px] text-(--f1)">
							{item.reason}
						</span>
						<span className="shrink-0 font-mono text-[9px] text-(--f4)">
							{item.time}
						</span>
					</div>
					<div className="mt-0.5 font-mono text-[9px] text-(--f4)">
						{item.meta}
					</div>
				</div>
			))}
			{history.map((item) => {
				const symbol = String(item.symbol || "unknown");
				const side = String(item.side || "trade").toUpperCase();
				const qty = Number(item.order_qty || item.last_qty || 0).toFixed(4);
				const price = Number(
					item.last_price || item.avg_price || item.price || 0,
				).toFixed(2);
				const execType = String(item.exec_type || "fill");
				const status = String(item.order_status || "settled");
				const key = String(
					item.id ??
						item.order_id ??
						item.txid ??
						item.exec_id ??
						item.timestamp_unix_nano ??
						`${symbol}:${side}:${qty}:${price}:${status}`,
				);

				const reason = `${side === "BUY" ? "Position Opened" : "Position Settled"}: ${symbol}`;
				const time = item.observed_at
					? new Date(Number(item.observed_at)).toLocaleTimeString("en-US", {
							hour12: false,
						})
					: new Date().toLocaleTimeString("en-US", { hour12: false });
				const meta = `qty ${qty} · px ${price} · ${execType} (${status})`;

				return (
					<div key={key} className="px-3 py-1.5 hover:bg-(--raised)">
						<div className="flex items-start justify-between gap-2">
							<span className="font-sans font-medium text-[11px] text-(--f1)">
								{reason}
							</span>
							<span className="shrink-0 font-mono text-[9px] text-(--f4)">
								{time}
							</span>
						</div>
						<div className="mt-0.5 font-mono text-[9px] text-(--f4)">
							{meta}
						</div>
					</div>
				);
			})}
		</div>
	);
};
