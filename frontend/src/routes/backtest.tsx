import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect } from "react";

import type { BacktestCapture } from "#/collections/app";
import { appStore } from "#/collections/app";
import {
	HindsightEmpty,
	HindsightPanel,
} from "#/components/backtest/hindsight";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { publishBacktestCommand } from "#/providers/websocket";

/*
backtestBaseUrl locates the hub's REST capture-listing endpoint, mirroring the
websocket origin (env override with a localhost default).
*/
const backtestBaseUrl = () => {
	if (import.meta.env.VITE_SYMM_WS_URL) {
		return import.meta.env.VITE_SYMM_WS_URL.replace(/^ws/, "http").replace(/\/ws$/, "");
	}

	const protocol = window.location.protocol === "https:" ? "https:" : "http:";
	const host =
		!window.location.hostname || window.location.hostname === "localhost"
			? "127.0.0.1"
			: window.location.hostname;

	return `${protocol}//${host}:8765`;
};

const BacktestRoute = () => {
	const backtest = useSelector(appStore, (state) => state.backtest);
	const hindsight = backtest.hindsight;

	useEffect(() => {
		let cancelled = false;

		const loadCaptures = async () => {
			try {
				const response = await fetch(`${backtestBaseUrl()}/backtest/captures`);

				if (!response.ok) {
					return;
				}

				const captures = (await response.json()) as BacktestCapture[];

				if (!cancelled) {
					appStore.actions.setBacktestCaptures(captures);
				}
			} catch {
				// The backend may still be booting; the websocket reconnects and a
				// later visit re-fetches.
			}
		};

		loadCaptures();

		return () => {
			cancelled = true;
		};
	}, []);

	return (
		<div className="flex h-full min-w-275 overflow-hidden bg-(--bg)">
			{/* Left sidebar — capture list */}
			<Section
				fit="pane"
				surface="surface"
				className="w-72 shrink-0 border-r border-(--line)"
			>
				<Section.Header title="Captures" size="lg" rule sticky />
				<Section.Body>
					{backtest.captures.length === 0 ? (
						<p className="px-3 py-3 font-mono text-[10px] text-(--f4)">
							No captures yet — every live run records itself automatically.
						</p>
					) : null}
					<ul className="flex flex-col divide-y divide-(--line)">
						{backtest.captures.map((capture) => {
							const active = backtest.captureId === capture.id;

							return (
								<li key={capture.id}>
									<Button
										variant="bare"
										disabled={backtest.rebooting}
										className={`flex w-full flex-col items-start gap-1 px-3 py-2.5 text-left hover:bg-(--raised) ${active ? "bg-(--raised)" : ""}`}
										onClick={() => {
											publishBacktestCommand(
												"select",
												undefined,
												capture.id,
											);
										}}
									>
										<Flex.Row
											align="center"
											justify="between"
											className="w-full"
										>
											<span className="font-mono text-[11px] font-semibold tabular-nums text-(--f1)">
												#{capture.id}
											</span>
											{active ? (
												<span className="rounded-[3px] border border-(--acc) px-1.5 py-px font-mono text-[8px] uppercase tracking-widest text-(--acc)">
													{backtest.rebooting
														? "loading…"
														: backtest.playing
															? "playing"
															: "loaded"}
												</span>
											) : null}
										</Flex.Row>
										<span className="font-mono text-[10px] text-(--f4)">
											{new Date(capture.startedAt).toLocaleString()}
										</span>
										<span className="font-mono text-[10px] text-(--f4)">
											{capture.frames.toLocaleString()} frames
										</span>
									</Button>
								</li>
							);
						})}
					</ul>
				</Section.Body>
				<div className="shrink-0 border-t border-(--line) px-3 py-2">
					<p className="font-mono text-[10px] text-(--f4)">
						{backtest.captureId === null || backtest.captureId === 0
							? "Select a capture to load it, then press Play in the top bar."
							: backtest.rebooting
								? "Loading the session stream…"
								: backtest.playing
									? "Streaming capture frames from the store."
									: "Loaded. Press Play in the top bar to run the session."}
					</p>
				</div>
			</Section>

			{/* Right pane — hindsight */}
			<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-hidden">
				{/* Pane header */}
				<Flex.Row
					align="center"
					justify="between"
					className="h-11.5 shrink-0 border-b border-(--line) bg-(--surface) px-4"
				>
					<Flex.Row align="center" gap={3}>
						<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							Hindsight
						</span>
						<span className="font-mono text-[12px] font-semibold text-(--f1)">
							What the tape offered vs. what we took
						</span>
					</Flex.Row>
					{backtest.captureId !== null ? (
						<Button
							variant="outline"
							size="xs"
							onClick={() => {
								publishBacktestCommand(
									"hindsight",
									undefined,
									backtest.captureId ?? undefined,
								);
							}}
						>
							Re-analyze
						</Button>
					) : null}
				</Flex.Row>

				{/* Pane body */}
				{hindsight === null || hindsight.status === "analyzing" ? (
					<Flex.Column className="flex-1">
						<HindsightEmpty captureId={backtest.captureId} />
						{hindsight?.status === "analyzing" ? (
							<p className="shrink-0 border-t border-(--line) px-4 py-2.5 font-mono text-[10px] text-(--acc)">
								Analyzing capture #{hindsight.captureId}…
							</p>
						) : null}
					</Flex.Column>
				) : null}

				{hindsight !== null && hindsight.status === "error" ? (
					<Flex.Column className="flex-1 p-4">
						<p className="font-mono text-[10px] text-(--down)">
							Hindsight analysis failed for capture #{hindsight.captureId}.
						</p>
					</Flex.Column>
				) : null}

				{hindsight !== null && hindsight.status === "ready" ? (
					<HindsightPanel report={hindsight} />
				) : null}
			</Flex.Column>
		</div>
	);
};

export const Route = createFileRoute("/backtest")({
	component: BacktestRoute,
});
