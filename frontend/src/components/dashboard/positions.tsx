
import type { KeyboardEvent, MouseEvent } from "react";
import { terminalStore } from "#/collections/terminal";
import { Component } from "#/components/ui/component";
import { List } from "#/components/ui/list";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";
import { publishPositionExit } from "#/providers/websocket";
import { Flex } from "@/components/ui/flex";
import { PositionStopGeometry } from "./position-stop-geometry";

/*
Positions is the open-lot list.

Each lot exposes the regulator as a price map: hard loss, break-even, minimum
profit, arming, active protection, mark, and peak all share one derived domain.
That makes the phase change from discovery to protected visible without
reconstructing any stop rule in the browser.

A card opens the carrier for its symbol, which is where the arbitration that
opened the lot is inspectable. The symbol comes out of the painted node rather
than out of React state: these rows are written by the websocket, so the DOM is
the only place that knows which lot a given card currently holds.
*/
const inspectPosition = (event: MouseEvent<HTMLElement>): void => {
	const symbol = event.currentTarget
		.querySelector<HTMLElement>("[data-paint='holding.symbol']")
		?.textContent?.trim();

	if (!symbol) {
		return;
	}

	terminalStore.actions.openThesis(symbol);
};

const inspectPositionKey = (event: KeyboardEvent<HTMLElement>): void => {
	if (event.key !== "Enter" && event.key !== " ") {
		return;
	}

	event.preventDefault();
	event.currentTarget.click();
};

const exitPosition = (event: MouseEvent<HTMLButtonElement>): void => {
	event.preventDefault();
	event.stopPropagation();

	const symbol = event.currentTarget
		.closest<HTMLElement>("[data-position-card]")
		?.querySelector<HTMLElement>("[data-paint='holding.symbol']")
		?.textContent?.trim();

	if (!symbol) {
		return;
	}

	const button = event.currentTarget;
	button.disabled = true;
	button.textContent = "EXITING";
	publishPositionExit(symbol);

	// A successful request removes or updates the lot through telemetry. Restore
	// the control only if the card is still present, allowing a retry after a
	// transport or venue rejection without keeping React state per painted row.
	window.setTimeout(() => {
		if (!button.isConnected) {
			return;
		}

		button.disabled = false;
		button.textContent = "EXIT";
	}, 3000);
};
export const Positions = () => (
	<Component registerKey="positions" repeat>
		{({ ref, className, slots }) => (
			<List ref={ref} className={cn("min-h-0 flex-1 p-1.5", className)}>
				{slots.map((slot) => (
					<div
						data-position-card
						data-index={slot}
						key={`${slot}-position`}
						role="button"
						tabIndex={0}
						onClick={inspectPosition}
						onKeyDown={inspectPositionKey}
						title="Inspect this lot"
						className="mb-1.25 block w-full cursor-pointer rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 text-left font-mono text-[11px] transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)]"
					>
						{/*
              The lot's own colour is set once as a custom property on the
              card, so the figures inside read from it instead of each having
              to work out the sign for themselves.
            */}
						<Flex.Column
							data-set="holding.pnl"
							data-set-scale="sign-color"
							data-target="style.--pnl"
							className="gap-0"
						>
							<Flex.Row className="items-center justify-between gap-2">
								<Flex.Row className="min-w-0 items-center gap-1.5">
									<Typography.Span
										data-paint="holding.symbol"
										className="font-semibold text-[11.5px] text-(--f1)"
									/>
									{/*
										The regulator publishes no phase of its own, so the badge
										states the lot's own lifecycle. Whether the floor has
										ratcheted past break-even is the Locked flag, and it reads
										in the stop geometry below where the prices explain it.
									*/}
									<Typography.Span
										data-paint="holding.status"
										data-paint-class="open:text-(--up) filled:text-(--up) partial_filled:text-(--warning) pending:text-(--warning) initializing:text-(--info)"
										className="rounded-xs border border-(--line) px-1 py-px text-[8px] uppercase tracking-wide"
									/>
									<Typography.Span
										data-paint="holding.stoploss.status"
										data-paint-class="initializing:text-(--info) armed:text-(--up) triggered:text-(--down)"
										className="text-[8px] uppercase text-(--f4)"
									/>
								</Flex.Row>
								<Flex.Row className="items-center gap-1.5">
									<Typography.Span
										data-paint="holding.pnl"
										data-paint-format=".4f"
										data-paint-suffix=" USD"
										className="text-right font-semibold text-[11.5px] text-(--pnl)"
									/>
									<button
										type="button"
										onClick={exitPosition}
										title="Exit this position immediately"
										className="rounded-xs border border-(--down) px-1.5 py-px text-[8px] font-semibold text-(--down) uppercase tracking-wide hover:bg-[color-mix(in_srgb,var(--down)_12%,transparent)] disabled:cursor-wait disabled:opacity-60"
									>
										EXIT
									</button>
								</Flex.Row>
							</Flex.Row>

							<Flex.Row className="mt-0.75 items-center justify-between gap-3 text-[9.5px] text-(--f4)">
								<Typography.Span>
									entry{" "}
									<span
										data-paint="holding.entry_price"
										data-paint-format=".6f"
									/>{" "}
									/ mark{" "}
									<span data-paint="holding.mark" data-paint-format=".6f" />
								</Typography.Span>
								<Typography.Span
									data-paint="holding.return_pct"
									data-paint-format=".2f"
									data-paint-suffix="%"
									className="text-(--pnl)"
								/>
							</Flex.Row>

							<PositionStopGeometry />
						</Flex.Column>
					</div>
				))}
			</List>
		)}
	</Component>
);

