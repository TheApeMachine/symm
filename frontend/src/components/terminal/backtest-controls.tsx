import { useSelector } from "@tanstack/react-store";

import { appStore } from "../../collections/app";
import { publishBacktestCommand } from "../../providers/websocket";

const formatClock = (iso: string | null): string => {
	if (iso === null) {
		return "--:--:--";
	}

	const parsed = new Date(iso);

	if (Number.isNaN(parsed.getTime())) {
		return "--:--:--";
	}

	return parsed.toLocaleTimeString([], { hour12: false });
};

/*
BacktestControls is the global playback cluster: play/pause, the capture-time
scrubber, and the loaded capture label. It lives in the top bar so it stays
reachable from every surface while a session replays. Scrubbing seeks by
rebuilding the stack, so the slider disables while a rebuild is in flight.
*/
export const BacktestControls = () => {
	const backtest = useSelector(appStore, (state) => state.backtest);

	// The wire frame carries a zero capture id before anything is loaded;
	// treat it like the null the store starts with.
	if (backtest.captureId === null || backtest.captureId === 0) {
		return null;
	}

	const startedMs =
		backtest.startedAt !== null ? Date.parse(backtest.startedAt) : null;
	const endedMs =
		backtest.endedAt !== null ? Date.parse(backtest.endedAt) : null;
	const positionMs =
		backtest.position !== null ? Date.parse(backtest.position) : null;

	const boundsReady =
		startedMs !== null &&
		endedMs !== null &&
		positionMs !== null &&
		!Number.isNaN(startedMs) &&
		!Number.isNaN(endedMs) &&
		!Number.isNaN(positionMs) &&
		endedMs > startedMs;

	const seek = (value: number) => {
		publishBacktestCommand("seek", new Date(value).toISOString());
	};

	return (
		<div className="flex items-center gap-2 text-xs">
			<button
				type="button"
				className="rounded border px-2 py-1 hover:bg-white/10"
				disabled={backtest.rebooting}
				onClick={() => {
					publishBacktestCommand(
						backtest.playing ? "pause" : "play",
					);
				}}
			>
				{backtest.playing ? "Pause" : "Play"}
			</button>
			{boundsReady ? (
				<input
					className="h-1 w-64 cursor-pointer accent-current"
					disabled={backtest.rebooting}
					max={endedMs}
					min={startedMs}
					onChange={(event) => {
						seek(Number(event.target.value));
					}}
					step={1000}
					type="range"
					value={positionMs}
				/>
			) : null}
			<span className="tabular-nums text-muted-foreground">
				{backtest.rebooting
					? "rebuilding…"
					: formatClock(backtest.position)}
			</span>
			<span className="text-muted-foreground">
				capture #{backtest.captureId}
			</span>
		</div>
	);
};
