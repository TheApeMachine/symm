import { createRef, useEffect } from "react";
import { appStore } from "#/collections/app";
import type { Holding, LifecycleRow } from "#/collections/types";
import {
	findingKey,
	holdingKey,
	paintJournalSurface,
} from "#/components/terminal/journal-paint";
import { TerminalSection } from "#/components/terminal/panels";
import type { Finding } from "#/types/thesis";
import { Panel } from "@/components/ui/panel";

const rootRef = createRef<HTMLDivElement>();

let lastLifecycle: LifecycleRow[] = [];
let lastHoldings: Holding[] = [];
let lastFindings: Finding[] = [];
let selectedSymbol: string | null = null;

const asRows = <T,>(value: unknown): T[] =>
	(Array.isArray(value)
		? value
		: value !== null && typeof value === "object"
			? Object.values(value as Record<string, T>)
			: value != null
				? [value]
				: []) as T[];

const selectLifecycleSymbol = (symbol: string) => {
	selectedSymbol = selectedSymbol === symbol ? null : symbol;
	appStore.actions.updateFocusSymbol(symbol);
	paint();
};

/*
paint writes lifecycle / holdings / findings shells from the module caches.
*/
const paint = () => {
	const focusSymbol = appStore.state.focusSymbol;
	const nextSymbols = [
		...new Set([
			...lastLifecycle.map((row) => row.symbol),
			...lastHoldings.map((holding) => holding.symbol),
		]),
	].sort();
	const nextHoldingKeys = [
		...new Set(lastHoldings.map((holding) => holdingKey(holding))),
	].sort();
	const nextFindingKeys = [
		...new Set(lastFindings.map((finding) => findingKey(finding))),
	].sort();
	const nextActive =
		selectedSymbol ??
		(nextSymbols.includes(focusSymbol) ? focusSymbol : nextSymbols[0]) ??
		null;

	paintJournalSurface(rootRef.current, {
		activeSymbol: nextActive,
		findings: lastFindings,
		findingKeys: nextFindingKeys,
		holdings: lastHoldings,
		holdingKeys: nextHoldingKeys,
		lifecycleBySymbol: Object.fromEntries(
			lastLifecycle.map((row) => [row.symbol, String(row.state)]),
		),
		onLifecycleSelect: selectLifecycleSymbol,
		online: appStore.state.online,
		symbols: nextSymbols,
	});
};

/*
paintJournalLifecycle refreshes the journal from the current DRAW lifecycle batch.
*/
export const paintJournalLifecycle = (value: unknown, _focusSymbol: string) => {
	lastLifecycle = asRows<LifecycleRow>(value);
	paint();
};

/*
paintJournalHoldings refreshes the journal from the current DRAW holdings batch.
*/
export const paintJournalHoldings = (value: unknown, _focusSymbol: string) => {
	lastHoldings = asRows<Holding>(value);
	paint();
};

/*
paintJournalFindings refreshes the journal from the current DRAW findings batch.
*/
export const paintJournalFindings = (value: unknown, _focusSymbol: string) => {
	lastFindings = asRows<Finding>(value);
	paint();
};

/*
JournalSurface is the static lifecycle / holdings / findings shell.
DRAW paints via paintJournalLifecycle, paintJournalHoldings, paintJournalFindings.
*/
export const JournalSurface = () => (
	<JournalSurfaceBody />
);

const JournalSurfaceBody = () => {
	useEffect(() => {
		paint();
	}, []);

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
				<div className="flex flex-col gap-2 p-2" data-journal-host="lifecycle">
					<Panel
						variant="surface"
						size="bare"
						data-journal="lifecycle-empty"
						className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
					>
						waiting for lifecycle frames
					</Panel>
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
			<div className="flex flex-col gap-2" data-journal-host="holdings">
				<Panel
					variant="surface"
					size="bare"
					data-journal="holdings-empty"
					className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
				>
					waiting for position frames
				</Panel>
			</div>
		</div>

		<div className="min-h-0 overflow-auto border-(--line) border-l p-3.5">
			<TerminalSection
				title="PostMortem findings"
				meta={<span data-journal="findings-meta">0 findings</span>}
			>
				<div className="flex flex-col gap-2 p-2" data-journal-host="findings">
					<Panel
						variant="surface"
						size="bare"
						data-journal="findings-empty"
						className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
					>
						waiting for findings frames
					</Panel>
				</div>
			</TerminalSection>
		</div>
		</div>
	);
};
