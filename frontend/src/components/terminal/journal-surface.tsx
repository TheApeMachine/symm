import { useSelector } from "@tanstack/react-store";
import { useMemo, useState } from "react";
import { appStore } from "#/collections/app";
import { findingsStore } from "#/collections/findings";
import { lifecycleStore } from "#/collections/lifecycle";
import { tradeJournalStore } from "#/collections/trade-journal";
import { LifecycleTrack } from "#/components/terminal/lifecycle-track";
import { TerminalSection } from "#/components/terminal/panels";
import { cn } from "#/lib/utils";
import type { Finding, TradeObservation } from "#/types/thesis";
import { Badge } from "@/components/ui/badge";
import { Meter } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";

const formatTime = (value: string): string =>
	value.length >= 19 ? value.slice(11, 19) : value;

const observationSummary = (observation: TradeObservation): string => {
	const parts = [
		observation.kind,
		observation.action,
		observation.side,
		observation.status,
	].filter((part) => typeof part === "string" && part.length > 0);

	if (observation.quantity && observation.price) {
		parts.push(`${observation.quantity} @ ${observation.price}`);
	}

	if (observation.pnl) {
		parts.push(`pnl ${observation.pnl}`);
	}

	if (observation.error) {
		parts.push(observation.error);
	}

	return parts.join(" · ");
};

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
JournalSurface visualizes thesis lifecycle state, immutable trade observations,
and PostMortem findings as three linked rails so backend-only data is inspectable.
*/
export const JournalSurface = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const lifecycle = useSelector(lifecycleStore, (state) => state.lifecycle);
	const observations = useSelector(
		tradeJournalStore,
		(state) => state.observations,
	);
	const findings = useSelector(findingsStore, (state) => state.findings);
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);

	const symbols = useMemo(() => {
		const symbolSet = new Set<string>([
			...Object.keys(lifecycle),
			...observations.map((observation) => observation.symbol),
		]);

		return [...symbolSet].sort();
	}, [lifecycle, observations]);

	const activeSymbol =
		selectedSymbol ??
		(symbols.includes(focusSymbol) ? focusSymbol : symbols[0]) ??
		null;

	const filteredObservations = useMemo(() => {
		const rows = activeSymbol
			? observations.filter(
					(observation) => observation.symbol === activeSymbol,
				)
			: observations;

		return [...rows].sort((left, right) => left.at.localeCompare(right.at));
	}, [activeSymbol, observations]);

	const activeFindings = activeSymbol
		? findings.filter((finding) =>
				finding.evidence.some((line) => line.includes(activeSymbol)),
			)
		: findings;

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
								waiting for lifecycle frames
							</Panel>
						) : null}
						{symbols.map((symbol) => (
							<button
								type="button"
								key={symbol}
								onClick={() =>
									setSelectedSymbol(activeSymbol === symbol ? null : symbol)
								}
								className={cn(
									"cursor-pointer text-left",
									activeSymbol === symbol &&
										"rounded ring-1 ring-[color-mix(in_srgb,var(--acc)_35%,transparent)]",
								)}
							>
								<LifecycleTrack
									symbol={symbol}
									state={lifecycle[symbol] ?? "observing"}
								/>
							</button>
						))}
					</div>
				</TerminalSection>
			</div>

			<div className="min-h-0 overflow-auto px-4 py-[18px]">
				<div className="mb-3 flex items-baseline justify-between">
					<span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
						Trade journal
					</span>
					<span className="font-mono text-[9.5px] text-(--f4)">
						{activeSymbol ?? "all symbols"} · {filteredObservations.length}{" "}
						events
					</span>
				</div>
				<div className="flex flex-col gap-2">
					{filteredObservations.length === 0 ? (
						<Panel
							variant="surface"
							size="bare"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							waiting for trade journal frames
						</Panel>
					) : null}
					{filteredObservations.map((observation, index) => (
						<Panel
							key={`${observation.symbol}:${observation.kind}:${observation.at}:${index}`}
							variant="surface"
							size="bare"
							className="grid grid-cols-[64px_1fr_auto] items-start gap-3 px-3 py-2.5"
						>
							<span className="font-mono text-[10px] text-(--f4)">
								{formatTime(observation.at)}
							</span>
							<div className="min-w-0">
								<div className="font-mono font-semibold text-[12px] text-(--f1)">
									{observation.symbol}
								</div>
								<div className="mt-0.5 truncate font-mono text-[10px] text-(--f3)">
									{observationSummary(observation)}
								</div>
							</div>
							<Badge
								label={observation.kind}
								variant={
									observation.kind === "lifecycle_transition"
										? "info"
										: observation.kind === "final_outcome"
											? "success"
											: observation.error
												? "error"
												: "warning"
								}
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
								waiting for findings frames
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
