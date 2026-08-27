import { useEffect, useRef } from "react";
import { positionStore } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import type { PositionsFrame } from "#/providers/telemetry/telemetry/positions-frame";
import { Holding } from "#/providers/telemetry/telemetry/holding";
import { Position } from "#/providers/telemetry/telemetry/position";
import { Stoploss } from "#/providers/telemetry/telemetry/stoploss";

const fmt = (value: unknown, digits: number): string => {
	if (typeof value === "number") {
		return value.toFixed(digits);
	}

	if (typeof value === "string" && value !== "" && Number.isFinite(Number(value))) {
		return Number(value).toFixed(digits);
	}

	return String(value ?? "—");
};

const positionInstance = new Position();
const holdingInstance = new Holding();
const stoplossInstance = new Stoploss();

/*
Floor and Peak bound the live stop interval, mapped onto the card's own domain.
*/
export const PositionStopGeometry = ({ symbol }: { symbol: string }) => {
	const root = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const updateElements = (lastPositionsFrame?: PositionsFrame | null) => {
			if (!root.current || !lastPositionsFrame) return;

			let targetHolding: Holding | null = null;

			for (let index = 0; index < lastPositionsFrame.rowsLength(); index++) {
				const currentPosition = lastPositionsFrame.rows(index, positionInstance);
				if (!currentPosition) continue;

				const currentHolding = currentPosition.holding(holdingInstance);
				if (currentHolding && currentHolding.symbol() === symbol) {
					targetHolding = currentHolding;
					break;
				}
			}

			if (!targetHolding) return;

			const currentStoploss = targetHolding.stoploss(stoplossInstance);

			const floorText = currentStoploss?.floor();
			const peakText = currentStoploss?.peak();
			const markText = currentStoploss?.mark() ?? targetHolding.mark();
			const profitText = currentStoploss?.profitLine();
			const armText = currentStoploss?.armAt();
			const lockText = currentStoploss?.lockFloor();

			const floorValue = floorText ? Number(floorText) : Number.NaN;
			const peakValue = peakText ? Number(peakText) : Number.NaN;
			const markValue = markText ? Number(markText) : Number.NaN;

			const setText = (field: string, textContent: string) => {
				const node = root.current?.querySelector<HTMLElement>(`[data-f="${field}"]`);
				if (node) node.textContent = textContent;
			};

			const setMarker = (markerField: string, priceValue: number, minFloor: number, maxPeak: number) => {
				const markerNode = root.current?.querySelector<HTMLElement>(`[data-f="${markerField}"]`);
				if (!markerNode) return;

				if (Number.isFinite(priceValue) && Number.isFinite(minFloor) && Number.isFinite(maxPeak) && maxPeak > minFloor) {
					const positionPercent = Math.min(Math.max(((priceValue - minFloor) / (maxPeak - minFloor)) * 100, 0), 100);
					markerNode.style.left = `${positionPercent}%`;
					markerNode.style.display = "block";
					return;
				}

				markerNode.style.display = "none";
			};

			setText("floor", fmt(floorText, 6));
			setText("peak", fmt(peakText, 6));
			setText("profit", fmt(profitText, 6));
			setText("arm", fmt(armText, 6));
			setText("lock", fmt(lockText, 6));
			setText("surge", String(currentStoploss?.surgeArmed() ?? false));
			setText("momentum", fmt(currentStoploss?.momentumFloor(), 6));
			setText("lastmove", fmt(currentStoploss?.lastMove(), 6));
			setText("trigger", currentStoploss?.triggerReason() ?? "—");
			setText("locked", String(currentStoploss?.locked() ?? false));
			setText("threshold", fmt(targetHolding.profitThreshold(), 6));
			setText("stopstatus", currentStoploss?.status() ?? "—");

			const indicatorNode = root.current.querySelector<HTMLElement>('[data-f="indicator"]');
			if (indicatorNode) {
				if (Number.isFinite(floorValue) && Number.isFinite(peakValue) && peakValue > floorValue && Number.isFinite(markValue)) {
					const markPercent = Math.min(Math.max(((markValue - floorValue) / (peakValue - floorValue)) * 100, 0), 100);
					indicatorNode.style.left = `${markPercent}%`;
					indicatorNode.style.display = "block";
				} else if (Number.isFinite(floorValue) && Number.isFinite(markValue)) {
					indicatorNode.style.left = markValue >= floorValue ? "100%" : "0%";
					indicatorNode.style.display = "block";
				}
			}

			if (Number.isFinite(floorValue) && Number.isFinite(peakValue) && peakValue > floorValue) {
				if (profitText) setMarker("profit-marker", Number(profitText), floorValue, peakValue);
				if (armText) setMarker("arm-marker", Number(armText), floorValue, peakValue);
				if (lockText) setMarker("lock-marker", Number(lockText), floorValue, peakValue);
			}
		};

		updateElements(positionStore.state.getLast());
		const subscription = positionStore.subscribe((state) => {
			updateElements(state.getLast());
		});

		return () => {
			subscription?.unsubscribe();
		};
	}, [symbol]);

	return (
		<div ref={root}>
			<div className="relative mt-2 h-1.5 overflow-visible rounded-full bg-[linear-gradient(90deg,color-mix(in_srgb,var(--down)_25%,transparent),color-mix(in_srgb,var(--warn)_25%,transparent)_40%,color-mix(in_srgb,var(--up)_25%,transparent))]">
				<div
					data-f="profit-marker"
					className="absolute -top-0.5 h-2.5 w-0.5 -translate-x-1/2 rounded-xs bg-(--info) opacity-70"
					style={{ left: "0%", display: "none" }}
					title="Profit line"
				/>
				<div
					data-f="arm-marker"
					className="absolute -top-0.5 h-2.5 w-0.5 -translate-x-1/2 rounded-xs bg-(--warn) opacity-70"
					style={{ left: "0%", display: "none" }}
					title="Arm threshold"
				/>
				<div
					data-f="lock-marker"
					className="absolute -top-0.5 h-2.5 w-0.5 -translate-x-1/2 rounded-xs bg-(--up) opacity-70"
					style={{ left: "0%", display: "none" }}
					title="Lock floor"
				/>
				<div
					data-f="indicator"
					data-stoploss-indicator
					className="absolute -top-1 h-3.5 w-1.5 -translate-x-1/2 rounded-full bg-(--f1) shadow-[0_0_6px_var(--acc)] ring-1 ring-(--acc) transition-[left] duration-100"
					style={{ left: "50%" }}
					title="Live mark stoploss indicator"
				/>
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


