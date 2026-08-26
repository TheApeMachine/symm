import { useRef, useState } from "react";
import { cognitionStore } from "#/collections/app";
import { meterTrackVariants } from "#/components/ui/meter";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";
import { Cognition } from "#/providers/telemetry/telemetry/cognition";
import { NamedNumber } from "#/providers/telemetry/telemetry/named-number";

type PredictionEntry = {
	name: string;
};

type PredictionQueryEntry = {
	path: HTMLElement | null;
	prob: HTMLElement | null;
	bar: HTMLElement | null;
};

const queryCache: Record<string, PredictionQueryEntry> = {};
const cogObj = new Cognition();
const predObj = new NamedNumber();

export const CortexBeamShell = ({ symbol }: { symbol: string }) => {
	const root = useRef<HTMLDivElement>(null);
	const [predictions, setPredictions] = useState<PredictionEntry[]>([]);

	cognitionStore.subscribe((state) => {
		if (!root.current) return;
		const last = state.getLast();
		if (!last) return;

		let targetRow: Cognition | null = null;
		for (let i = 0; i < last.rowsLength(); i++) {
			const row = last.rows(i, cogObj);
			if (row && row.symbol() === symbol) {
				targetRow = row;
				break;
			}
		}

		if (!targetRow) return;

		const currentPreds: PredictionEntry[] = [];

		for (let j = 0; j < targetRow.predictionsLength(); j++) {
			const pred = targetRow.predictions(j, predObj);
			if (!pred) continue;
			const name = pred.name() ?? "";
			const val = pred.value();
			if (!name) continue;

			currentPreds.push({ name });

			let element = queryCache[name];
			if (!element) {
				const cell = root.current.querySelector<HTMLElement>(`[data-path="${name}"]`);
				if (!cell) continue;

				element = {
					path: cell.querySelector<HTMLElement>('[data-bf="path"]'),
					prob: cell.querySelector<HTMLElement>('[data-bf="prob"]'),
					bar: cell.querySelector<HTMLElement>("[data-bbar]"),
				};
				queryCache[name] = element;
			}

			if (element.path) element.path.textContent = name;
			if (element.prob) element.prob.textContent = Number.isFinite(val) ? `${(val * 100).toFixed(1)}%` : "";
			if (element.bar && Number.isFinite(val)) {
				element.bar.style.width = `${Math.min(100, Math.max(0, val * 100)).toFixed(1)}%`;
			}
		}

		if (currentPreds.map((p) => p.name).join(",") !== predictions.map((p) => p.name).join(",")) {
			setPredictions(currentPreds);
		}
	});

	return (
		<div ref={root} className="flex min-h-0 flex-1 flex-col">
			{predictions.length === 0 ? (
				<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
					waiting for cognitive beam reading
				</div>
			) : (
				<div className="flex min-h-0 flex-1 flex-col gap-1.25 overflow-auto px-2 py-1.5">
					{predictions.map((pred, index) => (
						<Panel key={pred.name} size="s" data-path={pred.name} className="flex items-center gap-2">
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