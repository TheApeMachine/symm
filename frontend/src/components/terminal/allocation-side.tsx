import type { Balance } from "#/collections/types";
import type { CausalFrame } from "#/collections/types";
import type { Holding } from "#/collections/types";
import type { ManifoldFrame } from "#/collections/types";
import type { Order } from "#/collections/types";
import type { ResonanceFrame } from "#/collections/types";
import {
	causalConfidence,
	causalEntryBaseline,
	causalStrength,
} from "#/components/terminal/causal-view";
import { resonancePredict } from "#/components/terminal/decision-candidate";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";

type History<T> = { values: () => T[] };

type AllocationInput = {
	focusSymbol: string;
	symbols: string[];
	balances: Balance[];
	causal: Record<string, History<CausalFrame>>;
	manifold: Record<string, History<ManifoldFrame>>;
	holdings: Holding[];
	resonance: Record<string, History<ResonanceFrame>>;
};

type AllocationRow = {
	allocated: boolean;
	dotColor: string;
	inPlay: boolean;
	symbol: string;
	edge: number;
	edgeLeft: number;
	edgeWidth: number;
	notional: number;
	share: number;
	support: number;
	thesis: number;
	xPct: number;
};

export type AllocationSummary = {
	deployable: number;
	deployed: number;
	positionCount: number;
	mad: number;
	median: number;
	medianPct: number;
	quote: string;
	reserved: number;
	rows: AllocationRow[];
	threshold: number;
	thresholdPct: number;
};

const latest = <T,>(history?: History<T>): T | undefined =>
	history?.values().at(-1);

const finite = (value: unknown): number => {
	const number = typeof value === "number" ? value : Number(value);

	return Number.isFinite(number) ? number : 0;
};

const probability = (value: unknown): number =>
	Math.min(1, Math.max(0, finite(value)));

const median = (values: number[]): number => {
	if (values.length === 0) {
		return 0;
	}

	const order = [...values].sort((left, right) => left - right);

	return order[Math.floor(order.length / 2)] ?? 0;
};

const quoteAsset = (symbol: string): string => symbol.split("/")[1] ?? "";

const money = (value: number, quote: string): string =>
	`${value.toFixed(2)} ${quote}`;

const balanceAmount = (
	balances: Balance[],
	quote: string,
	field: "available" | "balance" | "reserved",
): number =>
	balances.reduce(
		(sum, balance) =>
			String(balance.asset).toUpperCase() === quote.toUpperCase()
				? sum + finite(balance[field])
				: sum,
		0,
	);

export const allocationSummary = ({
	focusSymbol,
	symbols,
	balances,
	causal,
	manifold,
	holdings,
	resonance,
}: AllocationInput): AllocationSummary => {
	const known = new Set([
		...Object.keys(causal),
		...Object.keys(manifold),
		...Object.keys(resonance),
	]);
	const orderedSymbols = [
		...symbols.filter((symbol) => known.has(symbol)),
		...[...known].filter((symbol) => !symbols.includes(symbol)).sort(),
	];
	const quote = quoteAsset(focusSymbol);
	const deployable = balanceAmount(balances, quote, "available");
	const reserved = balanceAmount(balances, quote, "reserved");
	const deployed = holdings
		.filter((holding) => quoteAsset(holding.symbol) === quote)
		.reduce((sum, holding) => sum + holding.qty * holding.mark, 0);
	const rows = orderedSymbols.map((symbol) => {
		const causalFrame = latest(causal[symbol]);
		const manifoldFrame = latest(manifold[symbol]);
		const resonanceFrame = latest(resonance[symbol]);
		const support = [causalFrame, manifoldFrame, resonanceFrame].filter(
			Boolean,
		).length;
		const thesis =
			Math.min(
				probability(resonancePredict(resonanceFrame) ?? 0),
				probability(causalConfidence(causalFrame)),
			) * Math.sqrt(Math.max(1, support));

		return {
			allocated: false,
			dotColor: "var(--f4)",
			edge: 0,
			edgeLeft: 0,
			edgeWidth: 0,
			inPlay:
				support >= 2 &&
				causalStrength(causalFrame) >= causalEntryBaseline(causalFrame),
			notional: 0,
			share: 0,
			support,
			symbol,
			thesis,
			xPct: 0,
		};
	});
	const scores = rows.map((row) => row.thesis);
	const med = median(scores);
	const dispersion =
		scores.length > 0
			? scores.reduce((sum, score) => sum + Math.abs(score - med), 0) /
				scores.length
			: 0;
	const threshold = med + dispersion;
	const sumPositive = scores.reduce(
		(sum, score) => sum + Math.max(0, score),
		0,
	);
	const lo = Math.min(...scores, threshold) * 0.92;
	const hi = Math.max(...scores, threshold) * 1.04;
	const span = hi - lo || 1;
	const pct = (value: number) =>
		Math.min(100, Math.max(0, ((value - lo) / span) * 100));
	const thresholdPct = pct(threshold);
	const medianPct = pct(med);

	for (const row of rows) {
		row.edge = row.thesis - threshold;
		row.share =
			row.edge > 0 && row.thesis + sumPositive > 0
				? row.edge / (row.thesis + sumPositive)
				: 0;
		row.allocated = row.edge > 0;
		row.notional = row.allocated ? deployable * row.share : 0;
		row.inPlay = row.inPlay || row.edge > 0;
		row.xPct = pct(row.thesis);
		row.edgeLeft = Math.min(thresholdPct, row.xPct);
		row.edgeWidth = row.edge > 0 ? Math.max(0, row.xPct - thresholdPct) : 0;
		row.dotColor = row.allocated
			? "var(--acc)"
			: row.inPlay
				? "var(--info)"
				: "var(--f4)";
	}

	return {
		deployable,
		deployed,
		mad: dispersion,
		median: med,
		medianPct,
		positionCount: holdings.length,
		quote,
		reserved,
		rows,
		threshold,
		thresholdPct,
	};
};

const visibleRows = (alloc: AllocationSummary) =>
	alloc.rows.filter((row) => row.allocated || row.inPlay);

export const AllocationMain = ({ alloc }: { alloc: AllocationSummary }) => {
	const rows = visibleRows(alloc);

	return (
		<div className="min-h-0 overflow-auto px-[18px] py-4">
			<div className="mb-3 flex items-center gap-3.5 font-mono text-[11px]">
				<span className="text-(--f3)">cross-section</span>
				<span className="text-(--f4)">
					median <span className="text-(--f2)">{alloc.median.toFixed(3)}</span>
				</span>
				<span className="text-(--f4)">
					mad <span className="text-(--f2)">{alloc.mad.toFixed(3)}</span>
				</span>
				<span className="text-(--f4)">
					entry{" "}
					<span className="text-(--acc)">{alloc.threshold.toFixed(3)}</span>
				</span>
				<span className="ml-auto text-(--f4)">live ladder frames</span>
			</div>

			<div className="flex items-center gap-[9px] border-(--line) border-b pb-[7px] font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.06em]">
				<span className="w-[58px] shrink-0">symbol</span>
				<span className="flex-1">thesis score {"->"} sqrt(confirmations)</span>
				<span className="w-[52px] shrink-0 text-right">edge</span>
				<span className="w-[42px] shrink-0 text-right">share</span>
				<span className="w-[74px] shrink-0 text-right">notional</span>
			</div>

			<div className="flex flex-col">
				{rows.length === 0 ? (
					<div className="py-24 text-center font-mono text-[11px] text-(--f4)">
						waiting for backend decision frames
					</div>
				) : null}

				{rows.map((row) => (
					<div
						key={row.symbol}
						data-symbol={row.symbol}
						className="flex items-center gap-[9px] border-(--line) border-b py-[7px]"
					>
						<span
							className="w-[58px] shrink-0 font-mono text-[11px] font-semibold"
							style={{ color: row.dotColor }}
						>
							{row.symbol.split("/")[0]}
						</span>
						<div className="relative h-[18px] flex-1">
							<div className="absolute top-2 right-0 left-0 h-px bg-(--line)" />
							<div
								className="absolute top-px bottom-px w-px bg-(--f4)"
								style={{ left: `${alloc.medianPct}%` }}
							/>
							<div
								className="absolute top-0 bottom-0 w-px bg-[color-mix(in_srgb,var(--acc)_70%,transparent)]"
								style={{ left: `${alloc.thresholdPct}%` }}
							/>
							<div
								className="absolute top-[7px] h-[3px] rounded-sm bg-(--acc)"
								style={{
									left: `${row.edgeLeft}%`,
									width: `${row.edgeWidth}%`,
								}}
							/>
							<div
								className="absolute top-1 h-[9px] w-[9px] rounded-full border border-(--sunken)"
								style={{
									left: `${row.xPct}%`,
									marginLeft: "-4.5px",
									background: row.dotColor,
								}}
							/>
						</div>
						<span
							className="w-[52px] shrink-0 text-right font-mono text-[10px]"
							style={{ color: row.edge > 0 ? "var(--up)" : "var(--f4)" }}
						>
							{row.edge >= 0 ? "+" : "-"}
							{Math.abs(row.edge).toFixed(3)}
						</span>
						<span className="w-[42px] shrink-0 text-right font-mono text-[10px] text-(--f2)">
							{(row.share * 100).toFixed(1)}%
						</span>
						<span
							className="w-[74px] shrink-0 text-right font-mono text-[10.5px]"
							style={{ color: row.allocated ? "var(--f1)" : "var(--f4)" }}
						>
							{row.allocated ? money(row.notional, alloc.quote) : "-"}
						</span>
					</div>
				))}
			</div>

			<div className="mt-[11px] flex items-center gap-4 font-mono text-[9px] text-(--f3)">
				{[
					["var(--acc)", "allocated"],
					["var(--info)", "in play · below edge"],
					["var(--f4)", "scanned"],
				].map(([color, label]) => (
					<span key={label} className="inline-flex items-center gap-[5px]">
						<span
							className="h-2 w-2 rounded-full"
							style={{ background: color }}
						/>
						{label}
					</span>
				))}
				<span className="inline-flex items-center gap-[5px]">
					<span className="h-px w-[10px] bg-(--acc)" /> entry line
				</span>
			</div>
		</div>
	);
};

export const AllocationSidePanel = ({
	alloc,
	orders,
}: {
	alloc: AllocationSummary;
	orders: Order[];
}) => {
	const deployedPercent =
		alloc.deployable > 0
			? Math.min(100, Math.round((alloc.deployed / alloc.deployable) * 100))
			: 0;
	const rows = alloc.rows.filter((row) => row.allocated);

	return (
		<div className="flex flex-col gap-3.5">
			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Capital deployment
					</span>
					<span className="font-mono text-[11px] text-(--acc)">
						{deployedPercent}%
					</span>
				</div>
				<div className="mt-1 mb-[11px] font-mono text-[9.5px] text-(--f4)">
					share of deployable free cash
				</div>
				<Meter
					layout="bar"
					percent={deployedPercent}
					variant="warning"
					size="lg"
				/>
				<div className="mt-[7px] flex justify-between font-mono text-[10px] text-(--f3)">
					<span>deployed {money(alloc.deployed, alloc.quote)}</span>
					<span>reserved {money(alloc.reserved, alloc.quote)}</span>
				</div>
				{orders.length > 0 ? (
					<div className="mt-3 border-(--line) border-t pt-2">
						<div className="mb-1 font-semibold text-[11px] text-(--f1)">
							Reserved orders
						</div>
						{orders.map((order) => (
							<div
								key={order.id}
								className="flex items-center justify-between gap-2 py-1 font-mono text-[9.5px]"
							>
								<span className="text-(--info)">
									{order.pair} {order.side} {order.type}
								</span>
								<span className="text-(--f2)">
									{money(finite(order.reserved_amount), order.reserved_asset)}
								</span>
							</div>
						))}
					</div>
				) : null}
			</Panel>

			<Panel>
				<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">
					Position sizing
				</div>
				<div className="mb-[11px] font-mono text-[9.5px] text-(--f4)">
					notional {alloc.quote} per allocated symbol
				</div>
				<div className="flex flex-col gap-[9px]">
					{rows.length === 0 ? (
						<div className="border-(--line) border-t pt-[11px] font-mono text-[9.5px] text-(--f4)">
							no symbols above entry edge
						</div>
					) : null}
					{rows.map((row) => (
						<div key={row.symbol} data-symbol={row.symbol}>
							<div className="mb-1 flex items-center justify-between">
								<span className="font-mono text-[11px] font-semibold text-(--f1)">
									{row.symbol.split("/")[0]}
								</span>
								<span className="font-mono text-[10.5px] text-(--acc)">
									{money(row.notional, alloc.quote)}
								</span>
							</div>
							<Meter
								layout="inline"
								percent={Math.round(row.share * 100)}
								value={`${(row.share * 100).toFixed(1)}%`}
								variant="warning"
								size="m"
								animated
							/>
						</div>
					))}
				</div>
			</Panel>
		</div>
	);
};
