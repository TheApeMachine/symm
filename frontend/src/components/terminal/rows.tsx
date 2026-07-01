import { useSelector } from "@tanstack/react-store";
import { decisionFunnelStore } from "#/collections/decision-funnel";
import { decisionsStore } from "#/collections/decisions";
import { diagnosticsStore } from "#/collections/diagnostics";
import { executionsStore } from "#/collections/executions";
import type { MeasurementHistorySample } from "#/collections/measurements";
import { measurementsStore } from "#/collections/measurements";
import type { WalkTrace } from "#/collections/playbook";
import { positionsStore } from "#/collections/positions";
import { terminalStore } from "#/collections/terminal";
import { fixed, whyLabel } from "#/components/terminal/decision-format";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
	orderedKernelSources,
	type SignalHealthStatus,
} from "#/components/terminal/kernel-meta";
import type { TerminalDecisionRow } from "#/components/terminal/model";
import { isConcreteSymbol, resolveScopedFrame } from "./scoped-frame";

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

	const scoped = resolveScopedFrame(bySymbol, focusSymbol, source);

	if (isConcreteSymbol(focusSymbol) && scoped.mode !== "concrete") {
		return undefined;
	}

	return scoped.frame ?? undefined;
};

const BACKEND_STATUSES = new Set<string>([
	"waiting",
	"standby",
	"calibrating",
	"fault",
	"ambiguous",
	"measured",
	"unknown",
]);

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

const outputMetric = (output: Record<string, unknown>, key: string): number =>
	numericValue(output[key]);

export const kernelStatus = (
	frame: Record<string, unknown> | undefined,
): SignalHealthStatus => {
	if (frame === undefined) {
		return "waiting";
	}

	const output = readingOutput(frame);
	const status =
		typeof output.status === "string"
			? output.status
			: typeof frame.status === "string"
				? frame.status
				: "";

	return BACKEND_STATUSES.has(status)
		? (status as SignalHealthStatus)
		: "unknown";
};

const kernelStamp = (
	frame: Record<string, unknown> | undefined,
	output: Record<string, unknown>,
): string => {
	const stamp =
		frame?.observed_at ??
		frame?.timestamp_unix_nano ??
		frame?.timestamp ??
		frame?.ts ??
		output.observed_at ??
		output.timestamp ??
		output.ts;

	return stamp === undefined ? "" : String(stamp);
};

export const kernelReadout = (frame: Record<string, unknown> | undefined) => {
	const output = readingOutput(frame);
	const confidence = finiteScore(output.confidence);
	const surprise = Math.max(0, outputMetric(output, "surprise"));
	const strength = Math.max(0, outputMetric(output, "strength"));
	const status = kernelStatus(frame);

	return {
		output,
		confidence,
		surprise,
		strength,
		status,
		stamp: kernelStamp(frame, output),
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
): number[] => {
	const values = kernelHistory(frame).flatMap((sample) =>
		readingOutput(sample).confidence === undefined
			? []
			: [finiteScore(readingOutput(sample).confidence)],
	);

	return values;
};

export const kernelHistoryCount = (
	frame: Record<string, unknown> | undefined,
): number => kernelHistory(frame).length;

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
	let measured = 0;

	for (const origin of sources) {
		const frame = kernelFrameForSource(readings, origin, focusSymbol);
		const { status } = kernelReadout(frame);

		if (status === "measured") {
			measured += 1;
		}
	}

	return {
		measured,
		total: sources.length,
		label: `${measured}/${sources.length} measured`,
	};
};

type DecisionInput =
	| Record<string, unknown>
	| Array<Record<string, unknown>>
	| null;

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

const decisionWithFrameMeta = (
	decision: Record<string, unknown>,
	frame: Record<string, unknown>,
): Record<string, unknown> => {
	const next = { ...decision };

	for (const key of [
		"tick",
		"seq",
		"observed_at",
		"timestamp",
		"timestamp_unix_nano",
	]) {
		if (next[key] === undefined && frame[key] !== undefined) {
			next[key] = frame[key];
		}
	}

	return next;
};

const decisionList = (frame: DecisionInput): Array<Record<string, unknown>> => {
	if (Array.isArray(frame)) {
		return frame.flatMap((item) => decisionList(item));
	}

	if (Array.isArray(frame?.decisions)) {
		return (frame.decisions as Array<Record<string, unknown>>).map((decision) =>
			decisionWithFrameMeta(decision, frame),
		);
	}

	if (
		frame?.role === "decision" ||
		(typeof frame?.symbol === "string" && frame?.verdict !== undefined)
	) {
		return [frame];
	}

	return [];
};

const decisionRecency = (decision: Record<string, unknown>): number => {
	const tick = numericValue(decision.tick);
	const seq = numericValue(decision.seq);
	const observedAt = observedAtMilliseconds(decision.observed_at);
	const timestamp = observedAtMilliseconds(decision.timestamp);
	const timestampUnixNano = numericValue(decision.timestamp_unix_nano);

	return Math.max(
		tick,
		seq,
		observedAt ?? 0,
		timestamp ?? 0,
		timestampUnixNano,
	);
};

const decisionScore = (
	decision: Record<string, unknown>,
): { value: number; missing: boolean } => {
	const nested = (decision.decision ?? {}) as Record<string, unknown>;
	const raw = decision.score ?? nested.score;
	const value = finiteScore(raw);

	return {
		value,
		missing: raw === undefined || raw === null || !Number.isFinite(Number(raw)),
	};
};

const decisionVerdict = (
	decision: Record<string, unknown>,
): TerminalDecisionRow["verdict"] => {
	const rawVerdict = String(decision.verdict ?? "").toLowerCase();

	if (
		decision.allowed === false ||
		rawVerdict === "blocked" ||
		rawVerdict === "deny"
	) {
		return "blocked";
	}

	if (rawVerdict === "allow") {
		return "allow";
	}

	if (rawVerdict === "below" || rawVerdict === "in-play") {
		return "in-play";
	}

	return "blocked";
};

const decisionReason = (
	decision: Record<string, unknown>,
	verdict: TerminalDecisionRow["verdict"],
): string => {
	if (decision.verdict === undefined) {
		return "missing_backend_verdict";
	}

	return String(decision.why ?? decision.reason ?? verdict);
};

export const decisionRowsFromFrame = (
	frame: DecisionInput,
): TerminalDecisionRow[] => {
	const list = decisionList(frame).filter(
		(decision) => typeof decision.symbol === "string" && decision.symbol !== "",
	);
	const scores = list.map((decision) => decisionScore(decision));

	return list.map((decision, index) => {
		const score = scores[index] ?? { value: 0, missing: true };
		const scoreValue = score.value;
		const rawVerdict = String(decision.verdict ?? "").toLowerCase();
		const verdict = decisionVerdict(decision);
		const source = String(decision.source ?? decision.type ?? "decision");
		const symbol = String(decision.symbol);
		const side = String(decision.side ?? "");
		const type = String(decision.type ?? "");
		const id = String(decision.action_id ?? decision.decision_id ?? "");
		const fraction = numericValue(decision.fraction);
		const tick = numericValue(decision.tick);
		const seq = numericValue(decision.seq);
		const recency = decisionRecency(decision);
		const derivedKey = [
			symbol,
			side,
			type,
			rawVerdict,
			fixed(scoreValue),
			tick > 0 ? tick : "",
			seq > 0 ? seq : "",
			String(
				decision.observed_at ??
					decision.timestamp ??
					decision.timestamp_unix_nano ??
					"",
			),
			index,
		]
			.filter(Boolean)
			.join(":");

		return {
			key: id || derivedKey,
			symbol,
			source,
			scoreText: fixed(scoreValue),
			scoreValue,
			scoreMissing: score.missing,
			verdict,
			why: whyLabel(decisionReason(decision, verdict)),
			signals:
				source === "decision" ? [] : [{ source, confidence: scoreValue }],
			edgeText: "",
			edgePositive: verdict === "allow",
			fraction,
			tick,
			seq,
			recency,
		};
	});
};

export const dashboardDecisionRows = (
	_readings: Record<string, Record<string, Record<string, unknown>>>,
	_focusSymbol: string,
	_walkEvaluations: Record<string, WalkTrace>,
	decisionFrame: DecisionInput,
) => {
	const rows = decisionRowsFromFrame(decisionFrame);

	return { rows, line: 0 };
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
	const { output, confidence, surprise, status } = kernelReadout(frame);
	const copy = kernelCopy(origin, String(output.category ?? origin));
	const confidenceText = `${Math.floor(confidence * 100)}%`;
	const statusMeta = kernelStatusMeta(status);
	const sparkValues = kernelHistoryValues(frame);
	const spark = kernelSparkPaths(sparkValues, status);
	const surpColor = status === "ambiguous" ? "var(--acc)" : "var(--f4)";

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
									width: `${confidence * 100}%`,
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

	if (sources.length === 0) {
		return (
			<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
				waiting for backend measurement frames
			</div>
		);
	}

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

const backendEmptyReason = (
	funnelFrame: Record<string, unknown> | null,
): string => {
	const reason =
		typeof funnelFrame?.first_blocker === "string"
			? funnelFrame.first_blocker.trim()
			: "";

	return reason === "" ? "" : whyLabel(reason);
};

export const DecisionLineMeta = () => {
	const decisionFrame = useSelector(decisionsStore, (state) => state.frame);
	const decisionFrames = useSelector(decisionsStore, (state) => state.frames);
	const funnelFrame = useSelector(decisionFunnelStore, (state) => state.frame);
	const decisionInput =
		decisionFrames.length > 0 ? decisionFrames : decisionFrame;
	const rows = decisionRowsFromFrame(decisionInput);
	const latest = Array.isArray(decisionInput)
		? (decisionInput.at(-1) ?? null)
		: decisionInput;
	const tick = Number(latest?.tick ?? funnelFrame?.tick ?? 0);

	return (
		<>
			{rows.length > 0 || funnelFrame !== null
				? `batch #${tick > 0 ? tick : "—"}`
				: "batch —"}
		</>
	);
};

export const DecisionRows = () => {
	const readings = useSelector(measurementsStore, (state) => state);
	const focusSymbol = useSelector(terminalStore, (state) => state.focusSymbol);
	const decisionFrame = useSelector(decisionsStore, (state) => state.frame);
	const decisionFrames = useSelector(decisionsStore, (state) => state.frames);
	const funnelFrame = useSelector(decisionFunnelStore, (state) => state.frame);
	const { rows } = dashboardDecisionRows(
		readings,
		focusSymbol,
		{},
		decisionFrames.length > 0 ? decisionFrames : decisionFrame,
	);
	const emptyReason = backendEmptyReason(funnelFrame);

	return (
		<div className="min-h-0 flex-1 overflow-auto">
			<table className="w-full border-collapse text-[11.5px]">
				<thead>
					<tr className="sticky top-0 bg-(--surface) text-[9.5px] text-(--f4) uppercase tracking-[0.06em]">
						<th className="px-3 py-1.5 text-left font-medium">Symbol</th>
						<th className="px-1.5 py-1.5 text-right font-medium">Score</th>
						<th className="px-1.5 py-1.5 text-right font-medium">Reason</th>
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
								{emptyReason === ""
									? "waiting for backend decision frames"
									: emptyReason}
							</td>
						</tr>
					) : (
						rows.map((decision) => {
							const meta = verdictMeta(decision.verdict);
							return (
								<tr key={decision.key} className="hover:bg-(--raised)">
									<td
										data-symbol={decision.symbol}
										className="cursor-pointer px-3 py-1.5 font-mono text-[11px] font-semibold text-(--f1)"
									>
										{decision.symbol}
									</td>
									<td className="px-1.5 py-1.5 text-right font-mono text-(--f2)">
										{decision.scoreText}
									</td>
									<td className="max-w-[104px] truncate px-1.5 py-1.5 text-right font-mono text-[10px] text-(--f4)">
										{decision.why}
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
	symbol: string;
};

const auditTimestamp = (frame: Record<string, unknown>): string => {
	const stamp = observedAtMilliseconds(frame.timestamp ?? frame.observed_at);

	if (stamp !== undefined) {
		return new Date(stamp).toLocaleTimeString("en-US", { hour12: false });
	}

	return typeof frame.seq === "number" || typeof frame.seq === "string"
		? `#${String(frame.seq)}`
		: "";
};

export const diagnosticRowsFromFrame = (
	frames: Array<Record<string, unknown>>,
): AuditItem[] =>
	frames.flatMap((frame) => {
		const symbol = String(frame.symbol ?? frame.scope ?? "broker");
		const reason = String(frame.reason ?? frame.reject_reason ?? "");
		const severity = String(frame.severity ?? "info");

		if (reason === "") {
			return [];
		}

		return [
			{
				key: `diagnostic:${String(frame.timestamp ?? frame.seq ?? "")}:${symbol}:${reason}`,
				reason: whyLabel(reason),
				meta: ["diagnostic", severity, symbol].filter(Boolean).join(" · "),
				time: auditTimestamp(frame),
				symbol,
			},
		];
	});

export const auditRowsFromDecisionFrame = (
	frame: DecisionInput,
): AuditItem[] => {
	const latest = Array.isArray(frame) ? (frame.at(-1) ?? null) : frame;
	const observedAt = observedAtMilliseconds(latest?.observed_at);
	const clock =
		observedAt === undefined
			? ""
			: new Date(observedAt).toLocaleTimeString("en-US", { hour12: false });
	const seq =
		typeof latest?.seq === "number" || typeof latest?.seq === "string"
			? `#${String(latest.seq)}`
			: "";
	const time = [seq, clock].filter(Boolean).join(" · ");

	return decisionList(frame).flatMap((decision) => {
		const symbol = String(decision.symbol ?? "");

		if (symbol === "") {
			return [];
		}

		const score = decisionScore(decision).value;
		const verdict = decisionVerdict(decision);
		const reason =
			verdict === "allow"
				? `candidate scored ${fixed(score)}`
				: whyLabel(String(decision.why ?? decision.reason ?? String(verdict)));
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
				key: `decision:${symbol}:${verdict}:${fixed(score)}:${time}:${meta}`,
				reason,
				meta,
				time,
				symbol,
			},
		];
	});
};

const currencySymbolFor = (quoteCurrency: string): string =>
	quoteCurrency === "EUR"
		? "€"
		: quoteCurrency === "USD"
			? "$"
			: `${quoteCurrency} `;

const signedCurrency = (
	value: number,
	currencySymbol: string,
	decimals: number,
): string => {
	const sign = value >= 0 ? "+" : "-";

	return `${sign}${currencySymbol}${Math.abs(value).toFixed(decimals)}`;
};

const quoteCurrencyFromPositions = (
	positionsFrame: Record<string, unknown> | null,
	positions: Array<Record<string, unknown>>,
): string | null => {
	const quote =
		typeof positionsFrame?.quote === "string"
			? positionsFrame.quote
			: typeof positionsFrame?.quote_currency === "string"
				? positionsFrame.quote_currency
				: typeof positionsFrame?.quoteCurrency === "string"
					? positionsFrame.quoteCurrency
					: typeof positions[0]?.quote === "string"
						? positions[0].quote
						: "";
	const normalized = quote.trim().toUpperCase();

	return normalized === "" ? null : normalized;
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
	const quoteCurrency = quoteCurrencyFromPositions(positionsFrame, positions);
	const currencySymbol =
		quoteCurrency === null ? "" : currencySymbolFor(quoteCurrency);
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
			plText:
				quoteCurrency === null
					? "P/L quote unavailable"
					: `P/L ${signedCurrency(pnl, currencySymbol, 4)}`,
			pnl,
			pnlPositive,
		};
	});

	const netPnl = rows.reduce((sum, row) => sum + row.pnl, 0);

	return {
		currencySymbol,
		netPnl,
		netPositive: netPnl >= 0,
		netText:
			quoteCurrency === null
				? "net quote unavailable"
				: `net ${signedCurrency(netPnl, currencySymbol, 2)}`,
		quoteCurrency: quoteCurrency ?? "quote unavailable",
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
						data-symbol={row.symbol}
						className="rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-1.5 font-mono text-xs hover:bg-(--raised)"
					>
						<div className="flex items-center justify-between">
							<span className="cursor-pointer font-semibold text-(--f1)">
								{row.symbol}
							</span>
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
	const decisionFrames = useSelector(decisionsStore, (state) => state.frames);
	const diagnostics = useSelector(diagnosticsStore, (state) => state.history);
	const decisionAudit = auditRowsFromDecisionFrame(decisionFrames);
	const diagnosticAudit = diagnosticRowsFromFrame(diagnostics);

	if (
		history.length === 0 &&
		decisionAudit.length === 0 &&
		diagnosticAudit.length === 0
	) {
		return (
			<div className="min-h-0 flex-1 overflow-auto py-0.5">
				<div className="px-3 py-8 text-center font-mono text-(--f4) text-xs">
					no backend audit events
				</div>
			</div>
		);
	}

	return (
		<div className="min-h-0 flex-1 overflow-auto divide-y divide-(--line)">
			{diagnosticAudit.map((item) => (
				<div
					key={item.key}
					data-symbol={item.symbol}
					className="px-3 py-1.5 hover:bg-(--raised)"
				>
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
			{decisionAudit.map((item) => (
				<div
					key={item.key}
					data-symbol={item.symbol}
					className="px-3 py-1.5 hover:bg-(--raised)"
				>
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
					<div
						key={key}
						data-symbol={symbol}
						className="px-3 py-1.5 hover:bg-(--raised)"
					>
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
