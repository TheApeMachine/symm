import { useSelector } from "@tanstack/react-store";
import { useRef, useState } from "react";
import { appStore } from "#/collections/app";
import type { Holding, LifecycleRow } from "#/collections/types";
import {
	LifecycleTrack,
	paintJournalSurface,
} from "#/components/terminal/lifecycle-track";
import { TerminalSection } from "#/components/terminal/panels";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { cn } from "#/lib/utils";
import { getWorker } from "#/providers/websocket";
import type { Finding } from "#/types/thesis";
import { badgeVariants } from "@/components/ui/badge";
import { Panel } from "@/components/ui/panel";

const sameSymbols = (left: string[], right: string[]): boolean =>
	left.length === right.length &&
	left.every((symbol, index) => symbol === right[index]);

const holdingKey = (holding: Holding): string =>
	`${holding.symbol}:${String(holding.status)}:${holding.qty}`;

const findingKey = (finding: Finding): string =>
	`${finding.symbol}:${finding.component}:${finding.condition}`;

/*
JournalSurface visualizes thesis lifecycle state, retained holdings by status,
and PostMortem findings. Holdings are the inventory authority — there is no
parallel trade-journal frame.
*/
export const JournalSurface = () => {
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const online = useSelector(appStore, (state) => state.online);
	const rootRef = useRef<HTMLDivElement>(null);
	const [symbols, setSymbols] = useState<string[]>([]);
	const [holdingKeys, setHoldingKeys] = useState<string[]>([]);
	const [findingKeys, setFindingKeys] = useState<string[]>([]);
	const [selectedSymbol, setSelectedSymbol] = useState<string | null>(null);

	const activeSymbol =
		selectedSymbol ??
		(symbols.includes(focusSymbol) ? focusSymbol : symbols[0]) ??
		null;

	useDirectStorePaint(
		getWorker(),
		[
			{ store: "lifecycle", key: "" },
			{ store: "holdings", key: "" },
			{ store: "findings", key: "" },
		],
		(buffers) => {
			const lifecycleRows = (buffers["lifecycle:"] ?? []) as LifecycleRow[];
			const holdings = (buffers["holdings:"] ?? []) as Holding[];
			const findings = (buffers["findings:"] ?? []) as Finding[];
			const nextSymbols = [
				...new Set([
					...lifecycleRows.map((row) => row.symbol),
					...holdings.map((holding) => holding.symbol),
				]),
			].sort();
			const nextHoldingKeys = [
				...new Set(holdings.map((holding) => holdingKey(holding))),
			].sort();
			const nextFindingKeys = [
				...new Set(findings.map((finding) => findingKey(finding))),
			].sort();
			const nextActive =
				selectedSymbol ??
				(nextSymbols.includes(focusSymbol)
					? focusSymbol
					: nextSymbols[0]) ??
				null;

			setSymbols((previous) =>
				sameSymbols(previous, nextSymbols) ? previous : nextSymbols,
			);
			setHoldingKeys((previous) =>
				sameSymbols(previous, nextHoldingKeys)
					? previous
					: nextHoldingKeys,
			);
			setFindingKeys((previous) =>
				sameSymbols(previous, nextFindingKeys)
					? previous
					: nextFindingKeys,
			);
			paintJournalSurface(rootRef.current, {
				activeSymbol: nextActive,
				findings,
				findingKeys: nextFindingKeys,
				holdings,
				holdingKeys: nextHoldingKeys,
				lifecycleBySymbol: Object.fromEntries(
					lifecycleRows.map((row) => [row.symbol, String(row.state)]),
				),
				online,
				symbols: nextSymbols,
			});
		},
		[online, selectedSymbol, focusSymbol],
	);

	return (
		<div
			ref={rootRef}
			className="grid h-full min-h-0 min-w-[1040px] grid-cols-[minmax(280px,320px)_minmax(420px,1fr)_minmax(280px,320px)]"
		>
			<div className="min-h-0 overflow-auto border-(--line) border-r p-3.5">
				<TerminalSection
					title="Lifecycle rail"
					meta={<span data-journal="lifecycle-meta">0 symbols</span>}
				>
					<div className="flex flex-col gap-2 p-2">
						<Panel
							variant="surface"
							size="bare"
							data-journal="lifecycle-empty"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							waiting for lifecycle frames
						</Panel>
						{symbols.map((symbol) => (
							<button
								type="button"
								key={symbol}
								onClick={() => {
									setSelectedSymbol(
										activeSymbol === symbol ? null : symbol,
									);
									appStore.actions.updateFocusSymbol(symbol);
								}}
								data-symbol={symbol}
								className={cn(
									"cursor-pointer text-left",
									activeSymbol === symbol &&
										"rounded ring-1 ring-[color-mix(in_srgb,var(--acc)_35%,transparent)]",
								)}
							>
								<LifecycleTrack symbol={symbol} />
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
					<span
						data-journal="holdings-meta"
						className="font-mono text-[9.5px] text-(--f4)"
					/>
				</div>
				<div className="flex flex-col gap-2">
					<Panel
						variant="surface"
						size="bare"
						data-journal="holdings-empty"
						className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
					>
						waiting for position frames
					</Panel>
					{holdingKeys.map((key) => (
						<Panel
							key={key}
							variant="surface"
							size="bare"
							data-holding={key}
							className="grid grid-cols-[1fr_auto] items-start gap-3 px-3 py-2.5"
							style={{ display: "none" }}
						>
							<div className="min-w-0">
								<div
									data-journal="holding-symbol"
									className="font-mono font-semibold text-[12px] text-(--f1)"
								/>
								<div
									data-journal="holding-meta"
									className="mt-0.5 truncate font-mono text-[10px] text-(--f3)"
								/>
							</div>
							<span data-journal="holding-status" />
						</Panel>
					))}
				</div>
			</div>

			<div className="min-h-0 overflow-auto border-(--line) border-l p-3.5">
				<TerminalSection
					title="PostMortem findings"
					meta={<span data-journal="findings-meta">0 findings</span>}
				>
					<div className="flex flex-col gap-2 p-2">
						<Panel
							variant="surface"
							size="bare"
							data-journal="findings-empty"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							waiting for findings frames
						</Panel>
						{findingKeys.map((key) => (
							<Panel
								key={key}
								variant="surface"
								size="bare"
								data-finding={key}
								className="px-3 py-2.5"
								style={{ display: "none" }}
							>
								<div className="mb-2 flex items-center justify-between gap-2">
									<span
										data-journal="finding-component"
										className={cn(
											badgeVariants({ variant: "warning", size: "xs" }),
										)}
									/>
									<span
										data-journal="finding-unc"
										className="font-mono text-[9px] text-(--f4)"
									/>
								</div>
								<div
									data-journal="finding-condition"
									className="font-medium text-[12px] text-(--f1)"
								/>
								<div className="mt-2">
									<div className="mb-1 flex justify-between font-mono text-[9px] text-(--f4)">
										<span>estimated effect</span>
										<span
											data-journal="finding-effect"
											className="text-(--f1)"
										/>
									</div>
									<div className="h-[5px] overflow-hidden rounded-[3px] bg-(--line)">
										<div
											data-journal="finding-fill"
											className="h-full"
											style={{ width: "0%" }}
										/>
									</div>
								</div>
								<div
									data-journal="finding-adjustment"
									className="mt-2 font-mono text-[10px] text-(--acc)"
									style={{ display: "none" }}
								/>
								<ul
									data-journal="finding-evidence"
									className="mt-2 flex flex-col gap-1 font-mono text-[9.5px] text-(--f3)"
								/>
								<div
									data-journal="finding-validate"
									className="mt-2 font-mono text-[9px] text-(--f4)"
								/>
							</Panel>
						))}
					</div>
				</TerminalSection>
			</div>
		</div>
	);
};
