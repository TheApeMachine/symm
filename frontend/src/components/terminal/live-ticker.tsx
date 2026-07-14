import { type RefObject, useRef } from "react";
import { appStore } from "#/collections/app";
import { type Balance, balancesStore } from "#/collections/balances";
import { type Position, positionsStore } from "#/collections/positions";
import { tickStore } from "#/collections/tick";
import { walletMetrics } from "#/components/terminal/panels";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";

const setText = (element: HTMLElement | null, value: string) => {
	if (element !== null) {
		element.textContent = value;
	}
};

const setTone = (element: HTMLElement | null, tone: string) => {
	if (element !== null) {
		element.style.color = tone;
	}
};

/*
LivePulseTicker paints the dashboard pulse strip directly from store snapshots so
tick cadence updates never traverse React reconciliation.
*/
export const LivePulseTicker = () => {
	const tickRef = useRef<HTMLSpanElement>(null);
	const phaseRef = useRef<HTMLSpanElement>(null);
	const measRef = useRef<HTMLSpanElement>(null);
	const candRef = useRef<HTMLSpanElement>(null);
	const openRef = useRef<HTMLSpanElement>(null);
	const quotesRef = useRef<HTMLSpanElement>(null);

	useDirectStorePaint(
		() => {
			const tick = tickStore.state.frame;
			const online = appStore.state.online;
			setText(tickRef.current, `#${String(tick?.count ?? 0)}`);
			setText(
				phaseRef.current,
				online ? String(tick?.phase ?? "stream") : "offline",
			);
			setText(measRef.current, String(tick?.measurements ?? "—"));
			setText(candRef.current, String(tick?.candidates ?? "—"));
			setText(openRef.current, String(tick?.open ?? "—"));
			setText(
				quotesRef.current,
				tick?.quotes_ready !== undefined && tick?.quotes_total !== undefined
					? `${String(tick.quotes_ready)}/${String(tick.quotes_total)}`
					: "—",
			);
		},
		[tickStore, appStore],
		[],
	);

	return (
		<div className="flex h-8 shrink-0 items-center gap-4 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)">
			<span ref={tickRef} className="font-semibold text-(--f1)" />
			<span>
				phase <span ref={phaseRef} className="text-(--acc)" />
			</span>
			<span>
				meas <span ref={measRef} />
			</span>
			<span>
				cand <span ref={candRef} />
			</span>
			<span>
				open <span ref={openRef} />
			</span>
			<span>
				quotes <span ref={quotesRef} />
			</span>
		</div>
	);
};

const setVisibility = (element: HTMLElement | null, visible: boolean) => {
	if (element !== null) {
		element.style.display = visible ? "" : "none";
	}
};

type WalletPaintRefs = {
	cash: HTMLSpanElement | null;
	available: HTMLSpanElement | null;
	reserved: HTMLSpanElement | null;
	equity: HTMLSpanElement | null;
	lambo: HTMLImageElement | null;
	tick: HTMLSpanElement | null;
	open: HTMLSpanElement | null;
};

const paintWalletMetrics = (
	refs: WalletPaintRefs,
	balances: Balance[],
	positions: Position[],
) => {
	const wallet = walletMetrics(balances, positions);
	const tick = tickStore.state.frame;
	const cashValue = wallet ? `${wallet.cash.toFixed(2)} ${wallet.asset}` : "—";
	const availableValue = wallet
		? `${wallet.available.toFixed(2)} ${wallet.asset}`
		: "—";
	const reservedValue = wallet
		? `${wallet.reserved.toFixed(2)} ${wallet.asset}`
		: "—";
	const inProfit = (wallet?.unrealized ?? 0) > 0;
	const equityValue = wallet
		? `${wallet.equity.toFixed(2)} ${wallet.asset}`
		: "—";

	setText(refs.cash, cashValue);
	setText(refs.available, availableValue);
	setText(refs.reserved, reservedValue);
	setText(refs.equity, equityValue);
	setTone(refs.equity, inProfit ? "var(--up)" : "var(--down)");
	setVisibility(refs.lambo, inProfit);
	setText(refs.tick, tick?.count !== undefined ? String(tick.count) : "…");
	setText(refs.open, String(tick?.open ?? 0));
};

/*
LiveOpenCount paints the open-position counter directly from tick snapshots.
*/
export const LiveOpenCount = () => {
	const openRef = useRef<HTMLSpanElement>(null);

	useDirectStorePaint(
		() => {
			setText(openRef.current, String(tickStore.state.frame?.open ?? 0));
		},
		[tickStore],
		[],
	);

	return (
		<span className="font-mono text-[12px] text-(--f3)">
			<span ref={openRef} /> open positions
		</span>
	);
};

/*
LiveWalletMetrics paints wallet and tick counters directly from store snapshots.
*/
export const LiveWalletMetrics = () => {
	const cashRef = useRef<HTMLSpanElement>(null);
	const availableRef = useRef<HTMLSpanElement>(null);
	const reservedRef = useRef<HTMLSpanElement>(null);
	const equityRef = useRef<HTMLSpanElement>(null);
	const lamboRef = useRef<HTMLImageElement>(null);
	const tickRef = useRef<HTMLSpanElement>(null);

	useDirectStorePaint(
		() => {
			paintWalletMetrics(
				{
					cash: cashRef.current,
					available: availableRef.current,
					reserved: reservedRef.current,
					equity: equityRef.current,
					lambo: lamboRef.current,
					tick: tickRef.current,
					open: null,
				},
				balancesStore.state.balances,
				positionsStore.state.positions,
			);
		},
		[balancesStore, positionsStore, tickStore],
		[],
	);

	return (
		<>
			<LiveTopMetric label="Cash" valueRef={cashRef} strong />
			<div className="relative flex flex-col items-end gap-px">
				<img
					ref={lamboRef}
					src="/lambo.png"
					alt=""
					aria-hidden="true"
					className="pointer-events-none absolute -top-1.5 right-0 h-11 opacity-60"
					style={{ display: "none" }}
				/>
				<span className="text-[9px] text-(--f4) uppercase tracking-widest">
					Equity
				</span>
				<span
					ref={equityRef}
					className="relative font-mono text-[12px] font-semibold"
				/>
			</div>
			<LiveTopMetric label="Available" valueRef={availableRef} />
			<LiveTopMetric label="Reserved" valueRef={reservedRef} />
			<LiveTopMetric label="Tick" valueRef={tickRef} accent />
		</>
	);
};

const LiveTopMetric = ({
	label,
	valueRef,
	accent = false,
	strong = false,
}: {
	label: string;
	valueRef: RefObject<HTMLSpanElement | null>;
	accent?: boolean;
	strong?: boolean;
}) => (
	<div className="flex flex-col items-end gap-px">
		<span className="text-[9px] text-(--f4) uppercase tracking-widest">
			{label}
		</span>
		<span
			ref={valueRef}
			className={[
				"font-mono text-[12px]",
				accent ? "font-semibold text-(--acc)" : "",
				strong && !accent ? "font-medium text-(--f1)" : "",
				!accent && !strong ? "font-medium text-(--f2)" : "",
			]
				.filter(Boolean)
				.join(" ")}
		/>
	</div>
);

/*
LiveEngineTicker paints the nav engine readout directly from tick snapshots.
*/
export const LiveEngineTicker = () => {
	const seqRef = useRef<HTMLSpanElement>(null);
	const phaseRef = useRef<HTMLSpanElement>(null);
	const candRef = useRef<HTMLSpanElement>(null);
	const openRef = useRef<HTMLSpanElement>(null);
	const quotesTextRef = useRef<HTMLSpanElement>(null);
	const quotesBarRef = useRef<HTMLDivElement>(null);
	const fluidTextRef = useRef<HTMLSpanElement>(null);
	const fluidBarRef = useRef<HTMLDivElement>(null);

	useDirectStorePaint(
		() => {
			const online = appStore.state.online;
			const tick = tickStore.state.frame;
			const storyTicks = tick?.count ?? 0;
			const enginePhase = (tick?.phase as string) ?? "";
			const candidates = (tick?.candidates as number) ?? 0;
			const open = (tick?.open as number) ?? 0;
			const fluid = (tick?.fluid as number) ?? 0;
			const quotesReady = (tick?.quotes_ready as number) ?? 0;
			const quotesTotal = (tick?.quotes_total as number) ?? 0;
			const quotesPercent =
				quotesTotal > 0 ? Math.round((quotesReady / quotesTotal) * 100) : 0;
			const fluidPercent =
				quotesTotal > 0 ? Math.round((fluid / quotesTotal) * 100) : 0;

			setText(seqRef.current, `#${storyTicks}`);
			setText(phaseRef.current, online ? enginePhase || "stream" : "offline");
			setText(candRef.current, candidates.toString());
			setText(openRef.current, open.toString());
			setText(
				quotesTextRef.current,
				quotesTotal > 0 ? `${quotesReady}/${quotesTotal}` : "—",
			);
			setText(fluidTextRef.current, fluid > 0 ? String(fluid) : "—");

			if (quotesBarRef.current !== null) {
				quotesBarRef.current.style.width = `${quotesPercent}%`;
			}

			if (fluidBarRef.current !== null) {
				fluidBarRef.current.style.width = `${fluidPercent}%`;
			}
		},
		[tickStore, appStore],
		[],
	);

	return (
		<div className="mx-2 border border-(--line) rounded-[3px] bg-(--sunken) p-2.5 font-mono text-[11px] leading-[1.7]">
			<div className="flex justify-between">
				<span className="text-(--f4)">seq</span>
				<span ref={seqRef} className="text-(--f1)" />
			</div>
			<div className="flex justify-between">
				<span className="text-(--f4)">phase</span>
				<span ref={phaseRef} className="text-(--acc)" />
			</div>
			<div className="flex justify-between">
				<span className="text-(--f4)">cand</span>
				<span ref={candRef} className="text-(--f1)" />
			</div>
			<div className="flex justify-between">
				<span className="text-(--f4)">open</span>
				<span ref={openRef} className="text-(--f1)" />
			</div>
			<div className="mt-[7px]">
				<div className="mb-1 flex justify-between">
					<span className="text-(--f4)">quotes</span>
					<span ref={quotesTextRef} className="text-(--f1)" />
				</div>
				<div className="h-1 overflow-hidden rounded-full bg-(--line)">
					<div ref={quotesBarRef} className="h-full rounded-full bg-(--info)" />
				</div>
			</div>
			<div className="mt-1.5">
				<div className="mb-1 flex justify-between">
					<span className="text-(--f4)">fluid</span>
					<span ref={fluidTextRef} className="text-(--f1)" />
				</div>
				<div className="h-1 overflow-hidden rounded-full bg-(--line)">
					<div ref={fluidBarRef} className="h-full rounded-full bg-(--warn)" />
				</div>
			</div>
		</div>
	);
};
