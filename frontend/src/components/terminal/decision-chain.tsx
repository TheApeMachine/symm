import { useSelector } from "@tanstack/react-store";
import { type MouseEvent, useEffect, useRef } from "react";
import { decisionStore } from "#/collections/app";
import {
	ExecutionStage,
	PrecursorStage,
	ReadinessStage,
} from "#/components/terminal/decision-chain-stages";
import { setDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { WarRoom } from "#/components/terminal/war-room";
import { Typography } from "#/components/ui/typography";

const selectRow = (row: HTMLElement, symbol: string): void => {
	setDecisionsScopeSymbol(symbol);

	for (const other of document.querySelectorAll<HTMLElement>(
		"[data-decision-chain='row']",
	)) {
		const selected = other === row;
		other.dataset.selected = String(selected);
		other.setAttribute("aria-expanded", String(selected));
	}
};

/*
text coerces a flatbuffer string field. The generated DecisionT types a string
as string | Uint8Array because the reader can return raw bytes; every field
read here is a real string, and this states that once rather than at each use.
*/
const text = (value: string | Uint8Array | null | undefined): string =>
	typeof value === "string" ? value : "";

export const DecisionChain = ({ symbol }: { symbol: string }) => {
	const rowRef = useRef<HTMLButtonElement>(null);

	/*
		The row is addressed by symbol, so it repaints from its own symbol's
		latest decision and from nothing else.

		It used to be pinned to a position in a 50-frame ring. A ring rotates:
		once full, every new frame shifted all positions by one, so the pinned
		index silently came to name a different frame on every tick. Rows drifted
		onto foreign data and the board reshuffled under an open row. Keying by
		symbol removes that failure rather than guarding against it — there is no
		position left to go stale.
	*/
	const decision = useSelector(
		decisionStore,
		(state) => state.bySymbol[symbol],
	);

	useEffect(() => {
		const row = rowRef.current;

		if (!row || !decision) return;

		const set = (field: string, value: string) => {
			const element = row.querySelector<HTMLElement>(`[data-df="${field}"]`);

			if (element) element.textContent = value;
		};

		set("symbol", text(decision.symbol));
		set("reason", text(decision.reason));
		set("confidence", `${(decision.confidence * 100).toFixed(1)}%`);
		set("action", text(decision.action) || "—");
		set("cause", text(decision.reason) || "pending");
	}, [decision]);

	if (!decision) {
		return null;
	}

	const selectDecision = (event: MouseEvent<HTMLButtonElement>): void => {
		selectRow(event.currentTarget, text(decision.symbol));
	};

	// Seed the paint targets from the frame already in hand. The subscription
	// above repaints them on every later frame, but until one arrives the row
	// would otherwise render as an empty outline.
	const seed = {
		symbol: text(decision.symbol),
		reason: text(decision.reason),
		confidence: `${(decision.confidence * 100).toFixed(1)}%`,
		action: text(decision.action) || "—",
	};

	return (
		<button
			ref={rowRef}
			type="button"
			data-decision-chain="row"
			data-selected="false"
			aria-expanded="false"
			onClick={selectDecision}
			className="group w-full cursor-pointer overflow-hidden rounded border border-(--line) bg-(--surface) text-left font-[inherit] transition-colors data-[selected=true]:border-[color-mix(in_srgb,var(--acc)_45%,transparent)]"
		>
			<div className="flex items-start justify-between gap-3 border-(--line) border-b px-3 py-2">
				<div className="min-w-0">
					<Typography.Span
						data-df="symbol"
						data-decision-chain="symbol"
						className="font-mono font-semibold text-[13px] text-(--f1)"
					>
						{seed.symbol}
					</Typography.Span>
					<Typography.Span
						data-df="reason"
						className="mt-0.5 block truncate font-mono text-[9px] text-(--f4)"
					>
						{seed.reason}
					</Typography.Span>
				</div>
				<div className="flex shrink-0 items-center gap-2 font-mono">
					<span className="text-[9px] text-(--f4)">
						conf=
						<b data-df="confidence" className="font-normal text-(--acc)">
							{seed.confidence}
						</b>
					</span>
					<Typography.Span
						data-df="action"
						className="rounded-[3px] border border-(--line) px-2 py-0.75 font-semibold text-[10px] uppercase"
					>
						{seed.action}
					</Typography.Span>
				</div>
			</div>

			<div className="hidden border-(--line) border-b px-3 py-2 font-mono text-[9px] text-(--f4) group-data-[selected=true]:block">
				<span className="text-(--f3)">verdict: </span>
				<span data-df="cause" className="text-(--f2)" />
				<span> · </span>
				<span data-df="reason" />
			</div>

			{/*
				The measured stages stay a compact row; the War Room gets the full
				width beneath them. A search tree squeezed into a fifth of the row
				is unreadable, and the reasoning is the reason the row was opened.
			*/}
			<div className="hidden grid-cols-3 gap-1.5 p-2 font-mono text-[8.5px] group-data-[selected=true]:grid">
				<PrecursorStage decision={decision} />
				<ReadinessStage decision={decision} />
				<ExecutionStage decision={decision} />
			</div>

			<div className="hidden border-(--line) border-t group-data-[selected=true]:block">
				<WarRoom symbol={text(decision.symbol)} className="h-112" />
			</div>

			<div className="hidden items-center gap-4 border-(--line) border-t px-3 py-1.5 font-mono text-[8.5px] text-(--f4) group-data-[selected=true]:flex">
				<span>selected root</span>
				<span
					data-df="recommended"
					className="max-w-80 truncate text-(--acc)"
				/>
				<span className="ml-auto">
					round <b data-df="round" className="font-normal text-(--f2)" />
				</span>
			</div>
		</button>
	);
};
