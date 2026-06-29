import type { ReactNode } from "react";
import {
	allocationEntryStats,
	fixed,
} from "#/components/terminal/decision-format";
import { terminalDecisionsFromWalk } from "#/components/terminal/decisions-from-walk";
import { kernelsForFocus } from "#/components/terminal/kernels";
import type { TerminalModel } from "#/components/terminal/model";
import { decisionRowsFromFrame } from "#/components/terminal/rows";

export type AllocationCandidate = {
	key: string;
	symbol: string;
	scoreValue: number;
	edge: number;
	share: number;
	notional: number;
	allocated: boolean;
	inPlay: boolean;
	positionPercent: number;
	medianPercent: number;
	thresholdPercent: number;
	edgeLeftPercent: number;
	edgeWidthPercent: number;
};

export type AllocationResult = {
	threshold: number;
	median: number;
	mad: number;
	candidates: AllocationCandidate[];
	deployed: number;
	deployedPercent: number;
	freeCash: number;
	reserved: number;
	quote: string;
	allocatedCount: number;
};

const parseCurrency = (value: string): number => {
	const numeric = Number(value.replace(/[^0-9.-]/g, ""));

	return Number.isFinite(numeric) ? numeric : 0;
};

const clamp = (value: number, min: number, max: number): number =>
	Math.min(max, Math.max(min, value));

const currency = (value: number, quote: string): string =>
	`${value.toLocaleString("en-US", {
		minimumFractionDigits: 2,
		maximumFractionDigits: 2,
	})} ${quote}`;

const quoteFromBalances = (
	balances: Record<string, unknown> | null,
): {
	quote: string;
	available: number;
	reserved: number;
} => {
	const assets =
		(balances?.asset as Array<Record<string, unknown>> | undefined) ?? [];
	const quoteAsset =
		(assets.find((asset) => asset.asset === "USD" || asset.asset === "EUR")
			?.asset as string | undefined) ||
		(assets[0]?.asset as string | undefined) ||
		"USD";
	const quoteRow = assets.find((asset) => asset.asset === quoteAsset);

	return {
		quote: quoteAsset,
		available: Number(quoteRow?.balance ?? 0),
		reserved: Number(balances?.reserved ?? 0),
	};
};

/*
allocationRows mirrors the tmp allocation x-ray:
edge = thesis - entry, share = edge / (thesis + sum positive thesis), and only
`allow` rows deploy notional. In-play rows stay visible but below deployable edge.
*/
export const allocationRows = (
	model: TerminalModel,
	quote = "USD",
): AllocationResult => {
	const decisions = model.decisions ?? [];
	const scores = decisions.map((decision) => decision.scoreValue);
	const stats = allocationEntryStats(scores);
	const freeCash = parseCurrency(model.wallet?.available ?? "0");
	const reserved = parseCurrency(model.wallet?.reserved ?? "0");
	const positiveThesis = decisions.reduce(
		(sum, decision) => sum + Math.max(0, decision.scoreValue),
		0,
	);
	const values = [...scores, stats.threshold];
	const low = values.length > 0 ? Math.min(...values) * 0.92 : 0;
	const high = values.length > 0 ? Math.max(...values) * 1.04 : 1;
	const span = high - low || 1;
	const percentOf = (value: number): number =>
		clamp(((value - low) / span) * 100, 0, 100);
	const medianPercent = percentOf(stats.median);
	const thresholdPercent = percentOf(stats.threshold);

	const candidates: AllocationCandidate[] = decisions
		.map((decision) => {
			const edge = decision.scoreValue - stats.threshold;
			const denominator = decision.scoreValue + positiveThesis || 1;
			const allocated = edge > 0 && decision.verdict === "allow";
			const share = edge > 0 ? edge / denominator : 0;
			const notional = share * freeCash;
			const positionPercent = percentOf(decision.scoreValue);

			return {
				key: decision.key,
				symbol: decision.symbol,
				scoreValue: decision.scoreValue,
				edge,
				share,
				notional,
				allocated,
				inPlay: decision.verdict === "in-play",
				positionPercent,
				medianPercent,
				thresholdPercent,
				edgeLeftPercent: Math.min(thresholdPercent, positionPercent),
				edgeWidthPercent: allocated
					? Math.max(0, positionPercent - thresholdPercent)
					: 0,
			};
		})
		.sort((left, right) => right.scoreValue - left.scoreValue);

	const deployed = candidates.reduce(
		(sum, candidate) => sum + (candidate.allocated ? candidate.notional : 0),
		0,
	);

	return {
		threshold: stats.threshold,
		median: stats.median,
		mad: stats.mad,
		candidates,
		deployed,
		deployedPercent:
			freeCash > 0 ? clamp((deployed / freeCash) * 100, 0, 100) : 0,
		freeCash,
		reserved,
		quote,
		allocatedCount: candidates.filter((candidate) => candidate.allocated)
			.length,
	};
};

export const allocationModelFromStores = (
	balances: Record<string, unknown> | null,
	evaluations: Parameters<typeof terminalDecisionsFromWalk>[0],
	readings: Parameters<typeof kernelsForFocus>[0],
	decisionFrame:
		| Record<string, unknown>
		| Array<Record<string, unknown>>
		| null = null,
): AllocationResult => {
	const funds = quoteFromBalances(balances);
	const kernels = kernelsForFocus(readings);
	const decisions =
		decisionFrame === null ||
		(Array.isArray(decisionFrame) && decisionFrame.length === 0)
			? terminalDecisionsFromWalk(evaluations, kernels)
			: decisionRowsFromFrame(decisionFrame);

	return allocationRows(
		{
			wallet: {
				available: `${funds.available}`,
				reserved: `${funds.reserved}`,
			},
			decisions,
		},
		funds.quote,
	);
};

const dotColor = (candidate: AllocationCandidate): string => {
	if (candidate.allocated) {
		return "var(--acc)";
	}

	if (candidate.inPlay) {
		return "var(--info)";
	}

	return "var(--f4)";
};

const edgeColor = (candidate: AllocationCandidate): string =>
	candidate.edge > 0 ? "var(--up)" : "var(--f4)";

const AllocationLegend = () => (
	<div className="mt-[11px] flex items-center gap-4 font-mono text-[9px] text-(--f3)">
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--acc)" />
			allocated
		</span>
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--info)" />
			in play · below edge
		</span>
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--f4)" />
			scanned
		</span>
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-px w-2.5 bg-(--acc)" />
			entry line
		</span>
	</div>
);

export const AllocationMain = ({ alloc }: { alloc: AllocationResult }) => {
	if (alloc.candidates.length === 0) {
		return (
			<div className="flex min-h-0 flex-1 items-center justify-center font-mono text-[11px] text-(--f4)">
				waiting for allocation frames
			</div>
		);
	}

	return (
		<div className="min-h-0 overflow-auto px-[18px] py-4">
			<div className="mb-3 flex items-center gap-3.5 font-mono text-[11px]">
				<span className="text-(--f3)">cross-section</span>
				<span className="text-(--f4)">
					median <span className="text-(--f2)">{fixed(alloc.median)}</span>
				</span>
				<span className="text-(--f4)">
					mad <span className="text-(--f2)">{fixed(alloc.mad)}</span>
				</span>
				<span className="text-(--f4)">
					entry <span className="text-(--acc)">{fixed(alloc.threshold)}</span>
				</span>
				<span className="ml-auto text-(--f4)">min cost €0.45 · edge × 2.0</span>
			</div>

			<div className="flex items-center gap-[9px] border-(--line) border-b pb-[7px] font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.06em]">
				<span className="w-[58px] shrink-0">symbol</span>
				<span className="flex-1">
					thesis score → (√ confirmations) · amber bar = edge past entry
				</span>
				<span className="w-[52px] shrink-0 text-right">edge</span>
				<span className="w-[42px] shrink-0 text-right">share</span>
				<span className="w-[66px] shrink-0 text-right">notional</span>
			</div>

			<div className="flex flex-col">
				{alloc.candidates.map((candidate) => (
					<div
						key={candidate.key}
						className="flex items-center gap-[9px] border-(--line) border-b py-[7px]"
					>
						<span
							className="w-[58px] shrink-0 font-mono text-[11px] font-semibold"
							style={{ color: dotColor(candidate) }}
						>
							{candidate.symbol}
						</span>
						<div className="relative h-[18px] flex-1">
							<div className="absolute top-2 right-0 left-0 h-px bg-(--line)" />
							<div
								className="absolute top-px bottom-px w-px bg-(--f4)"
								style={{ left: `${candidate.medianPercent}%` }}
							/>
							<div
								className="absolute top-0 bottom-0 w-px bg-[color-mix(in_srgb,var(--acc)_70%,transparent)]"
								style={{ left: `${candidate.thresholdPercent}%` }}
							/>
							<div
								className="absolute top-[7px] h-[3px] rounded-sm bg-(--acc)"
								style={{
									left: `${candidate.edgeLeftPercent}%`,
									width: `${candidate.edgeWidthPercent}%`,
								}}
							/>
							<div
								className="absolute top-1 h-[9px] w-[9px] rounded-full border border-(--sunken)"
								style={{
									left: `${candidate.positionPercent}%`,
									marginLeft: "-4.5px",
									background: dotColor(candidate),
								}}
							/>
						</div>
						<span
							className="w-[52px] shrink-0 text-right font-mono text-[10px]"
							style={{ color: edgeColor(candidate) }}
						>
							{candidate.edge >= 0 ? "+" : "−"}
							{fixed(Math.abs(candidate.edge))}
						</span>
						<span className="w-[42px] shrink-0 text-right font-mono text-[10px] text-(--f2)">
							{(candidate.share * 100).toFixed(1)}%
						</span>
						<span
							className="w-[66px] shrink-0 text-right font-mono text-[10.5px]"
							style={{
								color: candidate.allocated ? "var(--f1)" : "var(--f4)",
							}}
						>
							{candidate.allocated
								? currency(candidate.notional, alloc.quote)
								: "—"}
						</span>
					</div>
				))}
			</div>

			<AllocationLegend />
		</div>
	);
};

const Panel = ({ children }: { children: ReactNode }) => (
	<div className="rounded-[4px] border border-(--line) bg-(--sunken) p-3">
		{children}
	</div>
);

const Bar = ({ percent }: { percent: number }) => (
	<div className="h-2 overflow-hidden rounded bg-(--line)">
		<div className="h-full bg-(--acc)" style={{ width: `${percent}%` }} />
	</div>
);

export const AllocationSidePanel = ({ alloc }: { alloc: AllocationResult }) => {
	const allocated = alloc.candidates
		.filter((candidate) => candidate.allocated)
		.sort((left, right) => right.notional - left.notional)
		.slice(0, 8);

	return (
		<div className="flex flex-col gap-3.5">
			<Panel>
				<div className="flex items-center justify-between">
					<span className="font-semibold text-[12px] text-(--f1)">
						Capital deployment
					</span>
					<span className="font-mono text-[11px] text-(--acc)">
						{Math.round(alloc.deployedPercent)}%
					</span>
				</div>
				<div className="mt-1 mb-[11px] font-mono text-[9.5px] text-(--f4)">
					share of deployable free cash
				</div>
				<Bar percent={alloc.deployedPercent} />
				<div className="mt-[7px] flex justify-between font-mono text-[10px] text-(--f3)">
					<span>deployed {currency(alloc.deployed, alloc.quote)}</span>
					<span>reserved {currency(alloc.reserved, alloc.quote)}</span>
				</div>
			</Panel>

			<Panel>
				<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">
					Position sizing
				</div>
				<div className="mb-[11px] font-mono text-[9.5px] text-(--f4)">
					notional € per allocated symbol
				</div>
				<div className="flex flex-col gap-[9px]">
					{allocated.map((candidate) => (
						<div key={candidate.key}>
							<div className="mb-1 flex items-center justify-between">
								<span className="font-mono text-[11px] font-semibold text-(--f1)">
									{candidate.symbol}
								</span>
								<span className="font-mono text-[10.5px] text-(--acc)">
									{currency(candidate.notional, alloc.quote)}
								</span>
							</div>
							<div className="flex items-center gap-2">
								<div className="h-1.5 flex-1 overflow-hidden rounded bg-(--line)">
									<div
										className="h-full bg-(--acc) transition-[width] duration-500"
										style={{
											width: `${clamp(candidate.share * 100, 0, 100)}%`,
										}}
									/>
								</div>
								<span className="w-[38px] text-right font-mono text-[9.5px] text-(--f3)">
									{(candidate.share * 100).toFixed(1)}%
								</span>
							</div>
						</div>
					))}
				</div>
				<div className="mt-3 border-(--line) border-t pt-[11px] font-mono text-[9.5px] text-(--f4) leading-[1.6]">
					<div>· MinCostEUR €0.45 floor enforced per fill</div>
					<div>· EntryEdgeMultiple 2.0× vs round-trip friction</div>
					<div>· unallocated edge → held as free cash</div>
				</div>
			</Panel>
		</div>
	);
};
