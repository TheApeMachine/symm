import { type RefObject, useEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import { type Balance, balancesStore } from "#/collections/balances";
import { type Position, positionsStore } from "#/collections/positions";
import { tickStore } from "#/collections/tick";
import { formatUptime } from "#/components/terminal/kernel-meta";
import { walletMetrics } from "#/components/terminal/panels";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { Flex } from "@/components/ui/flex";

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
				online
					? String(
							tick?.completed === true ? "complete" : (tick?.phase ?? "stream"),
						)
					: "offline",
			);
			setText(measRef.current, String(tick?.measurements ?? "—"));
			setText(candRef.current, String(tick?.candidates ?? "—"));
			setText(openRef.current, String(tick?.open ?? "—"));
			setText(
				quotesRef.current,
				typeof tick?.ns === "number"
					? `${Math.round(tick.ns / 1_000_000)}ms`
					: tick?.quotes_ready !== undefined && tick?.quotes_total !== undefined
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
				tick <span ref={quotesRef} />
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
	// Equity tone follows price P&L (mark vs entry), not fee-dragged unrealized
	// dollars — entry fees alone would paint a green book red.
	const pricePnl = positions.reduce((total, position) => {
		if (
			!(position.qty > 0) ||
			!(position.entry_price > 0) ||
			!(position.mark > 0)
		) {
			return total;
		}

		return total + (position.mark - position.entry_price) * position.qty;
	}, 0);
	const inProfit = pricePnl > 0;
	const equityValue = wallet
		? `${wallet.equity.toFixed(2)} ${wallet.asset}`
		: "—";

	setText(refs.cash, cashValue);
	setText(refs.equity, equityValue);
	setTone(
		refs.equity,
		pricePnl > 0 ? "var(--up)" : pricePnl < 0 ? "var(--down)" : "var(--f3)",
	);
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
	const equityRef = useRef<HTMLSpanElement>(null);
	const lamboRef = useRef<HTMLImageElement>(null);
	const tickRef = useRef<HTMLSpanElement>(null);

	useDirectStorePaint(
		() => {
			paintWalletMetrics(
				{
					cash: cashRef.current,
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
cand/open are position counts (zeros are honest). tick latency and measurement
counts come from publishTick; retired quotes/fluid bars are not invented.
*/
export const LiveEngineTicker = () => {
	const seqRef = useRef<HTMLDivElement>(null);
	const phaseRef = useRef<HTMLDivElement>(null);
	const candRef = useRef<HTMLDivElement>(null);
	const openRef = useRef<HTMLDivElement>(null);
	const measRef = useRef<HTMLDivElement>(null);
	const latencyRef = useRef<HTMLDivElement>(null);

	useDirectStorePaint(
		() => {
			const online = appStore.state.online;
			const tick = tickStore.state.frame;
			const storyTicks = tick?.count ?? 0;
			const enginePhase =
				tick?.completed === true ? "complete" : ((tick?.phase as string) ?? "");
			const candidates = (tick?.candidates as number) ?? 0;
			const open = (tick?.open as number) ?? 0;
			const measurements =
				typeof tick?.measurements === "number" ? tick.measurements : null;
			const ns = typeof tick?.ns === "number" ? tick.ns : null;
			const latencyMs = ns === null ? null : Math.round(ns / 1_000_000);

			setText(seqRef.current, `#${storyTicks}`);
			setText(phaseRef.current, online ? enginePhase || "stream" : "offline");
			setText(candRef.current, candidates.toString());
			setText(openRef.current, open.toString());
			setText(
				measRef.current,
				measurements === null ? "—" : String(measurements),
			);
			setText(latencyRef.current, latencyMs === null ? "—" : `${latencyMs}ms`);
		},
		[tickStore, appStore],
		[],
	);

	return (
		<Flex.Column className="mx-2 border border-(--line) rounded-[3px] bg-(--sunken) p-2.5 font-mono text-[11px] leading-[1.7]">
			<Flex.Row justify="between">
				<Flex className="text-(--f4)">seq</Flex>
				<Flex ref={seqRef} className="text-(--f1)" />
			</Flex.Row>
			<Flex.Row justify="between">
				<Flex className="text-(--f4)">phase</Flex>
				<Flex ref={phaseRef} className="text-(--acc)" />
			</Flex.Row>
			<Flex.Row justify="between">
				<Flex className="text-(--f4)">cand</Flex>
				<Flex ref={candRef} className="text-(--f1)" />
			</Flex.Row>
			<Flex.Row justify="between">
				<Flex className="text-(--f4)">open</Flex>
				<Flex ref={openRef} className="text-(--f1)" />
			</Flex.Row>
			<Flex.Row justify="between">
				<Flex className="text-(--f4)">meas</Flex>
				<Flex ref={measRef} className="text-(--f1)" />
			</Flex.Row>
			<Flex.Row justify="between">
				<Flex className="text-(--f4)">tick</Flex>
				<Flex ref={latencyRef} className="text-(--f1)" />
			</Flex.Row>
		</Flex.Column>
	);
};

/*
LiveEngineClock paints UTC clock and session uptime from appStore.startedAtMs.
*/
export const LiveEngineClock = () => {
	const clockRef = useRef<HTMLDivElement>(null);
	const uptimeRef = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const paint = () => {
			setText(
				clockRef.current,
				`${new Date().toISOString().slice(11, 19)} UTC`,
			);
			setText(
				uptimeRef.current,
				`uptime ${formatUptime(appStore.state.startedAtMs)}`,
			);
		};

		paint();
		const interval = window.setInterval(paint, 1000);
		const subscription = appStore.subscribe(paint);

		return () => {
			window.clearInterval(interval);
			subscription.unsubscribe();
		};
	}, []);

	return (
		<>
			<Flex ref={clockRef} />
			<Flex ref={uptimeRef}>uptime —</Flex>
		</>
	);
};
