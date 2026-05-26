import "@tanstack/react-start/client-only";

import { memo, useCallback } from "react";
import { SciChartReact, type TResolvedReturnType } from "scichart-react";

import { ConfidenceDataProvider } from "#/components/symm/confidence-data-provider";
import { drawSignalGauge } from "#/components/symm/draw-signal-gauge";
import {
	SIGNAL_LABELS,
	SIGNAL_SOURCES,
	type SignalSource,
} from "#/lib/symm/signal-confidence";
import "#/lib/symm/scichart-setup";
import { Flex } from "./ui/flex";

type SignalGaugeProps = {
	source: SignalSource;
};

const SignalGauge = memo(function SignalGauge({ source }: SignalGaugeProps) {
	const initChart = useCallback((rootElement: string | HTMLDivElement) => {
		if (typeof rootElement === "string") {
			throw new Error("drawSignalGauge requires an HTMLDivElement root");
		}

		return drawSignalGauge(rootElement);
	}, []);

	const onInit = useCallback(
		(result: TResolvedReturnType<typeof drawSignalGauge>) => {
			const latest = ConfidenceDataProvider.snapshot().get(source);

			if (latest !== undefined) {
				result.controls.update(latest);
			}

			const unregister = ConfidenceDataProvider.registerSource(
				source,
				(confidence) => {
					result.controls.update(confidence);
				},
			);

			return () => {
				unregister();
				result.controls.dispose();
			};
		},
		[source],
	);

	return (
		<SciChartReact
			initChart={initChart}
			onInit={onInit}
			className="flex w-full h-full"
			innerContainerProps={{ className: "h-full w-full" }}
		/>
	);
});

export const Gauges = () => {
	return (
		<Flex.Column gap={1} padding={1} fullWidth fullHeight>
			<Flex.Row align="center" justify="center" fullWidth fullHeight>
				{SIGNAL_SOURCES.slice(0, 4).map((source: SignalSource) => (
					<Flex.Column key={source} fullWidth fullHeight>
						<small>{SIGNAL_LABELS[source]}</small>
						<SignalGauge key={source} source={source} />
					</Flex.Column>
				))}
			</Flex.Row>
			<Flex.Row align="center" justify="center" fullWidth fullHeight>
				{SIGNAL_SOURCES.slice(4).map((source: SignalSource) => (
					<Flex.Column key={source} fullWidth fullHeight>
						<small>{SIGNAL_LABELS[source]}</small>
						<SignalGauge key={source} source={source} />
					</Flex.Column>
				))}
			</Flex.Row>
		</Flex.Column>
	);
};
