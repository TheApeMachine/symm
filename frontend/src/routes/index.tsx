import { createFileRoute } from "@tanstack/react-router";
import { BoxIcon, HouseIcon, PanelsTopLeftIcon } from "lucide-react";
import { TabsPanel } from "node_modules/@base-ui/react/esm/tabs/panel/TabsPanel";
import { TabsTab } from "node_modules/@base-ui/react/esm/tabs/tab/TabsTab";
import { useRef } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import type { TResolvedReturnType } from "scichart-react";
import {
	SignalGauge,
	type SignalGaugeBridge,
} from "#/components/charts/confidence/Gauges";
import type { initFluidSurfaceChart } from "#/components/charts/fluid/init-fluid-surface-chart";
import { FluidFieldSurfaceChart } from "#/components/charts/fluid/SurfaceChart";
import { TradeChartGrid } from "#/components/charts/trade/TradeChart";
import {
	Card,
	CardFrame,
	CardFrameAction,
	CardFrameHeader,
	CardPanel,
} from "#/components/ui/card";
import { Flex } from "#/components/ui/flex";
import { Tabs } from "#/components/ui/tabs";
import { useWsStatus } from "#/providers/ws-status";

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

	const { setOnline, setBalance } = useWsStatus();

	useWebSocket(socketUrl, {
		shouldReconnect: () => true,
		onOpen: () => setOnline(true),
		onClose: () => setOnline(false),
		onError: () => setOnline(false),
		onMessage: (event) => {
			try {
				const raw = JSON.parse(event.data) as Record<string, unknown>;

				if (raw.event === "wallet") {
					setBalance(raw.balance as number);
					return;
				}

				if (raw.chart === "gauge") {
					const source = raw.source as string;
					const confidence = raw.confidence as number;
					const bridge = gaugeRefs.current[source];

					if (bridge) {
						if (bridge.ready) {
							bridge.update(confidence);
						} else {
							bridge.pending.push(confidence);
						}
					}

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
					<Tabs className="items-center h-full w-full" defaultValue="tab-1">
						<CardFrameHeader className="w-full">
							<CardFrameAction className="w-full">
								<div className="border-b w-full">
									<Tabs.List variant="underline" className="w-full">
										<TabsTab
											className="h-auto! w-full flex-col gap-1.5 py-[calc(--spacing(2)-1px)]"
											value="tab-1"
										>
											<HouseIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										</TabsTab>
										<TabsTab
											className="h-auto! w-full flex-col gap-1.5 py-[calc(--spacing(2)-1px)]"
											value="tab-2"
										>
											<PanelsTopLeftIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										</TabsTab>
										<TabsTab
											className="h-auto! w-full flex-col gap-1.5 py-[calc(--spacing(2.5)-1px)]"
											value="tab-3"
										>
											<BoxIcon
												aria-hidden="true"
												className="opacity-60"
												size={16}
											/>
										</TabsTab>
									</Tabs.List>
								</div>
							</CardFrameAction>
						</CardFrameHeader>
						<Card className="h-full w-full flex-1 overflow-hidden">
							<CardPanel className="h-full w-full p-0">
								<TabsPanel
									value="tab-1"
									className="flex align-center justify-evenly h-full w-full p-0"
								>
									<TradeChartGrid symbols={["BTCEUR"]} />
									<FluidFieldSurfaceChart bridgeRef={fluidRef} />
								</TabsPanel>
								<TabsPanel value="tab-2">
									<p className="p-4 text-center text-muted-foreground text-xs">
										Projects content
									</p>
								</TabsPanel>
								<TabsPanel value="tab-3">
									<p className="p-4 text-center text-muted-foreground text-xs">
										Packages content
									</p>
								</TabsPanel>
							</CardPanel>
						</Card>
					</Tabs>
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
