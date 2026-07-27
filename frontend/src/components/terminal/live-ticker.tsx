import { createRef } from "react";
import { appStore } from "#/collections/app";
import type { Balance, Holding, Position, TradeBalance } from "#/collections/types";
import { formatUptime } from "#/components/terminal/kernel-meta";
import { walletMetrics } from "#/components/terminal/panels";
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

const setVisibility = (element: HTMLElement | null, visible: boolean) => {
	if (element !== null) {
		element.style.display = visible ? "" : "none";
	}
};

const asRows = <T,>(value: unknown): T[] => {
	if (Array.isArray(value)) {
		return value as T[];
	}

	if (value !== null && typeof value === "object") {
		return Object.values(value as Record<string, T>);
	}

	if (value === undefined || value === null) {
		return [];
	}

	return [value as T];
};

const latest = <T,>(value: unknown): T | undefined => asRows<T>(value).at(-1);

const pulseTickRef = createRef<HTMLSpanElement>();
const pulsePhaseRef = createRef<HTMLSpanElement>();
const pulseMeasRef = createRef<HTMLSpanElement>();
const pulseCandRef = createRef<HTMLSpanElement>();
const pulseOpenRef = createRef<HTMLSpanElement>();
const pulseQuotesRef = createRef<HTMLSpanElement>();

const openCountRef = createRef<HTMLSpanElement>();

const walletCashRef = createRef<HTMLSpanElement>();
const walletReservedRef = createRef<HTMLSpanElement>();
const walletEquityRef = createRef<HTMLSpanElement>();
const walletLamboRef = createRef<HTMLImageElement>();
const walletTickRef = createRef<HTMLSpanElement>();

const engineSeqRef = createRef<HTMLDivElement>();
const enginePhaseRef = createRef<HTMLDivElement>();
const engineCandRef = createRef<HTMLDivElement>();
const engineOpenRef = createRef<HTMLDivElement>();
const engineMeasRef = createRef<HTMLDivElement>();
const engineLatencyRef = createRef<HTMLDivElement>();

const clockRef = createRef<HTMLDivElement>();
const uptimeRef = createRef<HTMLDivElement>();

let lastBalances: Balance[] = [];
let lastPositions: Position[] = [];
let lastTradeBalance: TradeBalance | null = null;
let clockTimer: number | null = null;

type TickRow = {
	count?: number;
	completed?: boolean;
	phase?: string;
	measurements?: number;
	candidates?: number;
	open?: number;
	ns?: number;
	quotes_ready?: number;
	quotes_total?: number;
};

const latestTick = (value: unknown): TickRow | undefined => {
	if (value !== null && typeof value === "object" && !Array.isArray(value)) {
		const obj = value as Record<string, unknown>;
		if ("count" in obj || "measurements" in obj || "candidates" in obj) {
			return obj as unknown as TickRow;
		}
	}
	return latest<TickRow>(value);
};

/*
paintPulseTick paints the dashboard pulse strip from the current DRAW tick.
*/
export const paintPulseTick = (value: unknown) => {
	const tick = latestTick(value);

	setText(pulseTickRef.current, `#${String(tick?.count ?? 0)}`);
	setText(
		pulsePhaseRef.current,
		tick?.completed === true ? "complete" : (tick?.phase ?? "stream"),
	);
	setText(pulseMeasRef.current, String(tick?.measurements ?? "—"));
	setText(pulseCandRef.current, String(tick?.candidates ?? "—"));
	setText(pulseOpenRef.current, String(tick?.open ?? "—"));
	setText(
		pulseQuotesRef.current,
		tick?.quotes_ready !== undefined && tick?.quotes_total !== undefined
			? `${String(tick.quotes_ready)}/${String(tick.quotes_total)}`
			: typeof tick?.ns === "number"
				? `${Math.round(tick.ns / 1_000_000)}ms`
				: "—",
	);
};

/*
paintOpenCount paints the open-position counter from the current DRAW tick.
*/
export const paintOpenCount = (value: unknown) => {
	setText(openCountRef.current, String(latestTick(value)?.open ?? 0));
};

/*
engineClockText formats the current UTC wall clock for the engine footer.
*/
export const engineClockText = (at = new Date()): string =>
	`${at.toISOString().slice(11, 19)} UTC`;

/*
engineUptimeText formats session uptime from appStore.startedAtMs.
*/
export const engineUptimeText = (startedAtMs: number | null): string =>
	`uptime ${formatUptime(startedAtMs)}`;

/*
paintEngineClock paints UTC wall time and session uptime into the clock shell.
*/
export const paintEngineClock = () => {
	setText(clockRef.current, engineClockText());
	setText(uptimeRef.current, engineUptimeText(appStore.state.startedAtMs));
};

const bindClock = (host: HTMLElement | null) => {
	if (host === null) {
		if (clockTimer !== null) {
			window.clearInterval(clockTimer);
			clockTimer = null;
		}

		return;
	}

	if (clockTimer !== null) {
		return;
	}

	paintEngineClock();
	clockTimer = window.setInterval(paintEngineClock, 1000);
};

/*
paintEngineTick paints the nav engine readout from the current DRAW tick.
*/
export const paintEngineTick = (value: unknown) => {
	const tick = latestTick(value);
	const online = appStore.state.online;

	setText(engineSeqRef.current, `#${tick?.count ?? 0}`);
	setText(
		enginePhaseRef.current,
		online
			? tick?.completed === true
				? "complete"
				: (tick?.phase ?? "stream")
			: "offline",
	);
	setText(engineCandRef.current, tick?.candidates?.toString() ?? "—");
	setText(engineOpenRef.current, tick?.open?.toString() ?? "—");
	setText(engineMeasRef.current, tick?.measurements?.toString() ?? "—");
	setText(
		engineLatencyRef.current,
		typeof tick?.ns === "number" ? `${Math.round(tick.ns / 1_000_000)}ms` : "—",
	);
	paintEngineClock();
};

const paintWallet = () => {
	const lastHoldings = lastPositions
		.map((position) => position.holding)
		.filter((holding): holding is Holding => holding !== undefined && holding !== null);
	const wallet = walletMetrics(lastBalances, lastHoldings);
	const cashValue = wallet ? `${wallet.cash.toFixed(2)} ${wallet.asset}` : "—";
	const reservedValue = wallet
		? `${wallet.reserved.toFixed(2)} ${wallet.asset}`
		: "—";
	const pnl = Number(lastTradeBalance?.n);
	const equityValue = wallet && Number.isFinite(Number(lastTradeBalance?.e))
		? `${Number(lastTradeBalance?.e).toFixed(2)} ${wallet.asset}`
		: "—";

	setText(walletCashRef.current, cashValue);
	setText(walletReservedRef.current, reservedValue);
	setText(walletEquityRef.current, equityValue);
	setTone(
		walletEquityRef.current,
		pnl > 0 ? "var(--up)" : pnl < 0 ? "var(--down)" : "var(--f3)",
	);
	setVisibility(walletLamboRef.current, pnl > 0);
};

/*
paintWalletBalances refreshes wallet cash/equity from the current DRAW balances.
*/
export const paintWalletBalances = (value: unknown) => {
	lastBalances = asRows<Balance>(value);
	paintWallet();
};

/*
paintWalletPositions refreshes wallet cash/equity from the current DRAW positions.
*/
export const paintWalletPositions = (value: unknown) => {
	lastPositions = (Array.isArray(value)
		? value
		: value !== null && typeof value === "object"
			? Object.values(value as Record<string, Position>)
			: value != null
				? [value]
				: []) as Position[];
	paintWallet();
};

/*
paintWalletTradeBalance paints backend-owned liquidation value from trade balance.
*/
export const paintWalletTradeBalance = (value: unknown) => {
	lastTradeBalance =
		value !== null && typeof value === "object"
			? (value as TradeBalance)
			: null;
	paintWallet();
};

export const paintWalletHoldings = paintWalletPositions;

/*
paintWalletTick paints the wallet tick counter from the current DRAW tick.
*/
export const paintWalletTick = (value: unknown) => {
	setText(walletTickRef.current, String(latestTick(value)?.count ?? 0));
};

/*
LivePulseTicker is the static pulse-strip shell. DRAW paints via paintPulseTick.
*/
export const LivePulseTicker = () => (
	<div className="flex h-8 shrink-0 items-center gap-4 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)">
		<span ref={pulseTickRef} className="font-semibold text-(--f1)" />
		<span>
			phase <span ref={pulsePhaseRef} className="text-(--acc)" />
		</span>
		<span>
			meas <span ref={pulseMeasRef} />
		</span>
		<span>
			cand <span ref={pulseCandRef} />
		</span>
		<span>
			open <span ref={pulseOpenRef} />
		</span>
		<span>
			tick <span ref={pulseQuotesRef} />
		</span>
	</div>
);

/*
LiveOpenCount is the static open-position counter shell.
*/
export const LiveOpenCount = () => (
	<span className="font-mono text-[12px] text-(--f3)">
		<span ref={openCountRef} /> open positions
	</span>
);

/*
LiveWalletMetrics is the static wallet metric shell.
*/
export const LiveWalletMetrics = () => (
	<>
		<div className="flex flex-col items-end gap-px">
			<span className="text-[9px] text-(--f4) uppercase tracking-widest">
				Cash
			</span>
			<span
				ref={walletCashRef}
				className="font-mono text-[12px] font-medium text-(--f1)"
			/>
		</div>
		<div className="flex flex-col items-end gap-px">
			<span className="text-[9px] text-(--f4) uppercase tracking-widest">
				Reserved
			</span>
			<span
				ref={walletReservedRef}
				className="font-mono text-[12px] font-medium text-(--f3)"
			/>
		</div>
		<div className="relative flex flex-col items-end gap-px">
			<img
				ref={walletLamboRef}
				src="/lambo.png"
				alt=""
				aria-hidden="true"
				className="pointer-events-none absolute -top-1.5 right-0 h-11 opacity-60"
				style={{ display: "none" }}
			/>
			<span className="text-[9px] text-(--f4) uppercase tracking-widest">
				Liquidation
			</span>
			<span
				ref={walletEquityRef}
				className="relative font-mono text-[12px] font-semibold"
			/>
		</div>
		<div className="flex flex-col items-end gap-px">
			<span className="text-[9px] text-(--f4) uppercase tracking-widest">
				Tick
			</span>
			<span
				ref={walletTickRef}
				className="font-mono text-[12px] font-semibold text-(--acc)"
			/>
		</div>
	</>
);

/*
LiveEngineTicker is the static nav engine readout shell.
*/
export const LiveEngineTicker = () => (
	<Flex.Column className="mx-2 border border-(--line) rounded-[3px] bg-(--sunken) p-2.5 font-mono text-[11px] leading-[1.7]">
		<Flex.Row justify="between">
			<Flex className="text-(--f4)">seq</Flex>
			<Flex ref={engineSeqRef} className="text-(--f1)" />
		</Flex.Row>
		<Flex.Row justify="between">
			<Flex className="text-(--f4)">phase</Flex>
			<Flex ref={enginePhaseRef} className="text-(--acc)" />
		</Flex.Row>
		<Flex.Row justify="between">
			<Flex className="text-(--f4)">cand</Flex>
			<Flex ref={engineCandRef} className="text-(--f1)" />
		</Flex.Row>
		<Flex.Row justify="between">
			<Flex className="text-(--f4)">open</Flex>
			<Flex ref={engineOpenRef} className="text-(--f1)" />
		</Flex.Row>
		<Flex.Row justify="between">
			<Flex className="text-(--f4)">meas</Flex>
			<Flex ref={engineMeasRef} className="text-(--f1)" />
		</Flex.Row>
		<Flex.Row justify="between">
			<Flex className="text-(--f4)">tick</Flex>
			<Flex ref={engineLatencyRef} className="text-(--f1)" />
		</Flex.Row>
	</Flex.Column>
);

/*
LiveEngineClock is the static UTC / uptime shell. bindClock owns the 1s timer;
DRAW also refreshes via paintEngineTick → paintEngineClock.
*/
export const LiveEngineClock = () => (
	<div ref={bindClock}>
		<Flex ref={clockRef} />
		<Flex ref={uptimeRef}>uptime —</Flex>
	</div>
);
