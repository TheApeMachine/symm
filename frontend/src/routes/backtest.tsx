import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";

import { appStore } from "#/collections/app";
import { publishBacktestCommand } from "#/providers/websocket";
import { Section } from "@/components/ui/section";

const BacktestRoute = () => {
	const backtest = useSelector(appStore, (state) => state.backtest);

	return (
		<Section className="p-4">
			<h2 className="text-base font-semibold">Captured sessions</h2>
			{backtest.captures.length === 0 ? (
				<p className="text-sm text-muted-foreground">
					No captures yet — every live run records itself into the
					capture store automatically.
				</p>
			) : null}
			<ul className="mt-2 flex flex-col gap-1">
				{backtest.captures.map((capture) => (
					<li key={capture.id}>
						<button
							type="button"
							className="w-full rounded border px-3 py-2 text-left text-sm hover:bg-white/10"
							disabled={backtest.rebooting}
							onClick={() => {
								publishBacktestCommand(
									"select",
									undefined,
									capture.id,
								);
							}}
						>
							<span className="tabular-nums">#{capture.id}</span>{" "}
							{new Date(capture.startedAt).toLocaleString()} ·{" "}
							{capture.frames.toLocaleString()} frames
							{backtest.captureId === capture.id ? " · loaded" : ""}
						</button>
					</li>
				))}
			</ul>
			<p className="mt-3 text-sm text-muted-foreground">
				Playback controls live in the top bar and work from every
				surface.
			</p>
		</Section>
	);
};

export const Route = createFileRoute("/backtest")({
	component: BacktestRoute,
});
