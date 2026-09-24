import { useEffect, useState } from "react";
import { signals } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Meter } from "#/components/ui/meter";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";
import { NamedNumber } from "#/providers/telemetry/telemetry/named-number";

type PredictionEntry = {
	name: string;
	value: number;
};

const predObj = new NamedNumber();

export const CortexBeamShell = ({ symbol }: { symbol: string }) => {
	const [predictions, setPredictions] = useState<PredictionEntry[]>([]);

	useEffect(() => {
		const apply = (state: typeof signals.cognition.state) => {
			const targetRow = state[symbol]?.getLast();

			if (!targetRow) return;

			const currentPreds: PredictionEntry[] = [];

			for (let index = 0; index < targetRow.predictionsLength(); index++) {
				const prediction = targetRow.predictions(index, predObj);
				if (!prediction) continue;

				const name = prediction.name() ?? "";
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

		apply(signals.cognition.state);
		const subscription = signals.cognition.subscribe(apply);
		return () => subscription.unsubscribe();
	}, [symbol]);

	return (
		<Flex.Column fullHeight className="min-h-0 flex-1">
			{predictions.length === 0 ? (
				<Typography.Paragraph
					variant="f4"
					className="px-3 py-6 text-center text-[11px]"
				>
					waiting for cognitive beam reading
				</Typography.Paragraph>
			) : (
				<Flex.Column
					gap={1}
					className="min-h-0 flex-1 overflow-auto px-2 py-1.5"
				>
					{predictions.map((pred, index) => (
						<Panel key={pred.name} size="s" className="flex items-center gap-2">
							<Typography.Mono size="s" className="w-4 shrink-0 text-(--info)">
								{index + 1}
							</Typography.Mono>
							<Typography.Span variant="f1" className="flex-1 text-[11px]">
								{pred.name}
							</Typography.Span>
							<Meter
								layout="bar"
								variant="info"
								size="xs"
								percent={pred.value * 100}
								className="w-[70px]"
							/>
							<Typography.Mono
								size="xs"
								tone="f3"
								className="w-11 shrink-0 text-right"
							>
								{`${(pred.value * 100).toFixed(1)}%`}
							</Typography.Mono>
						</Panel>
					))}
				</Flex.Column>
			)}
		</Flex.Column>
	);
};
