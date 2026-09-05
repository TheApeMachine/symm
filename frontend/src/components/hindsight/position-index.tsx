import { useMemo, useState } from "react";
import { Button } from "#/components/ui/button";
import { Input } from "#/components/ui/input";
import { Section } from "#/components/ui/section";
import type { Position } from "./positions";
import { formatCount } from "./timeline-scale";

/*
The desk's own record for this run: every position it actually held.

This is the index the tape was missing. Positions were drawn on the timeline,
but only for whichever instrument was already selected, so finding one meant
knowing in advance which of hundreds of instruments to look at. A run that held
a single position showed nothing at all, and read as a run that traded nothing.

Every value shown is a venue fact taken from the recorded fills. The realised
change is between the two fill prices and is not net of fees; fees are stated
separately because the record states them separately.
*/

const formatPrice = (value: number | null): string =>
	value === null ? "—" : value.toPrecision(6);

const formatChange = (value: number | null): string =>
	value === null ? "—" : `${value >= 0 ? "+" : ""}${(value * 100).toFixed(2)}%`;

/*
changeTone colours only the sign of a realised change, which is a fact about
the record. An open position has no realised change to judge.
*/
const changeTone = (value: number | null): string => {
	if (value === null) return "text-(--f4)";

	return value >= 0 ? "text-(--up)" : "text-(--down)";
};

const instantOf = (position: Position): number => {
	const at = position.entry?.at ?? position.openedAt ?? "";
	const value = new Date(at).getTime();

	return Number.isNaN(value) ? 0 : value;
};

/*
Seek is one edge of a position as a tape destination.

An edge with no recorded frame is shown disabled rather than hidden, so the
record states which edge it cannot take you to. A position closed before the
recorder stamped exits has no close frame, and saying so is the point.
*/
const Seek = ({
	label,
	sequence,
	onSeek,
}: {
	label: string;
	sequence: number | null;
	onSeek: () => void;
}) => {
	if (sequence === null) {
		return (
			<span
				title={`No frame was recorded for this position's ${label}, so the tape cannot be seeked to it.`}
				className="rounded-[2px] border border-(--line) px-1 py-0.5 text-(--f4) opacity-50"
			>
				{label} —
			</span>
		);
	}

	return (
		<Button
			variant="bare"
			size="xs"
			title={`Seek the tape to capture ${sequence}, the frame this position's ${label} was recorded at.`}
			className="rounded-[2px] border border-(--line) px-1 py-0.5 text-(--f3) hover:border-(--acc) hover:text-(--f1)"
			onClick={(clicked) => {
				clicked.stopPropagation();
				onSeek();
			}}
		>
			{label} #{sequence}
		</Button>
	);
};

export const PositionIndex = ({
	positions,
	selected,
	onSelect,
	onSeek,
}: {
	positions: Position[];
	selected: string | null;
	onSelect: (position: Position) => void;
	onSeek: (position: Position, edge: "entry" | "exit") => void;
}) => {
	const [filter, setFilter] = useState("");
	const [onlyOpen, setOnlyOpen] = useState(false);

	const rows = useMemo(() => {
		const needle = filter.trim().toUpperCase();

		return positions
			.filter((position) => {
				if (needle !== "" && !position.symbol.toUpperCase().includes(needle)) {
					return false;
				}

				return onlyOpen ? position.open : true;
			})
			.sort((left, right) => instantOf(right) - instantOf(left));
	}, [positions, filter, onlyOpen]);

	const openCount = positions.filter((position) => position.open).length;

	return (
		<Section fit="pane" surface="surface" className="min-h-0 shrink-0">
			<Section.Header
				title="Positions held"
				size="m"
				rule
				sticky
				meta={
					<span className="font-mono text-[9px] text-(--f4)">
						{formatCount(rows.length)}/{formatCount(positions.length)}
						{openCount > 0 ? ` · ${formatCount(openCount)} open` : ""}
					</span>
				}
			/>

			{positions.length === 0 ? (
				<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
					The desk held no position on this run.
				</p>
			) : (
				<>
					<div className="shrink-0 border-(--line) border-b px-2.5 py-2">
						<Input
							value={filter}
							placeholder="filter instrument"
							className="w-full font-mono text-[10px]"
							onChange={(event) => setFilter(event.currentTarget.value)}
						/>
						<Button
							variant="bare"
							size="xs"
							title="Show only positions that were still open at the end of the recorded tape."
							className="mt-1.5 w-full justify-start gap-1 whitespace-nowrap px-0 font-mono text-[9px] text-(--f4) hover:text-(--f2)"
							onClick={() => setOnlyOpen((current) => !current)}
						>
							<span className={onlyOpen ? "text-(--acc)" : ""}>
								{onlyOpen ? "▣" : "▢"}
							</span>
							only still open
						</Button>
					</div>

					<Section.Body>
						<ul className="flex max-h-56 flex-col divide-y divide-(--line) overflow-y-auto">
							{rows.map((position) => {
								const active = position.decisionId === selected;

								return (
									<li key={position.decisionId}>
										<Button
											variant="bare"
											size="xs"
											title={
												position.entrySeq === null
													? "No decision witness recorded the frame this position was opened on, so the tape cannot be seeked to it."
													: `Seek the tape to capture ${position.entrySeq}, the frame this position was opened on.`
											}
											className={`w-full flex-col items-stretch gap-0.5 px-2.5 py-1.5 text-left ${
												active ? "bg-(--sunken)" : ""
											}`}
											onClick={() => onSelect(position)}
										>
											<span className="flex items-baseline justify-between gap-2">
												<span className="truncate font-mono text-[10px] text-(--f1)">
													{position.symbol}
												</span>
												<span
													className={`shrink-0 font-mono text-[10px] ${changeTone(
														position.realisedPriceChange,
													)}`}
												>
													{position.open
														? "open"
														: formatChange(position.realisedPriceChange)}
												</span>
											</span>
											<span className="flex items-baseline justify-between gap-2 font-mono text-[8px] text-(--f4)">
												<span className="truncate">
													{formatPrice(position.entry?.price ?? null)}
													{position.exit === null
														? ""
														: ` → ${formatPrice(position.exit.price)}`}
												</span>
											</span>
										</Button>

										<span className="flex items-center gap-1 px-2.5 pb-1.5 font-mono text-[8px]">
											<Seek
												label="open"
												sequence={position.entrySeq}
												onSeek={() => onSeek(position, "entry")}
											/>
											<Seek
												label="close"
												sequence={position.exitSeq}
												onSeek={() => onSeek(position, "exit")}
											/>
										</span>
									</li>
								);
							})}
						</ul>
					</Section.Body>
				</>
			)}
		</Section>
	);
};
