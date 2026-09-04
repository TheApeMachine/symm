import { useSelector } from "@tanstack/react-store";
import { terminalStore } from "#/collections/terminal";
import { EntryDecisionSnapshot } from "#/components/terminal/entry-decision-snapshot";
import { ThesisDetailRail } from "#/components/terminal/thesis-detail-rail";
import { useTrace, WarRoom } from "#/components/terminal/war-room";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

/*
ThesisModal separates two different truths about one open lot:

  the immutable decision and market economics recorded at entry;
  the position's live mark, return, and protection state now.

The entry side always comes from Position.Decision. It never consults the
current strategy round, so opening the modal later cannot rewrite why the trade
was taken.
*/
export const openThesisShell = (symbol: string) => {
	terminalStore.actions.openThesis(symbol);
};

export const closeThesisShell = () => {
	terminalStore.actions.closeThesis();
};

export const ThesisModal = () => {
	const symbol = useSelector(terminalStore, (state) => state.thesisSymbol);
	const { isLive } = useTrace(symbol ?? "");

	if (symbol === null || symbol === "") {
		return null;
	}

	return (
		<div className="absolute inset-0 z-20 flex animate-[symFade_0.18s_ease] items-center justify-center bg-[color-mix(in_srgb,var(--sunken)_52%,transparent)] p-6 backdrop-blur-sm">
			<button
				type="button"
				aria-label="Close position snapshot"
				className="absolute inset-0 cursor-default"
				onClick={closeThesisShell}
			/>
			<div
				className={cn(
					"pointer-events-auto relative z-10 flex h-[min(90vh,960px)] w-[min(1320px,96vw)] flex-col overflow-hidden",
					"rounded-lg border border-(--line2) bg-(--surface) shadow-[0_28px_72px_-18px_rgba(0,0,0,0.78)]",
				)}
			>
				<div className="flex shrink-0 items-center justify-between gap-3 border-(--line) border-b px-5 py-3.5">
					<div className="flex items-center gap-3">
						<Typography.Display size="lg">{symbol}</Typography.Display>
						<span className="rounded border border-(--line2) bg-(--sunken) px-2 py-1 font-mono text-[9px] text-(--acc) uppercase tracking-wide">
							Entry decision snapshot
						</span>
					</div>

					<button
						type="button"
						onClick={closeThesisShell}
						className="flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-[3px] border border-(--line) bg-(--raised) p-0 text-(--f3) hover:border-(--line2) hover:text-(--f1)"
					>
						<svg
							width="13"
							height="13"
							viewBox="0 0 24 24"
							fill="none"
							stroke="currentColor"
							strokeWidth="2"
							aria-hidden="true"
						>
							<title>Close</title>
							<path d="M6 6l12 12M18 6L6 18" />
						</svg>
					</button>
				</div>

				<div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(300px,360px)]">
					<div className="flex min-h-0 flex-col border-(--line) border-r">
						<div className="min-h-0 flex-1 overflow-y-auto">
							<EntryDecisionSnapshot symbol={symbol} />
						</div>

						{/*
							The entry snapshot is why the lot was opened, frozen at
							entry. The War Room below is what the search is concluding
							about this symbol now — or the entry search trace if no
							new round ran for this held asset.
						*/}
						<div className="flex h-104 shrink-0 flex-col border-(--line) border-t">
							<div className="flex shrink-0 items-center justify-between border-(--line) border-b px-3 py-1.5">
								<span className="font-mono text-[9px] text-(--acc) uppercase tracking-wide">
									{isLive
										? "Live reasoning · current round"
										: "Entry reasoning · frozen decision"}
								</span>
								<span className="font-mono text-[8px] text-(--f4)">
									{isLive
										? "simulating live market horizon"
										: "snapshot at position execution"}
								</span>
							</div>
							<WarRoom symbol={symbol} className="min-h-0 flex-1" />
						</div>
					</div>

					<div className="min-h-0 overflow-y-auto p-3.5">
						<ThesisDetailRail symbol={symbol} />
					</div>
				</div>
			</div>
		</div>
	);
};
