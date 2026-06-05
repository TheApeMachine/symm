import { createFileRoute } from "@tanstack/react-router";
import {
	ActivityIcon,
	Dice6Icon,
	PentagonIcon,
	SparklesIcon,
	TrendingUpDownIcon,
	WavesIcon,
} from "lucide-react";
import { type RefObject, useRef } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import {
	SignalGauge,
	type SignalGaugeBridge,
} from "#/components/charts/confidence/Gauges";
import { gaugeWirePayload } from "#/components/charts/confidence/gauge-payload";
import {
	SignalHeatmap,
	type SignalHeatmapBridge,
} from "#/components/charts/confidence/SignalHeatmap";
import {
	SignalSurpriseHeatmap,
	type SignalSurpriseHeatmapBridge,
} from "#/components/charts/confidence/SignalSurpriseHeatmap";
import {
	deliverFluidWire,
	type FluidPushBridge,
} from "#/components/charts/fluid/fluid-push-bridge";
import { FluidFieldSurfaceChart } from "#/components/charts/fluid/SurfaceChart";
import {
	type PredictionBridge,
	PredictionChart,
} from "#/components/charts/prediction/PredictionChart";
import { SpiderChart } from "#/components/charts/spider/spider";
import {
	createSpiderBridge,
	deliverRegimeWire,
	parseRegimeWire,
	type SpiderBridge,
} from "#/components/charts/spider/spider-bridge";
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
import {
	applyGlobalFrame,
	statusSocketHandlers,
} from "#/providers/global-frames";

const socketUrl =
	import.meta.env.VITE_SYMM_WS_URL?.trim() || "ws://127.0.0.1:8765/ws";

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
	latest: {},
});

const WsFeed = ({
	fluidRef,
	gaugeRefs,
	heatmapRef,
	surpriseRef,
	spiderRef,
	predictionRef,
}: {
	fluidRef: RefObject<FluidPushBridge>;
	gaugeRefs: RefObject<Record<string, SignalGaugeBridge>>;
	heatmapRef: RefObject<SignalHeatmapBridge>;
	surpriseRef: RefObject<SignalSurpriseHeatmapBridge>;
	spiderRef: RefObject<SpiderBridge>;
	predictionRef: RefObject<PredictionBridge>;
}) => {
	useWebSocket(socketUrl, {
		...statusSocketHandlers,
		onMessage: (event) => {
			try {
				const raw = JSON.parse(event.data) as Record<string, unknown>;

				if (applyGlobalFrame(raw)) {
					return;
				}

				const regimeValues = parseRegimeWire(raw, REGIME_AXIS_KEYS);

				if (regimeValues !== null) {
					deliverRegimeWire(spiderRef.current, regimeValues);
					return;
				}

				if (raw.chart === "gauge") {
					const source = raw.source as string;
					const confidence = (raw.confidence as number) ?? 0;
					const snr = (raw.snr as number) ?? 0;
					const payload = gaugeWirePayload(raw);
					const bridge = gaugeRefs.current?.[source];

					if (bridge) {
						if (bridge.ready) {
							bridge.update(payload);
						} else {
							bridge.pending.push(payload);
						}
					}

					heatmapRef.current?.set(source, confidence);
					surpriseRef.current?.set(source, snr);

					if (source === "prediction") {
						const tsSec = Date.now() / 1000;
						const prediction = predictionRef.current;

						if (prediction?.ready) {
							prediction.append(tsSec, confidence);
						} else if (prediction) {
							prediction.pending.push([tsSec, confidence]);
						}
					}

					return;
				}

				if (typeof raw.symbol === "string" && typeof raw.open === "number") {
					// Candles drive the chart only. They MUST NOT write the marks map:
					// the OHLC stream always carries the chart-anchor (BTC/EUR), and
					// writing it into the shared, symbol-keyed marks map leaks a
					// foreign price onto a position (the €52k "ETH" mark). Positions
					// are marked solely by the backend per-position "mark" frame
					// (global-frames.ts), which is the exact symbol's authoritative bid.
					ingestCandleWire(raw);
					return;
				}

				const fluid = fluidRef.current;

				if (fluid) {
					deliverFluidWire(fluid, raw);
				}
			} catch {
				return;
			}
		},
	});

	return null;
};

const DashboardLayout = () => {
	const fluidRef = useRef<FluidPushBridge>({
		push: () => {},
		ready: false,
		pending: [],
	});

	const gaugeRefs = useRef<Record<string, SignalGaugeBridge>>(
		Object.fromEntries(
			Object.keys(SOURCES).map((source) => [source, emptyGaugeBridge()]),
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
	const spiderRef = useRef<SpiderBridge>(createSpiderBridge());
	const predictionRef = useRef<PredictionBridge>({
		append: () => {},
		ready: false,
		pending: [],
	});

	return (
		<Flex.Column gap={2} fullWidth fullHeight>
			<WsFeed
				fluidRef={fluidRef}
				gaugeRefs={gaugeRefs}
				heatmapRef={heatmapRef}
				surpriseRef={surpriseRef}
				spiderRef={spiderRef}
				predictionRef={predictionRef}
			/>
			<div className="flex w-full shrink-0 gap-2" style={{ height: "180px" }}>
				{TOP.map((source) => (
					<SignalGauge
						key={source}
						bridge={gaugeRefs.current[source]}
						label={SOURCES[source] ?? source}
					/>
				))}
			</div>
			<Flex.Row gap={2} fullWidth fullHeight>
				<div
					className="flex flex-col h-full gap-2 shrink-0"
					style={{ width: "180px" }}
				>
					{LEFT.map((source) => (
						<SignalGauge
							key={source}
							bridge={gaugeRefs.current[source]}
							label={SOURCES[source] ?? source}
						/>
					))}
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
					{RIGHT.map((source) => (
						<SignalGauge
							key={source}
							bridge={gaugeRefs.current[source]}
							label={SOURCES[source] ?? source}
						/>
					))}
				</div>
			</Flex.Row>
		</Flex.Column>
	);
};

export const Route = createFileRoute("/")({
	component: DashboardLayout,
});
