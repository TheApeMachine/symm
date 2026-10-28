import { useSelector } from "@tanstack/react-store";
import { memo, useEffect, useMemo, useState } from "react";
import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import {
	comparePositionRows,
	framePositionRows,
} from "#/collections/positions";
import {
	exploreExitGaugeMarks,
	onBook,
	pnlGaugeValue,
	positionReadout,
} from "#/components/book-readout";
import { OpenChoiceChart } from "#/components/learning/open-choice-chart";
import { PositionLotVisual } from "#/components/learning/position-lot-visual";
import {
	BadgeButton,
	Flex,
	formatValue,
	Gauge,
	type GaugeTone,
	Trail,
} from "#/components/ui";
import { cn } from "#/lib/utils";
import { sendCommand } from "#/providers/websocket";

/*
Open positions are the desk's per-lot frames. The connect snapshot and each
later mark tick land on the positions store; a tick names only the lots
that moved, and the book keeps every lot that tick did not mention.
*/

const trails = new Map<string, number[]>();
const trailAt = new Map<string, number>();
const TRAIL_SAMPLE_MS = 250;

/* Match default Gauge SVG height so the P&L trail aligns with the dials. */
const GAUGE_SIZE = 96;

/*
positionRows decodes every lot frame the store holds into its row objects.
The "positions" layer is a JSON array; each element is one lot row.
A layer that decodes to a single object (not an array) is also accepted.

A connect snapshot and a later per-lot update both carry the same id. The
newer tick wins, and a lot that only appears in one of them stays.
*/
export const positionRows = (
	state: Record<
		string,
		{ lookup: (keys: string[]) => unknown[]; tick?: bigint }
	>,
): Record<string, unknown>[] => {
	const byID = new Map<
		string,
		{ row: Record<string, unknown>; tick: bigint }
	>();
	const anon: Record<string, unknown>[] = [];

	const take = (row: Record<string, unknown>, tick: bigint) => {
		const id = typeof row.id === "string" ? row.id : "";

		if (id === "") {
			anon.push(row);
			return;
		}

		const prior = byID.get(id);

		if (!prior || tick >= prior.tick) {
			byID.set(id, { row, tick });
		}
	};

	for (const frame of Object.values(state)) {
		const tick = frame.tick ?? 0n;

		for (const row of framePositionRows(frame)) {
			take(row, tick);
		}
	}

	const rows = [...Array.from(byID.values(), (held) => held.row), ...anon];
	rows.sort(comparePositionRows);
	return rows;
};

/** Cheap fingerprint so cards re-render when lot marks move, not on unrelated scopes. */
export const positionsRevision = (
	state: Record<string, { tick?: bigint }>,
): string => {
	const parts: string[] = [];
	for (const [scope, frame] of Object.entries(state)) {
		parts.push(`${scope}:${frame.tick ?? 0n}`);
	}
	parts.sort();
	return parts.join("|");
};

export const openLots = (rows: Record<string, unknown>[], book: Book) =>
	rows
		.filter((row) => !positionReadout(row).closed && onBook(row, book))
		.map((row) => row.symbol)
		.sort();

/** Close-now return after fees; used for the in-profit filter on this page. */
export const lotInProfit = (row: Record<string, unknown>) => {
	const pct = positionReadout(row).returnPct;
	return pct !== null && pct > 0;
};

const duration = (seconds?: number) => {
	if (seconds === undefined) return "—";
	const m = Math.floor(seconds / 60);
	const h = Math.floor(m / 60);
	return h > 0
		? `${h}h ${m % 60}m`
		: m > 0
			? `${m}m ${Math.floor(seconds % 60)}s`
			: `${Math.floor(seconds)}s`;
};

const heldSeconds = (row: Record<string, unknown>) => {
	if ((row.hold_ns as number) > 0) return (row.hold_ns as number) / 1e9;
	if (row.opened_at === "" || row.opened_at == null) return undefined;
	const at = Date.parse(row.opened_at as string);
	if (Number.isNaN(at)) return undefined;
	return Math.max(0, (Date.now() - at) / 1000);
};

const PositionCard = memo(({ row }: { row: Record<string, unknown> }) => {
	const [exiting, setExiting] = useState(false);
	const [pnlTrail, setPnlTrail] = useState<number[]>([]);
	const readout = positionReadout(row);
	const entry = readout.entryPrice;
	const mark = readout.mark;
	const qty = readout.qty;
	const vwap = readout.exitVwap;
	const pnlPct = readout.returnPct ?? undefined;
	const room = readout.exitRoom ?? undefined;
	const pnlTone: GaugeTone =
		pnlPct === undefined ? "muted" : pnlPct >= 0 ? "up" : "down";
	const roomTone: GaugeTone =
		room === undefined ? "muted" : room < 1 ? "down" : room < 3 ? "warn" : "up";

	const lotId = String(row.id ?? row.symbol);

	useEffect(() => {
		if (pnlPct === undefined) {
			setPnlTrail([]);
			return;
		}

		const sample = () => {
			const prior = trails.get(lotId) ?? [];
			const now = Date.now();
			const last = trailAt.get(lotId) ?? 0;
			const changed = prior.length === 0 || prior[prior.length - 1] !== pnlPct;
			if (!changed && now - last < TRAIL_SAMPLE_MS) {
				setPnlTrail(prior);
				return;
			}

			const next = [...prior, pnlPct].slice(-90);
			trails.set(lotId, next);
			trailAt.set(lotId, now);
			setPnlTrail(next);
		};

		sample();
		const timer = window.setInterval(sample, TRAIL_SAMPLE_MS);
		return () => window.clearInterval(timer);
	}, [lotId, pnlPct]);
	const selling = !positionReadout(row).closed && qty === 0;
	const pnlMarks = exploreExitGaugeMarks(row);

	return (
		<Flex.Column
			gap={2}
			padding={3}
			className={cn(
				"border-r border-b border-(--line) bg-(--surface)",
				selling && "animate-pulse",
			)}
		>
			<Flex.Row align="center" justify="between">
				<Flex.Row align="center" gap={2}>
					<span
						className="w-2.5 h-2.5 rounded-full"
						style={{
							background: `var(--${pnlTone === "muted" ? "f4" : pnlTone})`,
						}}
					/>
					<span className="text-(--f1) font-bold text-sm">
						{row.symbol as string}
					</span>
					{selling && (
						<span className="text-[10px] px-1.5 rounded bg-(--down)/20 text-(--down)">
							flat
						</span>
					)}
				</Flex.Row>
				<span className="text-[10px] text-(--f3)">
					held {duration(heldSeconds(row))}
				</span>
			</Flex.Row>

			<Flex.Row gap={1} wrap="wrap">
				<Gauge
					value={pnlGaugeValue(pnlPct)}
					tone={pnlTone}
					marks={pnlMarks}
					label={
						pnlPct === undefined ? undefined : (
							<span style={{ color: `var(--${pnlTone})` }}>
								{formatValue(pnlPct, "pct")}
							</span>
						)
					}
					caption="P&L"
				/>
				<Gauge
					value={
						room === undefined
							? undefined
							: Math.max(
									0,
									Math.min(
										1,
										Math.log10(Math.max(room, 1e-3) * 10) / Math.log10(200),
									),
								)
					}
					tone={roomTone}
					ticks={[Math.log10(10) / Math.log10(200)]}
					label={
						room === undefined
							? undefined
							: `${room >= 100 ? Math.round(room) : room.toFixed(1)}×`
					}
					caption="exit room"
				/>
			</Flex.Row>
			<div className="w-full min-w-0">
				<Trail values={pnlTrail} height={GAUGE_SIZE} className="w-full" />
				<span className="-mt-3 block text-[10px] text-(--f3) leading-tight">
					Close-now P&amp;L trail
				</span>
			</div>

			<div className="border-t border-(--line) pt-3">
				<PositionLotVisual
					pnlPct={pnlPct}
					exitRoom={room}
					entry={entry ?? undefined}
					mark={mark ?? undefined}
					exitVwap={vwap ?? undefined}
					waitingOnBranch={row.path_found === true}
				/>
			</div>

			{/*
			Manual exit: the desk settles the lot (hub position.exit), and
			the closed round trip lands on the Position Life-Cycle tab for
			annotation.
			*/}
			<BadgeButton
				data-l="position-exit"
				disabled={exiting}
				size="xs"
				variant="error"
				className={cn(
					"self-start mt-0.5",
					exiting
						? "cursor-wait opacity-60"
						: "hover:bg-[color-mix(in_srgb,var(--error)_18%,transparent)]",
				)}
				onClick={() => {
					setExiting(true);

					try {
						sendCommand({
							type: "position.exit",
							symbol: row.symbol,
							virtual: row.virtual,
						});
					} catch (cause) {
						console.error(cause);
					}
				}}
				label={exiting ? "closing" : "exit"}
			/>
		</Flex.Column>
	);
});

export const OpenPositions = () => {
	const book = useSelector(bookAtom, (state) => state);
	const positionsState = useSelector(signals.positions, (state) => state);
	const [profitOnly, setProfitOnly] = useState(false);
	const held = useMemo(
		() =>
			positionRows(positionsState)
				.filter((row) => !positionReadout(row).closed && onBook(row, book))
				.sort(comparePositionRows),
		[book, positionsState],
	);
	const shown = useMemo(
		() => (profitOnly ? held.filter(lotInProfit) : held),
		[held, profitOnly],
	);

	if (held.length === 0) {
		return (
			<Flex.Column
				align="center"
				justify="center"
				gap={2}
				className="flex-1 text-(--f3)"
			>
				<div className="w-16 h-16 rounded-full border-2 border-dashed border-(--line)" />
				<span className="text-sm">
					{book === "virtual" ? "No virtual trades" : "No open positions"}
				</span>
				<span className="text-[10px] text-(--f4)">
					{book === "virtual"
						? "a card appears when a finished rule opens a virtual trade"
						: "cards appear here as soon as the desk holds a symbol"}
				</span>
			</Flex.Column>
		);
	}

	return (
		<Flex.Column className="flex-1 min-h-0 w-full min-w-0 overflow-auto">
			<Flex.Row
				align="center"
				justify="between"
				gap={2}
				className="shrink-0 border-b border-(--line) bg-(--surface) px-3 py-1.5"
			>
				<span className="text-[10px] uppercase tracking-wider text-(--f4)">
					Show
				</span>
				<fieldset className="m-0 flex items-center gap-0.5 rounded border border-(--line) bg-(--sunken) p-0.5">
					<legend className="sr-only">Open position filter</legend>
					<button
						type="button"
						data-l="positions-filter-all"
						aria-pressed={!profitOnly}
						onClick={() => setProfitOnly(false)}
						className={cn(
							"rounded px-2.5 py-0.5 text-[11px] transition-colors cursor-pointer",
							!profitOnly
								? "bg-(--raised) font-bold text-(--f1) shadow-xs"
								: "text-(--f3) hover:text-(--f1)",
						)}
					>
						All
						<span className="ml-1 tabular-nums text-(--f4)">{held.length}</span>
					</button>
					<button
						type="button"
						data-l="positions-filter-profit"
						aria-pressed={profitOnly}
						onClick={() => setProfitOnly(true)}
						className={cn(
							"rounded px-2.5 py-0.5 text-[11px] transition-colors cursor-pointer",
							profitOnly
								? "bg-(--raised) font-bold text-(--up) shadow-xs"
								: "text-(--f3) hover:text-(--f1)",
						)}
					>
						In profit
						<span className="ml-1 tabular-nums text-(--f4)">
							{held.filter(lotInProfit).length}
						</span>
					</button>
				</fieldset>
			</Flex.Row>
			<OpenChoiceChart lots={shown} />
			{shown.length === 0 ? (
				<Flex.Column
					align="center"
					justify="center"
					gap={2}
					className="flex-1 px-3 py-10 text-(--f3)"
				>
					<span className="text-sm">No lots ahead after fees</span>
					<span className="max-w-sm text-center text-[10px] text-(--f4)">
						Every open lot is flat or behind on close-now P&amp;L. Switch to All
						to see them.
					</span>
				</Flex.Column>
			) : (
				<div className="grid grid-cols-[repeat(auto-fill,minmax(440px,1fr))] content-start">
					{shown.map((row) => (
						<PositionCard key={String(row.id ?? row.symbol)} row={row} />
					))}
				</div>
			)}
		</Flex.Column>
	);
};
