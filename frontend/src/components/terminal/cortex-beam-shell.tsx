import { useEffect, useState } from "react";
import { signals } from "#/collections/app";
import { meterTrackVariants } from "#/components/ui/meter";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";

type PredictionEntry = {
	name: string;
	value: number;
};

export const CortexBeamShell = ({ symbol }: { symbol: string }) => {
	const [predictions, setPredictions] = useState<PredictionEntry[]>([]);

	useEffect(() => {
		const apply = () => {
			const targetRow = signals.cognition.state[symbol]?.getLast();

			if (!targetRow) return;

			const currentPreds: PredictionEntry[] = [];

			for (const metric of targetRow.metrics) {
				const name =
					typeof metric.name === "string" ? metric.name : "";
				if (!name) continue;

				const val = metric.hasNormalized ? metric.normalized : metric.raw;
				currentPreds.push({ name, value: val });
			}

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

		apply();
		const subscription = signals.cognition.subscribe(apply);
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
