import { createRef, useEffect } from "react";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import type { LifecycleRow, Position, Thesis } from "#/collections/types";
import {
	findingKey,
	journalEntryKey,
	paintJournalSurface,
} from "#/components/terminal/journal-paint";
import type { Finding } from "#/types/thesis";
import { Panel } from "@/components/ui/panel";
import { Section } from "@/components/ui/section";

const rootRef = createRef<HTMLDivElement>();

let lastLifecycle: LifecycleRow[] = [];
let lastPositions: Position[] = [];
let lastFindings: Finding[] = [];
let lastJournal: Thesis[] = [];
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

const selectJournalEntry = (key: string) => {
	const entry = lastJournal.find(
		(candidate) => journalEntryKey(candidate) === key,
	);
	const symbol =
		Object.keys(entry?.lifecycle ?? {})[0] ??
		Object.keys(entry?.holdings ?? {})[0] ??
		"";

	if (symbol === "") {
		return;
	}

	selectedSymbol = symbol;
	appStore.actions.updateFocusSymbol(symbol);
	terminalStore.actions.openThesis(symbol);
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
			...lastPositions
				.map((position) => position.holding?.symbol)
				.filter(Boolean),
			...lastJournal.map((entry) => entry.symbol),
		]),
	].sort();
	const nextJournalKeys = [
		...new Set(lastJournal.map((entry) => journalEntryKey(entry))),
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
		entries: lastJournal,
		findings: lastFindings,
		findingKeys: nextFindingKeys,
		journalKeys: nextJournalKeys,
		lifecycleBySymbol: Object.fromEntries(
			lastLifecycle.map((row) => [row.symbol, String(row.state)]),
		),
		onJournalSelect: selectJournalEntry,
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
 paintJournalPositions refreshes the journal from the current DRAW positions batch.
 */
export const paintJournalPositions = (value: unknown, _focusSymbol: string) => {
	lastPositions = (
		Array.isArray(value)
			? value
			: value !== null && typeof value === "object"
				? Object.values(value as Record<string, Position>)
				: value != null
					? [value]
					: []
	) as Position[];
	paint();
};

export const paintJournalHoldings = paintJournalPositions;

/*
paintJournalFindings refreshes the journal from the current DRAW findings batch.
*/
export const paintJournalFindings = (value: unknown, _focusSymbol: string) => {
	lastFindings = asRows<Finding>(value);
	paint();
};

export const paintJournalEntries = (value: unknown, _focusSymbol: string) => {
	lastJournal = asRows<Thesis>(value);
	paint();
};

/*
JournalSurface is the static lifecycle / holdings / findings shell.
 DRAW paints via paintJournalLifecycle, paintJournalPositions, paintJournalFindings.
*/
export const JournalSurface = () => <JournalSurfaceBody />;

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
				<Section>
					<Section.Header
						title="Lifecycle rail"
						meta={<span data-journal="lifecycle-meta">0 symbols</span>}
					/>
					<div
						className="flex flex-col gap-2 p-2"
						data-journal-host="lifecycle"
					>
						<Panel
							variant="surface"
							size="bare"
							data-journal="lifecycle-empty"
							className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
						>
							waiting for lifecycle frames
						</Panel>
					</div>
				</Section>
			</div>

			<div className="min-h-0 overflow-auto px-4 py-[18px]">
				<Section.Header
					size="bare"
					rule={false}
					title="Journal"
					meta={<span data-journal="holdings-meta" />}
				/>
				<div className="flex flex-col gap-2" data-journal-host="journal">
					<Panel
						variant="surface"
						size="bare"
						data-journal="holdings-empty"
						className="px-3 py-8 text-center font-mono text-[11px] text-(--f4)"
					>
						waiting for journal frames
					</Panel>
				</div>
			</div>

			<div className="min-h-0 overflow-auto border-(--line) border-l p-3.5">
				<Section>
					<Section.Header
						title="PostMortem findings"
						meta={<span data-journal="findings-meta">0 findings</span>}
					/>
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
				</Section>
			</div>
		</div>
	);
};
