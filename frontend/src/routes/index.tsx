import { createFileRoute } from "@tanstack/react-router";
import {
	ActivityIcon,
	BoxIcon,
	Dice6Icon,
	PentagonIcon,
	SparklesIcon,
	TrendingUpDownIcon,
	WavesIcon,
} from "lucide-react";
import { SignalGauge } from "#/components/charts/confidence/Gauges";
import { SignalHeatmap } from "#/components/charts/confidence/SignalHeatmap";
import { SignalSurpriseHeatmap } from "#/components/charts/confidence/SignalSurpriseHeatmap";
import { FluidFieldSurfaceChart } from "#/components/charts/fluid/SurfaceChart";
import { ManifoldSurfaceChart } from "#/components/charts/manifold/ManifoldSurfaceChart";
import { PredictionChart } from "#/components/charts/prediction/PredictionChart";
import { SpiderChart } from "#/components/charts/spider/spider";
import { TabbedChart } from "#/components/charts/tabbed";
import { PositionTradeCharts } from "#/components/charts/trade/TradeChart";
import {
	Card,
	CardFrame,
	CardFrameAction,
	CardFrameHeader,
	CardPanel,
} from "#/components/ui/card";
import { Flex } from "#/components/ui/flex";

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
	manifold: "Manifold",
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

const DashboardLayout = () => {
	return (
		<Flex.Column gap={2} fullWidth fullHeight>
			<div className="flex w-full shrink-0 gap-2" style={{ height: "180px" }}>
				{TOP.map((source) => (
					<SignalGauge
						key={source}
						source={source}
						label={SOURCES[source] ?? source}
					/>
				))}
			</div>
			<Flex.Row gap={2} fullWidth fullHeight>
				<div
					className="flex h-full shrink-0 flex-col gap-2"
					style={{ width: "180px" }}
				>
					{LEFT.map((source) => (
						<SignalGauge
							key={source}
							source={source}
							label={SOURCES[source] ?? source}
						/>
					))}
				</div>
				<CardFrame className="h-full w-full">
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
										component: <PositionTradeCharts />,
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
										component: <PredictionChart />,
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
											<SignalHeatmap sources={ALL_SOURCES} labels={SOURCES} />
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
										component: <FluidFieldSurfaceChart />,
									},
									{
										label: "Manifold",
										icon: (
											<BoxIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: <ManifoldSurfaceChart />,
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
											/>
										),
									},
								]}
							/>
						</CardPanel>
					</Card>
				</CardFrame>
				<div
					className="flex h-full shrink-0 flex-col gap-2"
					style={{ width: "180px" }}
				>
					{RIGHT.map((source) => (
						<SignalGauge
							key={source}
							source={source}
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
