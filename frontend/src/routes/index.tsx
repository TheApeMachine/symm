import { createFileRoute } from "@tanstack/react-router";
import {
	ActivityIcon,
	BoxIcon,
	BrainCircuitIcon,
	Dice6Icon,
	PentagonIcon,
	ScanEyeIcon,
	SparklesIcon,
	TrendingUpDownIcon,
	WavesIcon,
} from "lucide-react";
import { useState } from "react";
import { CognitivePanel } from "#/components/charts/cognitive/CognitivePanel";
import { SignalGauge } from "#/components/charts/confidence/Gauges";
import { SignalHeatmap } from "#/components/charts/confidence/SignalHeatmap";
import { SignalSurpriseHeatmap } from "#/components/charts/confidence/SignalSurpriseHeatmap";
import { FluidFieldSurfaceChart } from "#/components/charts/fluid/SurfaceChart";
import { ManifoldSurfaceChart } from "#/components/charts/manifold/ManifoldSurfaceChart";
import { PredictionChart } from "#/components/charts/prediction/PredictionChart";
import { ResonanceXRayChart } from "#/components/charts/resonance/ResonanceXRayChart";
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
import {
	ALL_SIGNAL_SOURCES,
	SIGNAL_COMPACT_LABELS,
} from "#/collections/signals";
import { Flex } from "#/components/ui/flex";
import { cn } from "@/lib/utils";

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
const RESONANCE_CHART_TAB = "Resonance";
const SECONDARY_DEFAULT_TAB = "Fluid";

const DashboardLayout = () => {
	const [secondaryActiveTab, setSecondaryActiveTab] = useState(
		SECONDARY_DEFAULT_TAB,
	);
	const isResonanceFocus = secondaryActiveTab === RESONANCE_CHART_TAB;

	return (
		<Flex.Column gap={2} fullWidth fullHeight>
			<div className="flex w-full shrink-0 gap-2" style={{ height: "180px" }}>
				{TOP.map((source) => (
					<SignalGauge
						key={source}
						source={source}
						label={SIGNAL_COMPACT_LABELS[source] ?? source}
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
							label={SIGNAL_COMPACT_LABELS[source] ?? source}
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
								className={cn(isResonanceFocus && "hidden")}
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
											<SignalHeatmap
												sources={ALL_SIGNAL_SOURCES}
												labels={SIGNAL_COMPACT_LABELS}
											/>
										),
									},
								]}
							/>
							<TabbedChart
								onActiveTabChange={setSecondaryActiveTab}
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
										label: RESONANCE_CHART_TAB,
										icon: (
											<ScanEyeIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: <ResonanceXRayChart />,
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
										label: "Cognitive",
										icon: (
											<BrainCircuitIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										),
										component: <CognitivePanel />,
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
												sources={ALL_SIGNAL_SOURCES}
												labels={SIGNAL_COMPACT_LABELS}
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
							label={SIGNAL_COMPACT_LABELS[source] ?? source}
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
