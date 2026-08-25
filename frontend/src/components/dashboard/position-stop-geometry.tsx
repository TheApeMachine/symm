import { Typography } from "#/components/ui/typography";
import { Flex } from "@/components/ui/flex";
import { positionsStore, useSubscribe } from "#/providers/ws-stores";

const fmt = (value: unknown, digits: number): string =>
	typeof value === "number" ? value.toFixed(digits) : String(value ?? "—");

/*
Floor and Peak bound the live stop interval, mapped onto the card's own domain.
*/
export const PositionStopGeometry = ({ symbol }: { symbol: string }) => {
	const root = useSubscribe(positionsStore, (state) => {
		const position = state.positions[symbol]?.latest();

		if (position === undefined) {
			return;
		}

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("floor", fmt(position.holding.stoploss?.floor, 6));
		set("peak", fmt(position.holding.stoploss?.peak, 6));
		set("profit", fmt(position.holding.stoploss?.profit_line, 6));
		set("arm", fmt(position.holding.stoploss?.arm_at, 6));
		set("lock", fmt(position.holding.stoploss?.lock_floor, 6));
		set("surge", String(position.holding.stoploss?.surge_armed ?? false));
		set("momentum", fmt(position.holding.stoploss?.momentum_floor, 6));
		set("lastmove", fmt(position.holding.stoploss?.last_move, 6));
		set("trigger", position.holding.stoploss?.trigger_reason ?? "—");
		set("locked", String(position.holding.stoploss?.locked ?? false));
		set("threshold", fmt(position.holding.profit_threshold, 6));
		set("stopstatus", position.holding.stoploss?.status ?? "—");
	});

	return (
		<div ref={root}>
			<div className="relative mt-2 h-1 overflow-visible rounded-full bg-[linear-gradient(90deg,color-mix(in_srgb,var(--down)_12%,transparent),color-mix(in_srgb,var(--f4)_18%,transparent)_42%,color-mix(in_srgb,var(--up)_12%,transparent))]">
				<div className="pointer-events-none absolute top-1/2 left-1/2 h-3.5 w-[2px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-(--acc)" />
				<div className="pointer-events-none absolute top-1/2 left-2/3 h-3 w-px -translate-x-1/2 -translate-y-1/2 bg-(--up)" />
			</div>

			<Flex.Row className="mt-1.25 items-center justify-between gap-2 text-[8.5px]">
				<Typography.Span className="text-(--acc)">
					floor <span data-f="floor" />
				</Typography.Span>
				<Typography.Span className="text-(--up)">
					peak <span data-f="peak" />
				</Typography.Span>
			</Flex.Row>

			<div className="mt-1 grid grid-cols-3 gap-x-2 gap-y-0.5 border-(--line) border-t pt-1 text-[8px] text-(--f4)">
				<span>profit <b className="font-normal text-(--info)" data-f="profit" /></span>
				<span>arm <b className="font-normal text-(--warn)" data-f="arm" /></span>
				<span>lock <b className="font-normal text-(--up)" data-f="lock" /></span>
			</div>

			<div className="mt-1 grid grid-cols-2 gap-x-2 gap-y-0.5 border-(--line) border-t pt-1 text-[8px] text-(--f4)">
				<span>surge <b className="font-normal" data-f="surge" /></span>
				<span>momentum floor <b className="font-normal text-(--warn)" data-f="momentum" /></span>
				<span>last move <b className="font-normal text-(--f3)" data-f="lastmove" /></span>
				<span className="min-w-0 truncate text-right">trigger <b className="font-normal text-(--down)" data-f="trigger" /></span>
			</div>

			<Flex.Row className="mt-1 items-center justify-between gap-2 text-[8px] text-(--f4)">
				<span>locked <b className="font-normal" data-f="locked" /></span>
				<span>threshold <b className="font-normal text-(--f3)" data-f="threshold" /></span>
				<b className="font-normal uppercase" data-f="stopstatus" />
			</Flex.Row>
		</div>
	);
};
