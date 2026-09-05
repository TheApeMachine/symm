import { useEffect, useState } from "react";
import { cognitionStore } from "#/collections/app";
import { meterTrackVariants } from "#/components/ui/meter";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";
import { EnvelopeCognitionPrediction } from "#/providers/telemetry/telemetry/envelope-cognition-prediction";

type PredictionEntry = {
	name: string;
	value: number;
};

const predObj = new EnvelopeCognitionPrediction();

export const CortexBeamShell = ({ symbol }: { symbol: string }) => {
	const [predictions, setPredictions] = useState<PredictionEntry[]>([]);

	useEffect(() => {
		const apply = (state: typeof cognitionStore.state) => {
			const targetRow = state.getLast(symbol);

			if (!targetRow) return;

			const currentPreds: PredictionEntry[] = [];

			for (let index = 0; index < targetRow.predictionsLength(); index++) {
				const prediction = targetRow.predictions(index, predObj);
				if (!prediction) continue;

				const name = prediction.key() ?? "";
				if (!name) continue;

				currentPreds.push({ name, value: prediction.value() });
			}

			/*
			The rows render straight from this state, so a reading that changes
			only the scores still repaints. Bypassing React to poke the cells by
			hand was what left a row sitting empty whenever the roster itself
			held steady.
			*/
			setPredictions((prev) =>
				prev.length === currentPreds.length &&
				prev.every(
					(entry, index) =>
						entry.name === currentPreds[index]?.name &&
						entry.value === currentPreds[index]?.value,
				)
					? prev
					: currentPreds,
			);
		};

		apply(cognitionStore.state);
		const subscription = cognitionStore.subscribe(apply);
		return () => subscription.unsubscribe();
	}, [symbol]);

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			{predictions.length === 0 ? (
				<div className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)">
					waiting for cognitive beam reading
				</div>
			) : (
				<div className="flex min-h-0 flex-1 flex-col gap-1.25 overflow-auto px-2 py-1.5">
					{predictions.map((pred, index) => (
						<Panel key={pred.name} size="s" className="flex items-center gap-2">
							<span className="w-4 shrink-0 font-mono text-[10px] text-(--info)">
								{index + 1}
							</span>
							<Typography.Span className="flex-1 font-mono text-[11px] text-(--f1)">
								{pred.name}
							</Typography.Span>
							<div
								className={meterTrackVariants({ variant: "info", size: "xs" })}
								style={{ width: "70px" }}
							>
								<div
									className="h-full bg-(--meter-tone)"
									style={{
										width: `${Math.min(100, Math.max(0, pred.value * 100)).toFixed(1)}%`,
									}}
								/>
							</div>
							<Typography.Span className="w-11 shrink-0 text-right font-mono text-[9.5px] text-(--f3)">
								{`${(pred.value * 100).toFixed(1)}%`}
							</Typography.Span>
						</Panel>
					))}
				</div>
			)}
		</div>
	);
};
