import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Typography } from "@/components/ui/typography";
import { cognitionStore, useSubscribe } from "#/providers/ws-stores";

export const CortexBeamShell = ({ symbol }: { symbol: string }) => {
	const root = useSubscribe(cognitionStore, (state) => {
		const row = state.cognition[symbol]?.latest();
		const predictions = row?.predictions ?? {};

		for (const [path, prediction] of Object.entries(predictions)) {
			const cell = root.current?.querySelector<HTMLElement>(`[data-path="${path}"]`);

			if (!(cell instanceof HTMLElement)) {
				continue;
			}

			const probability = prediction?.probability;

			const set = (q: string, value: string) => {
				const el = cell.querySelector<HTMLElement>(`[data-bf="${q}"]`);

				if (el instanceof HTMLElement) {
					el.textContent = value;
				}
			};

			set("path", prediction?.predictedPath ?? path);
			set("prob", probability === undefined ? "" : `${(probability * 100).toFixed(1)}%`);

			const bar = cell.querySelector<HTMLElement>("[data-bbar]");

			if (bar instanceof HTMLElement && probability !== undefined) {
				bar.style.width = `${Math.min(100, Math.max(0, probability * 100)).toFixed(1)}%`;
			}
		}
	}, [symbol]);

	const predictions = cognitionStore.state.cognition[symbol]?.latest()?.predictions ?? {};

	return (
		<div ref={root} className="flex min-h-0 flex-1 flex-col">
			{Object.keys(predictions).length === 0 ? (
				<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
					waiting for cognitive beam reading
				</div>
			) : (
				<div className="flex min-h-0 flex-1 flex-col gap-1.25 overflow-auto px-2 py-1.5">
					{Object.entries(predictions).map(([path], index) => (
						<Panel key={path} size="s" data-path={path} className="flex items-center gap-2">
							<span className="w-4 shrink-0 font-mono text-[10px] text-(--info)">{index + 1}</span>
							<Typography.Span data-bf="path" className="flex-1 font-mono text-[11px] text-(--f1)" />
							<div className={meterTrackVariants({ variant: "info", size: "xs" })} style={{ width: "70px" }}>
								<div data-bbar className="h-full bg-(--meter-tone)" style={{ width: "0%" }} />
							</div>
							<Typography.Span data-bf="prob" className="w-11 shrink-0 text-right font-mono text-[9.5px] text-(--f3)" />
						</Panel>
					))}
				</div>
			)}
		</div>
	);
};