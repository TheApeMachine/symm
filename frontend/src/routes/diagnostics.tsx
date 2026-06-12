import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { SIGNAL_SOURCES } from "#/collections/signals";
import { SignalPanel } from "#/components/diagnostics/signal-panel";
import {
	Frame,
	FrameDescription,
	FrameHeader,
	FramePanel,
	FrameTitle,
} from "#/components/ui/frame";
import { Separator } from "#/components/ui/separator";

const DiagnosticsPage = () => {
	const online = useSelector(appStore, (state) => state.online);

	return (
		<div className="flex h-full min-h-0 w-full flex-col">
			<Frame className="w-full">
				<FrameHeader>
					<div className="flex items-center gap-2">
						<FrameTitle>Signal Insight</FrameTitle>
						{online ? (
							<span className="rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-emerald-400">
								Live
							</span>
						) : null}
					</div>
					<FrameDescription>
						Live signal diagnostics from the engine gauge feed — confidence,
						surprise, warmup, and overall health per signal.
					</FrameDescription>
				</FrameHeader>
				<FramePanel className="p-0">
					{SIGNAL_SOURCES.map((source, index) => (
						<div key={source}>
							{index > 0 ? <Separator /> : null}
							<SignalPanel source={source} />
						</div>
					))}
				</FramePanel>
			</Frame>
		</div>
	);
};

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsPage,
});
