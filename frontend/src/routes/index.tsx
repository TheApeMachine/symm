import { createFileRoute } from "@tanstack/react-router";
import {
	ActivityIcon,
	Dice6Icon,
	PentagonIcon,
	SparklesIcon,
	TrendingUpDownIcon,
	WavesIcon,
} from "lucide-react";
import { useRef } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import {
	SignalGauge,
	type SignalGaugeBridge,
} from "#/components/charts/confidence/Gauges";
import {
	SignalHeatmap,
	type SignalHeatmapBridge,
} from "#/components/charts/confidence/SignalHeatmap";
import {
	SignalSurpriseHeatmap,
	type SignalSurpriseHeatmapBridge,
} from "#/components/charts/confidence/SignalSurpriseHeatmap";
import { FluidFieldSurfaceChart } from "#/components/charts/fluid/SurfaceChart";
import {
	type PredictionBridge,
	PredictionChart,
} from "#/components/charts/prediction/PredictionChart";
import {
	type SpiderBridge,
	SpiderChart,
} from "#/components/charts/spider/spider";
import { TabbedChart } from "#/components/charts/tabbed";
import { TradeChartGrid } from "#/components/charts/trade/TradeChart";
import { ingestCandleWire } from "#/components/charts/trade/trade-chart-wire";
import {
	Card,
	CardFrame,
	CardFrameAction,
	CardFrameHeader,
	CardPanel,
} from "#/components/ui/card";
import { Flex } from "#/components/ui/flex";
import { type ActionVerdict, useWsStatus } from "#/providers/ws-status";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

export type FluidPushBridge = {
	push: (raw: unknown) => void;
	ready: boolean;
	pending: unknown[];
};

const SOURCES: Record<string, string> = {
	hawkes: "Hawkes",
	fluid: "Fluid",
	pumpdump: "Pump",
	causal: "Causal",
	depthflow: "Depth",
	leadlag: "L-Lag",
	liquidity: "Liquidity",
	sentiment: "Sent",
	toxicity: "Toxic",
	correlation: "Corr",
	exhaustion: "Exhaust",
	prediction: "Pred",
	cvd: "CVD",
};

const ALL_SOURCES = Object.keys(SOURCES);

// Axes of the regime radar — the price-action feature vector the backend
// classifier (market/perspectives.ClassifyRegime) emits on {chart:"regime"}.
const REGIME_AXES: Record<string, string> = {
	volatility: "Vol",
	trend: "Trend",
	bullish: "Bull",
	bearish: "Bear",
	choppiness: "Chop",
};

const REGIME_AXIS_KEYS = Object.keys(REGIME_AXES);

const TOP = ["hawkes", "fluid", "pumpdump", "causal", "depthflow"];
const LEFT = ["leadlag", "liquidity", "sentiment", "toxicity"];
const RIGHT = ["correlation", "exhaustion", "prediction", "cvd"];

const emptyGaugeBridge = (): SignalGaugeBridge => ({
	update: () => {},
	ready: false,
	pending: [],
});

const DashboardLayout = () => {
	const fluidRef = useRef<FluidPushBridge>({
		push: () => {},
		ready: false,
		pending: [],
	});

	const gaugeRefs = useRef<Record<string, SignalGaugeBridge>>(
		Object.fromEntries(
			Object.keys(SOURCES).map((s) => [s, emptyGaugeBridge()]),
		),
	);

	const heatmapRef = useRef<SignalHeatmapBridge>({
		set: () => {},
		ready: false,
	});
	const surpriseRef = useRef<SignalSurpriseHeatmapBridge>({
		set: () => {},
		ready: false,
	});
	const spiderRef = useRef<SpiderBridge>({ set: () => {}, ready: false });
	const predictionRef = useRef<PredictionBridge>({
		append: () => {},
		ready: false,
	});

	const { setOnline, setWallet, pushAction, setPositions, setMark } =
		useWsStatus();

	useWebSocket(socketUrl, {
		shouldReconnect: () => true,
		onOpen: () => setOnline(true),
		onClose: () => setOnline(false),
		onError: () => setOnline(false),
		onMessage: (event) => {
			try {
				const raw = JSON.parse(event.data) as Record<string, unknown>;

				if (raw.event === "wallet") {
					setWallet((raw.balance as number) ?? 0, (raw.open as number) ?? 0);
					return;
				}

				if (raw.event === "decision") {
					pushAction({
						type: raw.type as string,
						symbol: raw.symbol as string,
						ts: Date.now(),
						verdict: (raw.verdict as ActionVerdict) ?? "rejected",
						reason: (raw.reason as string) ?? "",
					});
					return;
				}

				if (raw.event === "positions") {
					const rows = (raw.positions as Record<string, unknown>[]) ?? [];
					setPositions(
						rows.map((row) => ({
							symbol: row.symbol as string,
							qty: row.qty as number,
							avgEntry: row.avg_entry as number,
						})),
					);
					return;
				}

				if (raw.chart === "regime") {
					for (const axis of REGIME_AXIS_KEYS) {
						spiderRef.current.set(axis, (raw[axis] as number) ?? 0);
					}
					return;
				}

				if (raw.chart === "gauge") {
					const source = raw.source as string;
					const confidence = (raw.confidence as number) ?? 0;
					const snr = (raw.snr as number) ?? 0;
					const bridge = gaugeRefs.current[source];

					if (bridge) {
						if (bridge.ready) {
							bridge.update(confidence);
						} else {
							bridge.pending.push(confidence);
						}
					}

					heatmapRef.current.set(source, confidence);
					surpriseRef.current.set(source, snr);

					if (source === "prediction") {
						predictionRef.current.append(Date.now() / 1000, confidence);
					}

					return;
				}

				// ohlc candle data — route to trade chart and mark open positions
				if (typeof raw.symbol === "string" && typeof raw.open === "number") {
					if (typeof raw.close === "number") {
						setMark(raw.symbol, raw.close);
					}
					ingestCandleWire(raw);
					return;
				}

				const fluid = fluidRef.current;

				if (fluid.ready) {
					fluid.push(raw);
				} else {
					fluid.pending.push(raw);
				}
			} catch {
				return;
			}
		},
	});

	const gauge = (source: string) => (
		<SignalGauge
			key={source}
			bridgeRef={{ current: gaugeRefs.current[source] }}
			label={SOURCES[source] ?? source}
		/>
	);

	return (
		<Flex.Column gap={2} fullWidth fullHeight>
			<div className="flex w-full shrink-0 gap-2" style={{ height: "180px" }}>
				{TOP.map(gauge)}
			</div>
			<Flex.Row gap={2} fullWidth fullHeight>
				<div
					className="flex flex-col h-full gap-2 shrink-0"
					style={{ width: "180px" }}
				>
					{LEFT.map(gauge)}
				</div>
				<CardFrame className="w-full h-full">
					<CardFrameHeader className="w-full">
						<CardFrameAction className="w-full"></CardFrameAction>
					</CardFrameHeader>
					<Card className="h-full w-full flex-1 overflow-hidden">
						<CardPanel className="h-full w-full p-0 flex">
							<TabbedChart
								tabs={[
									{
										label: "Overview",
										icon: (
											<TrendingUpDownIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: <TradeChartGrid symbols={["BTC/EUR"]} />,
									},
									{
										label: "Prediction",
										icon: (
											<Dice6Icon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: <PredictionChart bridgeRef={predictionRef} />,
									},
									{
										label: "Signal",
										icon: (
											<ActivityIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: (
											<SignalHeatmap
												sources={ALL_SOURCES}
												labels={SOURCES}
												bridgeRef={heatmapRef}
											/>
										),
									},
								]}
							/>
							<TabbedChart
								tabs={[
									{
										label: "Fluid",
										icon: (
											<WavesIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: <FluidFieldSurfaceChart bridgeRef={fluidRef} />,
									},
									{
										label: "Regime",
										icon: (
											<PentagonIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: (
											<SpiderChart
												sources={REGIME_AXIS_KEYS}
												labels={REGIME_AXES}
												bridgeRef={spiderRef}
											/>
										),
									},
									{
										label: "Surprise",
										icon: (
											<SparklesIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: (
											<SignalSurpriseHeatmap
												sources={ALL_SOURCES}
												labels={SOURCES}
												bridgeRef={surpriseRef}
											/>
										),
									},
								]}
							/>
						</CardPanel>
					</Card>
				</CardFrame>
				<div
					className="flex flex-col h-full gap-2 shrink-0"
					style={{ width: "180px" }}
				>
					{RIGHT.map(gauge)}
				</div>
			</Flex.Row>
		</Flex.Column>
	);
};

export const Route = createFileRoute("/")({
	component: DashboardLayout,
});
