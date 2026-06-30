import type { ReactNode } from "react";
import { fixed } from "#/components/terminal/decision-format";
import type { TerminalModel } from "#/components/terminal/model";
import { decisionRowsFromFrame } from "#/components/terminal/rows";

export type AllocationCandidate = {
	key: string;
	symbol: string;
	scoreValue: number;
	verdict: "allow" | "in-play" | "blocked";
	why: string;
	fraction: number;
	positionPercent: number;
	tick?: number;
};

export type AllocationResult = {
	candidates: AllocationCandidate[];
	deployed: number;
	deployedPercent: number;
	freeCash: number;
	reserved: number;
	quote: string;
	admittedCount: number;
	positionCount: number;
	tick?: number;
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
		(balances?.data as Array<Record<string, unknown>> | undefined) ?? [];
	const configuredQuote =
		typeof balances?.quote === "string"
			? balances.quote
			: typeof balances?.quote_currency === "string"
				? balances.quote_currency
				: typeof balances?.quoteCurrency === "string"
					? balances.quoteCurrency
					: "";
	const markedQuoteRow = assets.find(
		(asset) =>
			asset.quote === true ||
			asset.is_quote === true ||
			asset.role === "quote" ||
			asset.type === "quote",
	);
	const quoteAsset =
		configuredQuote || (markedQuoteRow?.asset as string | undefined) || "";
	const quoteRow =
		quoteAsset === ""
			? undefined
			: assets.find((asset) => asset.asset === quoteAsset);

	return {
		quote: quoteAsset || "quote unavailable",
		available: Number(quoteRow?.balance ?? 0),
		reserved: Number(balances?.reserved ?? 0),
	};
};

const positionExposure = (
	positionsFrame: Record<string, unknown> | null = null,
): {
	deployed: number;
	positionCount: number;
} => {
	const positions =
		(positionsFrame?.positions as Array<Record<string, unknown>> | undefined) ??
		[];

	return {
		deployed: positions.reduce((sum, position) => {
			const value = Number(position.value ?? 0);
			const mark = Number(position.mark ?? 0);
			const quantity = Number(position.quantity ?? 0);
			const exposure =
				Number.isFinite(value) && value > 0 ? value : mark * quantity;

			return Number.isFinite(exposure) && exposure > 0 ? sum + exposure : sum;
		}, 0),
		positionCount: positions.length,
	};
};

export const allocationRows = (
	model: TerminalModel,
	quote = "quote unavailable",
	exposure: { deployed: number; positionCount: number } = {
		deployed: 0,
		positionCount: 0,
	},
): AllocationResult => {
	const decisions = model.decisions ?? [];
	const freeCash = parseCurrency(model.wallet?.available ?? "0");
	const reserved = parseCurrency(model.wallet?.reserved ?? "0");
	const scores = decisions.map((decision) => decision.scoreValue);
	const low = scores.length > 0 ? Math.min(...scores) * 0.92 : 0;
	const high = scores.length > 0 ? Math.max(...scores) * 1.04 : 1;
	const span = high - low || 1;
	const percentOf = (value: number): number =>
		clamp(((value - low) / span) * 100, 0, 100);

	const candidates: AllocationCandidate[] = decisions.map((decision) => {
		const positionPercent = percentOf(decision.scoreValue);

		return {
			key: decision.key,
			symbol: decision.symbol,
			scoreValue: decision.scoreValue,
			verdict: decision.verdict,
			why: decision.why,
			fraction: decision.fraction ?? 0,
			positionPercent,
			tick: decision.tick,
		};
	});

	const deployed = Math.max(0, exposure.deployed);
	const accountValue = freeCash + reserved + deployed;
	const tick = candidates.reduce(
		(maxTick, candidate) => Math.max(maxTick, candidate.tick ?? 0),
		0,
	);

	return {
		candidates,
		deployed,
		deployedPercent:
			accountValue > 0 ? clamp((deployed / accountValue) * 100, 0, 100) : 0,
		freeCash,
		reserved,
		quote,
		admittedCount: candidates.filter(
			(candidate) => candidate.verdict === "allow",
		).length,
		positionCount: exposure.positionCount,
		tick: tick > 0 ? tick : undefined,
	};
};

export const allocationModelFromStores = (
	balances: Record<string, unknown> | null,
	decisionFrame:
		| Record<string, unknown>
		| Array<Record<string, unknown>>
		| null = null,
	positionsFrame: Record<string, unknown> | null = null,
): AllocationResult => {
	const funds = quoteFromBalances(balances);
	const exposure = positionExposure(positionsFrame);
	const decisions =
		decisionFrame === null ||
		(Array.isArray(decisionFrame) && decisionFrame.length === 0)
			? []
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
		exposure,
	);
};

const dotColor = (candidate: AllocationCandidate): string => {
	if (candidate.verdict === "allow") {
		return "var(--acc)";
	}

	if (candidate.verdict === "in-play") {
		return "var(--info)";
	}

	return "var(--f4)";
};

const AllocationLegend = () => (
	<div className="mt-[11px] flex items-center gap-4 font-mono text-[9px] text-(--f3)">
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--acc)" />
			admitted
		</span>
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--info)" />
			in play
		</span>
		<span className="inline-flex items-center gap-[5px]">
			<span className="h-2 w-2 rounded-full bg-(--f4)" />
			blocked
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
				<span className="text-(--f3)">decision batch</span>
				<span className="text-(--f4)">
					tick <span className="text-(--f2)">{alloc.tick ?? "—"}</span>
				</span>
				<span className="text-(--f4)">
					admitted <span className="text-(--acc)">{alloc.admittedCount}</span>
				</span>
				<span className="ml-auto text-(--f4)">
					score, verdict, and fraction are backend fields
				</span>
			</div>

			<div className="flex items-center gap-[9px] border-(--line) border-b pb-[7px] font-mono text-[8.5px] text-(--f4) uppercase tracking-[0.06em]">
				<span className="w-[58px] shrink-0">symbol</span>
				<span className="flex-1">trader score</span>
				<span className="w-[66px] shrink-0 text-right">verdict</span>
				<span className="w-[58px] shrink-0 text-right">fraction</span>
				<span className="w-[100px] shrink-0 text-right">reason</span>
			</div>

			<div className="flex flex-col">
				{alloc.candidates.map((candidate) => (
					<div
						key={candidate.key}
						data-symbol={candidate.symbol}
						className="flex items-center gap-[9px] border-(--line) border-b py-[7px]"
					>
						<span
							className="w-[58px] shrink-0 cursor-pointer font-mono text-[11px] font-semibold"
							style={{ color: dotColor(candidate) }}
						>
							{candidate.symbol}
						</span>
						<div className="relative h-[18px] flex-1">
							<div className="absolute top-2 right-0 left-0 h-px bg-(--line)" />
							<div
								className="absolute top-[7px] left-0 h-[3px] rounded-sm bg-(--acc)"
								style={{
									width: `${candidate.positionPercent}%`,
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
						<span className="w-[66px] shrink-0 text-right font-mono text-[10px] uppercase">
							{candidate.verdict}
						</span>
						<span className="w-[58px] shrink-0 text-right font-mono text-[10px] text-(--f2)">
							{(candidate.fraction * 100).toFixed(1)}%
						</span>
						<span className="w-[100px] truncate text-right font-mono text-[10px] text-(--f4)">
							{candidate.why}
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
	const admitted = alloc.candidates
		.filter((candidate) => candidate.verdict === "allow");

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
					open position value from backend ledger
				</div>
				<Bar percent={alloc.deployedPercent} />
				<div className="mt-[7px] flex justify-between font-mono text-[10px] text-(--f3)">
					<span>deployed {currency(alloc.deployed, alloc.quote)}</span>
					<span>positions {alloc.positionCount}</span>
				</div>
			</Panel>

			<Panel>
				<div className="mb-0.5 font-semibold text-[12px] text-(--f1)">
					Admitted candidates
				</div>
				<div className="mb-[11px] font-mono text-[9.5px] text-(--f4)">
					current backend decision batch
				</div>
				<div className="flex flex-col gap-[9px]">
					{admitted.map((candidate) => (
						<div key={candidate.key} data-symbol={candidate.symbol}>
							<div className="mb-1 flex items-center justify-between">
								<span className="cursor-pointer font-mono text-[11px] font-semibold text-(--f1)">
									{candidate.symbol}
								</span>
								<span className="font-mono text-[10.5px] text-(--acc)">
									score {fixed(candidate.scoreValue)}
								</span>
							</div>
							<div className="flex items-center gap-2">
								<div className="h-1.5 flex-1 overflow-hidden rounded bg-(--line)">
									<div
										className="h-full bg-(--acc) transition-[width] duration-500"
										style={{
											width: `${clamp(candidate.fraction * 100, 0, 100)}%`,
										}}
									/>
								</div>
								<span className="w-[38px] text-right font-mono text-[9.5px] text-(--f3)">
									{(candidate.fraction * 100).toFixed(1)}%
								</span>
							</div>
						</div>
					))}
				</div>
				<div className="mt-3 border-(--line) border-t pt-[11px] font-mono text-[9.5px] text-(--f4) leading-[1.6]">
					<div>· no notional is shown without a backend order quantity</div>
					<div>· deployed capital comes only from backend positions</div>
					<div>· every backend candidate remains visible</div>
				</div>
			</Panel>
		</div>
	);
};
