import { cn } from "@/lib/utils";
import { Flex } from "./flex";
import { List } from "./list";
import { Typography } from "./typography";

/*
fixed writes a decimal the producer sent as text at a reading precision; text
that is not a plain decimal is shown as sent.
*/
const fixed = (value: string | number, digits: number) =>
	/^-?\d+(\.\d+)?$/.test(String(value))
		? Number(value).toFixed(digits)
		: String(value);

/*
OpenPosition is one lot as a reader sees it. Amounts arrive as the exact
decimal text the producer settled them at; the list writes them at reading
precision and leaves any other text as sent.
*/
export type OpenPosition = {
	symbol: string;
	status: string;
	pnl: string;
	pnlValue: number;
	entryPrice: string;
	mark: string;
	returnPct: string;
};

export type PositionListProps = {
	className?: string;
	positions?: OpenPosition[];
	/* Symbols whose exit has been asked for and not yet taken effect. */
	exiting?: readonly string[];
	onInspect?: (symbol: string) => void;
	onExit?: (symbol: string) => void;
	emptyLabel?: string;
};

const toneOf = (value: number) => {
	if (value > 0) {
		return "text-(--up)";
	}

	if (value < 0) {
		return "text-(--down)";
	}

	return "text-(--f2)";
};

/*
PositionList draws the lots that are currently open.

It holds no positions of its own and reaches for nothing: the lots, which of
them are on their way out, and what to do when one is clicked all arrive as
props. That is what lets the same list be drawn by a React surface that reads
the desk directly and by a graph-authored one that is handed the same rows.
*/
export const PositionList = ({
	className,
	positions = [],
	exiting = [],
	onInspect,
	onExit,
	emptyLabel = "no open positions",
}: PositionListProps) => {
	const pending = new Set(exiting);

	if (positions.length === 0) {
		return (
			<List className={cn("min-h-0 flex-1 p-1.5", className)}>
				<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
					{emptyLabel}
				</div>
			</List>
		);
	}

	return (
		<List className={cn("min-h-0 flex-1 p-1.5", className)}>
			{positions.map((position) => {
				const tone = toneOf(position.pnlValue);
				const leaving = pending.has(position.symbol);

				return (
					// biome-ignore lint/a11y/useSemanticElements: a <button> can't legally nest the EXIT <button>.
					<div
						role="button"
						tabIndex={0}
						data-pos={position.symbol}
						data-position-card
						key={position.symbol}
						onClick={() => onInspect?.(position.symbol)}
						onKeyDown={(event) => {
							if (event.key === "Enter" || event.key === " ") {
								event.preventDefault();
								onInspect?.(position.symbol);
							}
						}}
						title="Inspect this lot"
						className="mb-1.25 block w-full cursor-pointer rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 text-left font-mono text-[11px] transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)]"
					>
						<Flex.Column className="gap-0">
							<Flex.Row className="items-center justify-between gap-2">
								<Flex.Row className="min-w-0 items-center gap-1.5">
									<Typography.Span className="font-semibold text-[11.5px] text-(--f1)">
										{position.symbol}
									</Typography.Span>
									<Typography.Span className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase tracking-wide">
										{position.status}
									</Typography.Span>
								</Flex.Row>
								<Flex.Row className="items-center gap-1.5">
									<Typography.Span
										className={cn(
											"text-right font-semibold text-[11.5px]",
											tone,
										)}
									>
										{fixed(position.pnl, 4)}
									</Typography.Span>
									{onExit ? (
										<button
											type="button"
											disabled={leaving}
											onClick={(event) => {
												event.preventDefault();
												event.stopPropagation();
												onExit(position.symbol);
											}}
											title="Exit this position immediately"
											className="rounded-xs border border-(--down) px-1.5 py-px font-semibold text-[8px] text-(--down) uppercase tracking-wide hover:bg-[color-mix(in_srgb,var(--down)_12%,transparent)] disabled:cursor-wait disabled:opacity-60"
										>
											{leaving ? "EXITING" : "EXIT"}
										</button>
									) : null}
								</Flex.Row>
							</Flex.Row>

							<Flex.Row className="mt-0.75 items-center justify-between gap-3 text-[9.5px] text-(--f4)">
								<Typography.Span>
									entry {fixed(position.entryPrice, 2)} / mark{" "}
									{fixed(position.mark, 2)}
								</Typography.Span>
								<Typography.Span className={cn(tone)}>
									{fixed(position.returnPct, 2)}
									{/^-?[\d.]+$/.test(String(position.returnPct)) ? "%" : ""}
								</Typography.Span>
							</Flex.Row>
						</Flex.Column>
					</div>
				);
			})}
		</List>
	);
};
