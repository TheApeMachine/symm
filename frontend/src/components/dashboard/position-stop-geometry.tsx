import { useRef } from "react";
import { positionStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";
import { Stoploss } from "#/providers/telemetry/telemetry/stoploss";

const fmt = (value: unknown, digits: number): string =>
	typeof value === "number"
		? value.toFixed(digits)
		: typeof value === "string" && value !== "" && Number.isFinite(Number(value))
			? Number(value).toFixed(digits)
			: String(value ?? "—");

type QueryEntry = {
	floor: HTMLElement | null;
	peak: HTMLElement | null;
	profit: HTMLElement | null;
	arm: HTMLElement | null;
	lock: HTMLElement | null;
	surge: HTMLElement | null;
	momentum: HTMLElement | null;
	lastmove: HTMLElement | null;
	trigger: HTMLElement | null;
	locked: HTMLElement | null;
	threshold: HTMLElement | null;
	stopstatus: HTMLElement | null;
};

const queryCache: Record<string, QueryEntry> = {};
const posObj = new Position();
const holdingObj = new Holding();
const stoplossObj = new Stoploss();

/*
Floor and Peak bound the live stop interval, mapped onto the card's own domain.
*/
export const PositionStopGeometry = ({ symbol }: { symbol: string }) => {
	const root = useRef<HTMLDivElement>(null);

	positionStore.subscribe((state) => {
		if (!root.current) return;
		const last = state.getLast();
		if (!last) return;

		let targetHolding: Holding | null = null;

		for (let i = 0; i < last.rowsLength(); i++) {
			const pos = last.rows(i, posObj);
			if (!pos) continue;
			const h = pos.holding(holdingObj);
			if (h && h.symbol() === symbol) {
				targetHolding = h;
				break;
			}
		}

		if (!targetHolding) return;

		const stoploss = targetHolding.stoploss(stoplossObj);

		let element = queryCache[symbol];
		if (!element) {
			element = {
				floor: root.current.querySelector<HTMLElement>('[data-f="floor"]'),
				peak: root.current.querySelector<HTMLElement>('[data-f="peak"]'),
				profit: root.current.querySelector<HTMLElement>('[data-f="profit"]'),
				arm: root.current.querySelector<HTMLElement>('[data-f="arm"]'),
				lock: root.current.querySelector<HTMLElement>('[data-f="lock"]'),
				surge: root.current.querySelector<HTMLElement>('[data-f="surge"]'),
				momentum: root.current.querySelector<HTMLElement>('[data-f="momentum"]'),
				lastmove: root.current.querySelector<HTMLElement>('[data-f="lastmove"]'),
				trigger: root.current.querySelector<HTMLElement>('[data-f="trigger"]'),
				locked: root.current.querySelector<HTMLElement>('[data-f="locked"]'),
				threshold: root.current.querySelector<HTMLElement>('[data-f="threshold"]'),
				stopstatus: root.current.querySelector<HTMLElement>('[data-f="stopstatus"]'),
			};
			queryCache[symbol] = element;
		}

		if (element.floor) element.floor.textContent = fmt(stoploss?.floor(), 6);
		if (element.peak) element.peak.textContent = fmt(stoploss?.peak(), 6);
		if (element.profit) element.profit.textContent = fmt(stoploss?.profitLine(), 6);
		if (element.arm) element.arm.textContent = fmt(stoploss?.armAt(), 6);
		if (element.lock) element.lock.textContent = fmt(stoploss?.lockFloor(), 6);
		if (element.surge) element.surge.textContent = String(stoploss?.surgeArmed() ?? false);
		if (element.momentum) element.momentum.textContent = fmt(stoploss?.momentumFloor(), 6);
		if (element.lastmove) element.lastmove.textContent = fmt(stoploss?.lastMove(), 6);
		if (element.trigger) element.trigger.textContent = stoploss?.triggerReason() ?? "—";
		if (element.locked) element.locked.textContent = String(stoploss?.locked() ?? false);
		if (element.threshold) element.threshold.textContent = fmt(targetHolding.profitThreshold(), 6);
		if (element.stopstatus) element.stopstatus.textContent = stoploss?.status() ?? "—";
	});

	return (
		<div ref={root}>
			<div className="relative mt-2 h-1 overflow-visible rounded-full bg-[linear-gradient(90deg,color-mix(in_srgb,var(--down)_12%,transparent),color-mix(in_srgb,var(--f4)_18%,transparent)_42%,color-mix(in_srgb,var(--up)_12%,transparent))]" />

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


