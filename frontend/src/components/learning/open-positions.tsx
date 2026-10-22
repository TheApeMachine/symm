import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import { cognitionStore, regionHistoryStore } from "#/collections/cognition";
import { positionReadout } from "#/components/book-readout";
import {
	DistanceBar,
	Flex,
	formatValue,
	Gauge,
	type GaugeTone,
	RadialHistory,
	TokenRibbon,
	Trail,
} from "#/components/ui";
import { cn } from "#/lib/utils";
import { sendCommand } from "#/providers/websocket";
import { useSelector } from "@tanstack/react-store";
import { useMemo, useState } from "react";

/*
Open positions are the desk's per-lot frames: one Value per symbol on the
positions store, whose "position" layer carries the row — entry, mark, fee
and the sale return. The token path and the region ring are the live
train/regions sentence for the same symbol.
*/

const REGIONS = 15;
const TOKEN_GATE = 0.5;
const regionLabel = (sector: number) => `R${String(sector).padStart(2, "0")}`;
const trails = new Map<string, number[]>();

/*
positionRows decodes every lot frame the store holds into its row objects.
The "positions" layer is a JSON array; each element is one lot row.
A layer that decodes to a single object (not an array) is also accepted.
*/
export const positionRows = (
	state: Record<string, { lookup: (keys: string[]) => unknown[] }>,
): Record<string, unknown>[] => {
	const rows: Record<string, unknown>[] = [];

	for (const frame of Object.values(state)) {
		for (const layer of frame.lookup(["position"])) {
			if (Array.isArray(layer)) {
				for (const row of layer) {
					if (row && typeof row === "object" && !Array.isArray(row)) {
						rows.push(row as Record<string, unknown>);
					}
				}
			} else if (layer && typeof layer === "object") {
				rows.push(layer as Record<string, unknown>);
			}
		}
	}

	return rows;
};

export const openLots = (rows: Record<string, unknown>[], book: Book) =>
	rows
		.filter(
			(row) =>
				!positionReadout(row).closed && row.virtual === (book === "virtual"),
		)
		.map((row) => row.symbol)
		.sort();

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

const numberOf = (value: unknown) => {
	if (value === null || value === undefined) return undefined;
	const parsed = Number(value);
	return Number.isFinite(parsed) ? parsed : undefined;
};

const heldSeconds = (row: Record<string, unknown>) => {
	if ((row.hold_ns as number) > 0) return (row.hold_ns as number) / 1e9;
	if (row.opened_at === "" || row.opened_at == null) return undefined;
	const at = Date.parse(row.opened_at as string);
	if (Number.isNaN(at)) return undefined;
	return Math.max(0, (Date.now() - at) / 1000);
};

const sectorOf = (token: string) => {
	const sector = Number(token.slice(1));
	return Number.isInteger(sector) ? sector : undefined;
};

const PositionCard = ({ row }: { row: Record<string, unknown> }) => {
	const [exiting, setExiting] = useState(false);
	const match = useSelector(
		cognitionStore,
		(state) => state.matches[row.symbol as string],
	);
	const rings = useSelector(
		regionHistoryStore,
		(state) => state[row.symbol as string] ?? [],
	);
	const readout = positionReadout(row);
	const entry = readout.entryPrice ?? numberOf(row.entry_price);
	const mark = numberOf(row.mark);
	const qty = numberOf(row.qty);
	const vwap = readout.exitVwap;
	const settled = numberOf(row.net_return);

	/*
	"If sold now" is the settled round trip, not the mark-over-entry price
	move: proceeds against cost after both fee legs — the same arithmetic
	the desk realizes at exit (price.RealizedReturn). The fee fields are
	percent rates (the fee row reads ~1.587 = 1.587%), so the round trip on
	fresh marks is the price move minus the entry rate, plus the exit rate.
	Once the desk has settled, net_return (already percent) is the answer.
	*/
	const entryFee = numberOf(row.entry_fee);
	const exitFee = numberOf(row.exit_fee);
	const soldNow = useMemo(() => {
		if (settled !== undefined) return settled / 100;
		if (entry === undefined || mark === undefined || entry <= 0) {
			return undefined;
		}
		const move = (mark - entry) / entry;
		return move - ((entryFee ?? 0) + (exitFee ?? 0)) / 100;
	}, [settled, entry, mark, entryFee, exitFee]);
	const pnlPct = soldNow;
	const room = readout.exitRoom ?? undefined;
	const pnlTone: GaugeTone =
		pnlPct === undefined ? "muted" : pnlPct >= 0 ? "up" : "down";
	const roomTone: GaugeTone =
		room === undefined ? "muted" : room < 1 ? "down" : room < 3 ? "warn" : "up";

	if (pnlPct !== undefined) {
		const prior = trails.get(row.id as string) ?? [];
		if (prior[prior.length - 1] !== pnlPct) {
			trails.set(row.id as string, [...prior, pnlPct].slice(-90));
		}
	}

	const pnlTrail = trails.get(row.id as string) ?? [];
	const tokens = (match?.tokens ?? [])
		.map(sectorOf)
		.filter((sector): sector is number => sector !== undefined)
		.map((sector) => [sector]);
	const strength = rings.length
		? rings[rings.length - 1]?.[tokens[tokens.length - 1]?.[0] ?? -1]
		: undefined;
	const peak = Math.max(
		1,
		...rings.flat().filter((value): value is number => value !== undefined),
	);
	const scaled = rings.map((ring) =>
		ring.map((value) => (value === undefined ? value : value / peak)),
	);
	const selling = !positionReadout(row).closed && qty === 0;

	return (
		<div
			className={cn(
				"border-r border-b border-(--line) bg-(--surface) p-3 flex flex-col gap-2",
				selling && "animate-pulse",
			)}
		>
			<div className="flex items-center justify-between">
				<div className="flex items-center gap-2">
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
				</div>
				<span className="text-[10px] text-(--f3)">
					held {duration(heldSeconds(row))}
				</span>
			</div>

			<div className="grid grid-cols-[auto_1fr] gap-4 items-center">
				<div className="flex gap-1">
					<Gauge
						value={
							pnlPct === undefined
								? undefined
								: 0.5 + Math.max(-0.5, Math.min(0.5, pnlPct * 5))
						}
						tone={pnlTone}
						ticks={[0.5]}
						label={
							pnlPct === undefined ? undefined : (
								<span style={{ color: `var(--${pnlTone})` }}>
									{formatValue(pnlPct, "pct")}
								</span>
							)
						}
						caption="if sold now"
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
				</div>
				<div className="flex flex-col gap-2 min-w-0">
					<DistanceBar
						origin={entry}
						value={mark}
						markers={
							vwap === null || vwap === undefined
								? []
								: [
										{
											value: vwap,
											label: "average price selling everything",
											color: "var(--info)",
										},
									]
						}
						leftLabel="less than paid"
						rightLabel="more than paid"
					/>
					<Trail values={pnlTrail} width={240} height={40} />
				</div>
			</div>

			<div className="grid grid-cols-[auto_1fr] gap-4 items-start border-t border-(--line) pt-3">
				<div className="flex flex-col items-center">
					<RadialHistory
						sectors={REGIONS}
						rows={scaled}
						highlight={tokens[tokens.length - 1] ?? []}
						labels={regionLabel}
						size={170}
					/>
					<span className="text-[9px] text-(--f4)">
						grid regions · outer ring = now
					</span>
				</div>
				<div className="flex flex-col gap-3 min-w-0">
					<div>
						<div className="text-[10px] text-(--f3) mb-1">token sequence</div>
						<TokenRibbon tokens={tokens} label={regionLabel} />
					</div>
					<div className="flex items-center gap-2">
						<Gauge
							size={56}
							sweep={180}
							value={
								strength === undefined
									? undefined
									: Math.min(1, strength / (2 * TOKEN_GATE))
							}
							tone={
								strength !== undefined && strength >= TOKEN_GATE
									? "acc"
									: "muted"
							}
							ticks={[0.5]}
						/>
						<span className="text-[10px] text-(--f3) leading-tight">
							token strength
							<br />
							<span className="text-(--f4)">tick = needed to advance</span>
						</span>
					</div>
				</div>
			</div>

			<div className="grid grid-cols-4 gap-2 border-t border-(--line) pt-2 text-[10px] text-(--f3)">
				<span>qty {(row.qty as number) ?? "—"}</span>
				<span>entry {(row.entry_price as number) ?? "—"}</span>
				<span>mark {(row.mark as number) ?? "—"}</span>
				<span>fee {(row.entry_fee as number) ?? "—"}</span>
			</div>

			{/*
			Manual exit: the desk settles the lot (hub position.exit), and
			the closed round trip lands on the Position Life-Cycle tab for
			annotation.
			*/}
			<button
				type="button"
				data-l="position-exit"
				disabled={exiting}
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
				className={cn(
					"self-start mt-0.5 px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider border transition-colors",
					exiting
						? "border-(--line) text-(--f4) cursor-wait"
						: "border-(--down)/40 text-(--down) hover:bg-(--down)/10 cursor-pointer",
				)}
			>
				{exiting ? "closing" : "exit"}
			</button>
		</div>
	);
};

export const OpenPositions = () => {
	const book = useSelector(bookAtom, (state) => state);
	const state = useSelector(signals.positions, (state) => state);
	const held = positionRows(state)
		.filter(
			(row) =>
				!positionReadout(row).closed && row.virtual === (book === "virtual"),
		)
		.sort((a, b) => String(a.symbol).localeCompare(String(b.symbol)));

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
		<div className="flex-1 min-h-0 overflow-auto grid grid-cols-[repeat(auto-fill,minmax(440px,1fr))] content-start">
			{held.map((row) => (
				<PositionCard key={String(row.id ?? row.symbol)} row={row} />
			))}
		</div>
	);
};
