import { useSelector } from "@tanstack/react-store";
import { measurementsStore } from "#/collections/measurements";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelStatusMeta,
	kernelSparkPaths,
	type SignalHealthStatus,
} from "#/components/terminal/kernel-meta";
import { balancesStore } from "#/collections/balances";
import { executionsStore } from "#/collections/executions";
import { decisionsStore } from "#/collections/decisions";

const getHealthStatus = (
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
	const { inspectSource, selectSource } = terminalStore.actions;
	const sources = origins ?? Object.keys(readings);

	return (
		<div className="min-h-0 overflow-auto">
			{sources.map((origin) => {
				const frame = readings[origin]?.[focusSymbol] as
					| Record<string, unknown>
					| undefined;
				const output = (frame?.output ?? {}) as Record<string, unknown>;
				const confidence =
					((frame?.confidence as number) ?? (output.confidence as number)) ?? 0;
				const surprise =
					((frame?.surprise as number) ?? (output.surprise as number)) ?? 0;
				const copy = kernelCopy(origin, String(output.category ?? origin));
				const inspecting = inspectorSource === origin;
				const selected = selectedSource === origin;
				const confidenceText = `${Math.round(confidence * 100)}%`;

				const healthStatus = getHealthStatus(origin, confidence, surprise);
				const statusMeta = kernelStatusMeta(healthStatus);
				const spark = kernelSparkPaths([confidence], surprise);
				const surpColor = spark.firing ? "var(--acc)" : "var(--f4)";

				return (
					<button
						type="button"
						key={origin}
						onClick={() =>
							compact ? selectSource(origin) : inspectSource(origin)
						}
						className="block w-full cursor-pointer border-(--line) border-b border-l-2 px-3 py-2.5 text-left font-[inherit] hover:bg-(--raised)"
						style={{
							borderLeftColor:
								inspecting || selected ? "var(--acc)" : "transparent",
							background:
								inspecting || selected ? "var(--raised)" : "transparent",
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
			})}
		</div>
	);
};

export const DecisionRows = () => {
	const frame = useSelector(decisionsStore, (state) => state.frame);
	const list = (frame?.decisions as Array<Record<string, unknown>>) ?? [];

	if (list.length === 0) {
		return (
			<div className="min-h-0 flex-1 overflow-auto">
				<div className="px-3 py-8 text-center font-mono text-(--f4) text-xs">
					Waiting for playbook decisions...
				</div>
			</div>
		);
	}

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
					{list.map((d, index) => {
						const symbol = String(d.symbol || "");
						const combined = Number(d.confidence ?? d.combined ?? 0).toFixed(3);
						const edge = d.edge !== undefined ? String(d.edge) : "—";
						const verdict = String(d.verdict || "below").toLowerCase();

						let verdictLabel = "BELOW";
						let verdictBg = "var(--line)";
						let verdictFg = "var(--f3)";

						if (verdict === "allow") {
							verdictLabel = "ALLOW";
							verdictBg = "color-mix(in srgb, var(--up) 16%, transparent)";
							verdictFg = "var(--up)";
						} else if (verdict === "deny") {
							verdictLabel = "DENY";
							verdictBg = "color-mix(in srgb, var(--down) 16%, transparent)";
							verdictFg = "var(--down)";
						}

						return (
							<tr key={index} className="hover:bg-(--raised)">
								<td className="px-3 py-1.5 font-mono text-[11px] font-semibold text-(--f1)">
									{symbol}
								</td>
								<td className="px-1.5 py-1.5 text-right font-mono text-(--f2)">
									{combined}
								</td>
								<td className="px-1.5 py-1.5 text-right font-mono text-[10px] text-(--f3)">
									{edge}
								</td>
								<td className="px-3 py-1.5">
									<span
										className="rounded-[2px] px-1.5 py-0.5 text-[9px] font-semibold tracking-wider"
										style={{ backgroundColor: verdictBg, color: verdictFg }}
									>
										{verdictLabel}
									</span>
								</td>
							</tr>
						);
					})}
				</tbody>
			</table>
		</div>
	);
};

export const PositionRows = () => {
	const balances = useSelector(balancesStore, (state) => state.frame);
	const history = useSelector(executionsStore, (state) => state.history);
	const readings = useSelector(measurementsStore, (state) => state);
	const balancesList = (balances?.asset as Array<Record<string, unknown>>) ?? [];
	const usdBalance =
		balancesList.find((b) => b.asset === "USD" || b.asset === "EUR") ??
		balancesList[0];
	const quoteCurrency = (usdBalance?.asset as string) || "EUR";
	const positions = balancesList.filter(
		(b) => b.asset !== quoteCurrency && Number(b.balance) > 0.00001,
	);

	if (positions.length === 0) {
		return (
			<div className="min-h-0 flex-1 overflow-auto p-1.5">
				<div className="px-2 py-8 text-center font-mono text-(--f4) text-xs">
					No open positions
				</div>
			</div>
		);
	}

	const currencySymbol = quoteCurrency === "EUR" ? "€" : "$";

	return (
		<div className="min-h-0 flex-1 overflow-auto p-1.5 space-y-1.5">
			{positions.map((p) => {
				const asset = p.asset as string;
				const balance = Number(p.balance || 0);
				const symbol = `${asset}/${quoteCurrency}`;

				let mark = 0;
				for (const origin of Object.keys(readings)) {
					const frame = readings[origin]?.[symbol] as
						| Record<string, unknown>
						| undefined;
					if (frame?.price !== undefined) {
						mark = Number(frame.price);
						break;
					}
					const output = frame?.output as Record<string, unknown> | undefined;
					if (output?.last !== undefined) {
						mark = Number(output.last);
						break;
					}
				}

				const lastBuy = history.find(
					(h) =>
						String(h.symbol).toUpperCase() === symbol.toUpperCase() &&
						(String(h.side).toLowerCase() === "buy" ||
							String(h.side).toLowerCase() === "enter"),
				);
				const entry = lastBuy
					? Number(
							lastBuy.last_price || lastBuy.avg_price || lastBuy.price || mark,
						)
					: mark;

				const pnl = mark > 0 && entry > 0 ? (mark - entry) * balance : 0;
				const pct = entry > 0 ? ((mark - entry) / entry) * 100 : 0;

				const plFg = pnl >= 0 ? "var(--up)" : "var(--down)";
				const plSign = pnl >= 0 ? "+" : "−";
				const pctSign = pct >= 0 ? "+" : "−";

				const formattedPl = `P/L ${plSign}${currencySymbol}${Math.abs(pnl).toFixed(2)}`;
				const formattedPct = `${pctSign}${Math.abs(pct).toFixed(2)}%`;
				const detail = `entry ${entry > 0 ? entry.toFixed(2) : "—"} · mark ${mark > 0 ? mark.toFixed(2) : "—"}`;

				return (
					<div
						key={asset}
						className="rounded-[3px] border border-(--line) bg-(--sunken) px-2.5 py-1.5 font-mono text-xs hover:bg-(--raised)"
					>
						<div className="flex items-center justify-between">
							<span className="font-semibold text-(--f1)">{symbol}</span>
							<span className="font-semibold" style={{ color: plFg }}>
								{formattedPl}
							</span>
						</div>
						<div className="mt-1 flex items-center justify-between text-[9.5px] text-(--f4)">
							<span>{detail}</span>
							<span style={{ color: plFg }}>{formattedPct}</span>
						</div>
					</div>
				);
			})}
		</div>
	);
};

export const AuditRows = () => {
	const history = useSelector(executionsStore, (state) => state.history);

	if (history.length === 0) {
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
			{history.map((item, index) => {
				const symbol = String(item.symbol || "unknown");
				const side = String(item.side || "trade").toUpperCase();
				const qty = Number(item.order_qty || item.last_qty || 0).toFixed(4);
				const price = Number(
					item.last_price || item.avg_price || item.price || 0,
				).toFixed(2);
				const execType = String(item.exec_type || "fill");
				const status = String(item.order_status || "settled");

				const reason = `${side === "BUY" ? "Position Opened" : "Position Settled"}: ${symbol}`;
				const time = item.observed_at
					? new Date(Number(item.observed_at)).toLocaleTimeString("en-US", {
							hour12: false,
						})
					: new Date().toLocaleTimeString("en-US", { hour12: false });
				const meta = `qty ${qty} · px ${price} · ${execType} (${status})`;

				return (
					<div key={index} className="px-3 py-1.5 hover:bg-(--raised)">
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
