import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { signals } from "#/collections/app";
import { type Book, bookAtom } from "#/collections/book";
import {
	exploreExitGaugeMarks,
	pnlGaugeValue,
	positionReadout,
} from "#/components/book-readout";
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
Open positions are the desk's per-lot frames: one Value per symbol on the
positions store, whose "position" layer carries the row — entry, mark, fee
and the sale return from broker/price.go.
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

		for (const layer of frame.lookup(["position"])) {
			if (Array.isArray(layer)) {
				for (const row of layer) {
					if (row && typeof row === "object" && !Array.isArray(row)) {
						take(row as Record<string, unknown>, tick);
					}
				}
			} else if (layer && typeof layer === "object") {
				take(layer as Record<string, unknown>, tick);
			}
		}
	}

	return [...byID.values().map((held) => held.row), ...anon];
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

const heldSeconds = (row: Record<string, unknown>) => {
	if ((row.hold_ns as number) > 0) return (row.hold_ns as number) / 1e9;
	if (row.opened_at === "" || row.opened_at == null) return undefined;
	const at = Date.parse(row.opened_at as string);
	if (Number.isNaN(at)) return undefined;
	return Math.max(0, (Date.now() - at) / 1000);
};

const PositionCard = ({ row }: { row: Record<string, unknown> }) => {
	const [exiting, setExiting] = useState(false);
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

	if (pnlPct !== undefined) {
		const id = String(row.id ?? row.symbol);
		const prior = trails.get(id) ?? [];
		const now = Date.now();
		const last = trailAt.get(id) ?? 0;
		const changed = prior.length === 0 || prior[prior.length - 1] !== pnlPct;
		if (changed || now-last >= TRAIL_SAMPLE_MS) {
			trails.set(id, [...prior, pnlPct].slice(-90));
			trailAt.set(id, now);
		}
	}

	const pnlTrail = trails.get(String(row.id ?? row.symbol)) ?? [];
	const selling = !positionReadout(row).closed && qty === 0;
	const pnlMarks = exploreExitGaugeMarks(row);

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

			<div className="grid grid-cols-[auto_1fr] gap-4 items-start">
				<div className="flex gap-1">
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
				</div>
				<div className="flex min-w-0 flex-col gap-0">
					<Trail
						values={pnlTrail}
						width={240}
						height={GAUGE_SIZE}
						className="max-w-full"
					/>
					<span className="-mt-3 text-[10px] text-(--f3) leading-tight">
						Close-now P&amp;L trail
					</span>
				</div>
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
