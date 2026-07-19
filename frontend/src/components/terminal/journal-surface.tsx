import { useSelector } from "@tanstack/react-store";
import { useMemo, useState } from "react";
import { appStore } from "#/collections/app";
import type { Holding, LifecycleRow } from "#/collections/types";
import { fixed } from "#/components/terminal/decision-format";
import { LifecycleTrack } from "#/components/terminal/lifecycle-track";
import { TerminalSection } from "#/components/terminal/panels";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { cn } from "#/lib/utils";
import { getWorker } from "#/providers/websocket";
import type { Finding } from "#/types/thesis";
import { Badge } from "@/components/ui/badge";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";

const isOpenLot = (holding: Holding): boolean =>
	holding.qty > 0 &&
	holding.status !== "closed" &&
	holding.status !== "canceled";

const FindingCard = ({ finding }: { finding: Finding }) => {
	const effectPercent = Math.min(
		100,
		Math.round(Math.abs(finding.estimatedEffect) * 100),
	);

	return (
		<Panel variant="surface" size="bare" className="px-3 py-2.5">
			<div className="mb-2 flex items-center justify-between gap-2">
				<Badge label={finding.component} variant="warning" size="xs" />
				<span className="font-mono text-[9px] text-(--f4)">
					±{finding.uncertainty.toFixed(3)} unc
				</span>
			</div>
			<div className="font-medium text-[12px] text-(--f1)">
				{finding.condition}
			</div>
			<Meter
				layout="stacked"
				label="estimated effect"
				value={finding.estimatedEffect.toFixed(4)}
				percent={effectPercent}
				variant={finding.estimatedEffect >= 0 ? "success" : "error"}
				size="s"
				className="mt-2"
				labelClassName="text-[9px] text-(--f4)"
			/>
			{finding.proposedAdjustment ? (
				<div className="mt-2 font-mono text-[10px] text-(--acc)">
					→ {finding.proposedAdjustment}
				</div>
			) : null}
			<ul className="mt-2 flex flex-col gap-1 font-mono text-[9.5px] text-(--f3)">
				{finding.evidence.map((line) => (
					<li key={line} className="border-(--line) border-l pl-2">
						{line}
					</li>
				))}
			</ul>
			<div className="mt-2 font-mono text-[9px] text-(--f4)">
				validate · {finding.requiredValidation}
			</div>
		</Panel>
	);
};

/*
JournalSurface visualizes thesis lifecycle state, retained holdings by status,
and PostMortem findings. Holdings are the inventory authority — there is no
parallel trade-journal frame.
*/
export const JournalSurface = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);
	const [lifecycleRows, setLifecycleRows] = useState<LifecycleRow[]>([]);
	const [holdings, setHoldings] = useState<Holding[]>([]);
	const [findings, setFindings] = useState<Finding[]>([]);
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "lifecycle", key: "" },
			{ store: "holdings", key: "" },
			{ store: "findings", key: "" },
		],
		(buffers) => {
			setLifecycleRows((buffers["lifecycle:"] ?? []) as LifecycleRow[]);
			setHoldings((buffers["holdings:"] ?? []) as Holding[]);
			setFindings((buffers["findings:"] ?? []) as Finding[]);
		},
		[online],
	);

	const lifecycleBySymbol = useMemo(() => {
		const map: Record<string, string> = {};

		for (const row of lifecycleRows) {
			map[row.symbol] = row.state;
		}

		return map;
	}, [lifecycleRows]);

	const symbols = useMemo(() => {
		const symbolSet = new Set<string>([
			...lifecycleRows.map((row) => row.symbol),
			...holdings.map((holding) => holding.symbol),
		]);

		return [...symbolSet].sort();
	}, [lifecycleRows, holdings]);

	const activeSymbol =
		selectedSymbol ??
		(symbols.includes(focusSymbol) ? focusSymbol : symbols[0]) ??
		null;

	const filteredHoldings = useMemo(() => {
		const rows = activeSymbol
			? holdings.filter((holding) => holding.symbol === activeSymbol)
			: holdings;

		return [...rows].sort((left, right) =>
			left.symbol.localeCompare(right.symbol),
		);
	}, [activeSymbol, holdings]);

	const activeFindings = useMemo(() => {
		if (!activeSymbol) {
			return findings;
		}

		return findings.filter((finding) => finding.symbol === activeSymbol);
	}, [activeSymbol, findings]);

	return (
		<div className="grid h-full min-h-0 min-w-[1040px] grid-cols-[minmax(280px,320px)_minmax(420px,1fr)_minmax(280px,320px)]">
			<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
				<TerminalSection
					title="Lifecycle rail"
					meta={`${symbols.length} symbol${symbols.length === 1 ? "" : "s"}`}
				>
					<div className="flex flex-col gap-2 p-2">
						{symbols.length === 0 ? (
							<Panel
								variant="surface"
								size="bare"
								className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
							>
								{online
									? "no active lifecycle"
									: "waiting for lifecycle frames"}
							</Panel>
						) : null}
						{symbols.map((symbol) => (
							<button
								type="button"
								key={symbol}
								onClick={() => {
									setSelectedSymbol(activeSymbol === symbol ? null : symbol);
									appStore.actions.updateFocusSymbol(symbol);
								}}
								data-symbol={symbol}
								className={cn(
									"cursor-pointer text-left",
									activeSymbol === symbol &&
										"rounded ring-1 ring-[color-mix(in_srgb,var(--acc)_35%,transparent)]",
								)}
							>
								<LifecycleTrack
									symbol={symbol}
									state={lifecycleBySymbol[symbol] ?? "observing"}
								/>
							</button>
						))}
					</div>
				</TerminalSection>
			</div>

			<div className="min-h-0 overflow-auto px-4 py-[18px]">
				<div className="mb-3 flex items-baseline justify-between">
					<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Holdings
					</span>
					<span className="font-mono text-[9.5px] text-(--f4)">
						{activeSymbol ?? "all symbols"} · {filteredHoldings.length} lots
					</span>
				</div>
				<div className="flex flex-col gap-2">
					{filteredHoldings.length === 0 ? (
						<Panel
							variant="surface"
							size="bare"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							{online ? "no holdings retained" : "waiting for position frames"}
						</Panel>
					) : null}
					{filteredHoldings.map((holding) => (
						<Panel
							key={`${holding.symbol}:${String(holding.status)}:${holding.qty}`}
							variant="surface"
							size="bare"
							className="grid grid-cols-[1fr_auto] items-start gap-3 px-3 py-2.5"
						>
							<div className="min-w-0">
								<div className="font-mono font-semibold text-[12px] text-(--f1)">
									{holding.symbol}
								</div>
								<div className="mt-0.5 truncate font-mono text-[10px] text-(--f3)">
									qty {fixed(holding.qty)} · mark {fixed(holding.mark)} · pnl{" "}
									{fixed(holding.pnl)}
									{isOpenLot(holding) ? "" : " · closed"}
								</div>
							</div>
							<Badge
								label={
									typeof holding.status === "string"
										? holding.status
										: "unknown"
								}
								variant={isOpenLot(holding) ? "success" : "info"}
								size="xs"
							/>
						</Panel>
					))}
				</div>
			</div>

			<div className="min-h-0 overflow-auto border-(--line) border-l p-3.5">
				<TerminalSection
					title="PostMortem findings"
					meta={`${activeFindings.length} finding${activeFindings.length === 1 ? "" : "s"}`}
				>
					<div className="flex flex-col gap-2 p-2">
						{activeFindings.length === 0 ? (
							<Panel
								variant="surface"
								size="bare"
								className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
							>
								{online
									? "no postmortem findings"
									: "waiting for findings frames"}
							</Panel>
						) : null}
						{activeFindings.map((finding) => (
							<FindingCard
								key={`${finding.component}:${finding.condition}:${finding.estimatedEffect}`}
								finding={finding}
							/>
						))}
					</div>
				</TerminalSection>
			</div>
		</div>
	);
};
