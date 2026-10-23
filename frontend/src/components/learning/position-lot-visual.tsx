import { DistanceBar, Meter, type MeterVariant } from "#/components/ui";
import { cn } from "#/lib/utils";

type Tone = "up" | "down" | "warn" | "muted";

type WordPair = { short: string; title: string; variant: MeterVariant };

const toneBg: Record<Tone, string> = {
	up: "bg-(--up)/15 border-(--up)/40 text-(--up)",
	down: "bg-(--down)/15 border-(--down)/40 text-(--down)",
	warn: "bg-(--warn)/15 border-(--warn)/40 text-(--warn)",
	muted: "bg-(--sunken) border-(--line) text-(--f3)",
};

const closeVerdict = (
	pnlPct: number | undefined,
	_room: number | undefined,
): { text: string; tone: Tone } => {
	if (pnlPct === undefined) {
		return { text: "Waiting for desk book price", tone: "muted" };
	}

	if (pnlPct >= 0.02) {
		return { text: "Close now: ahead after fees", tone: "up" };
	}

	if (pnlPct <= -0.02) {
		return { text: "Close now: behind after fees", tone: "down" };
	}

	return { text: "Close now: roughly flat after fees", tone: "warn" };
};

const markMoveFrac = (entry?: number, mark?: number) => {
	if (entry === undefined || mark === undefined || entry <= 0) return undefined;
	return (mark - entry) / entry;
};

const bookHaircutWords = (
	pnlPct: number | undefined,
	entry?: number,
	mark?: number,
): WordPair => {
	const move = markMoveFrac(entry, mark);
	if (pnlPct === undefined || move === undefined) {
		return {
			short: "—",
			title: "Waiting for mark and close-now price",
			variant: "disabled",
		};
	}

	const drag = pnlPct - move;
	const dragPp = Math.abs(drag) * 100;

	if (Math.abs(drag) < 0.003) {
		return {
			short: "Aligned",
			title: "Close-now matches the mark move; fees are already in the P&L gauge",
			variant: "success",
		};
	}

	if (move >= 0.005 && pnlPct < -0.005) {
		return {
			short: "Hidden loss",
			title: `Mark looks up, but closing is still losing — book/fees drag ~${dragPp.toFixed(1)} pts`,
			variant: "error",
		};
	}

	if (move <= -0.005 && pnlPct > 0.005) {
		return {
			short: "Book helps",
			title: `Mark looks down, but closing still wins — book/fees help ~${dragPp.toFixed(1)} pts`,
			variant: "success",
		};
	}

	if (drag < 0) {
		return {
			short: `${dragPp.toFixed(1)}pt drag`,
			title: `Selling into the book costs ~${dragPp.toFixed(1)} pts versus the headline mark`,
			variant: "warning",
		};
	}

	return {
		short: "Book boost",
		title: `Exit book is ~${dragPp.toFixed(1)} pts better than mark alone suggests`,
		variant: "success",
	};
};

const bookHaircutHeat = (
	pnlPct: number | undefined,
	entry?: number,
	mark?: number,
) => {
	const move = markMoveFrac(entry, mark);
	if (pnlPct === undefined || move === undefined) return 0;
	const drag = pnlPct - move;
	return Math.max(0, Math.min(100, 50 + (drag / 0.08) * 50));
};

const exitVsMarkWords = (mark?: number, exitVwap?: number): WordPair => {
	if (mark === undefined || exitVwap === undefined || mark <= 0) {
		return {
			short: "—",
			title: "No full-size exit price yet",
			variant: "disabled",
		};
	}

	const rel = (exitVwap - mark) / mark;
	if (rel <= -0.02) {
		return {
			short: "Heavy slip",
			title: "Full-size exit is well below the mark (slippage)",
			variant: "error",
		};
	}
	if (rel <= -0.005) {
		return {
			short: "Some slip",
			title: "Full-size exit is a little below the mark",
			variant: "warning",
		};
	}
	return {
		short: "At mark",
		title: "Full-size exit matches or beats the headline mark",
		variant: "success",
	};
};

const exitVsMarkHeat = (mark?: number, exitVwap?: number) => {
	if (mark === undefined || exitVwap === undefined || mark <= 0) return 0;
	const rel = Math.max(-0.05, Math.min(0.05, (exitVwap - mark) / mark));
	return ((rel + 0.05) / 0.1) * 100;
};

export type PositionLotVisualProps = {
	pnlPct?: number;
	exitRoom?: number;
	entry?: number;
	mark?: number;
	exitVwap?: number;
	className?: string;
};

/*
Extra readouts the top gauges do not show: mark-vs-close drag and where a
full-size sell lands versus the headline mark.
*/
export const PositionLotVisual = ({
	pnlPct,
	exitRoom,
	entry,
	mark,
	exitVwap,
	className,
}: PositionLotVisualProps) => {
	const verdict = closeVerdict(pnlPct, exitRoom);
	const haircut = bookHaircutWords(pnlPct, entry, mark);
	const exitGap = exitVsMarkWords(mark, exitVwap);

	return (
		<div className={cn("flex flex-col gap-2 min-w-0", className)}>
			<div
				className={cn(
					"rounded border px-2 py-1.5 text-[10px] font-medium leading-tight line-clamp-2",
					toneBg[verdict.tone],
				)}
				title={verdict.text}
			>
				{verdict.text}
			</div>

			<Meter
				layout="stacked"
				size="xs"
				label="Book drag"
				value={haircut.short}
				percent={bookHaircutHeat(pnlPct, entry, mark)}
				variant={haircut.variant}
				animated
				title={haircut.title}
				valueClassName="truncate max-w-[45%] text-right"
			/>
			<Meter
				layout="stacked"
				size="xs"
				label="Exit slip"
				value={exitGap.short}
				percent={exitVsMarkHeat(mark, exitVwap)}
				variant={exitGap.variant}
				animated
				title={exitGap.title}
				valueClassName="truncate max-w-[45%] text-right"
			/>

			<div title="Green fill is mark vs entry only — not profit after fees. Blue tick is full-size exit VWAP.">
				<div className="text-[9px] text-(--f4) mb-0.5">
					Mark vs entry · blue = exit VWAP
				</div>
				<DistanceBar
					origin={entry}
					value={mark}
					markers={
						exitVwap === undefined
							? []
							: [
									{
										value: exitVwap,
										label: "average price if you sold the full lot now",
										color: "var(--info)",
									},
								]
					}
					leftLabel="below entry"
					rightLabel="above entry"
				/>
			</div>
		</div>
	);
};
