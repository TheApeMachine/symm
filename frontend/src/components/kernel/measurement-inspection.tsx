import type { Measurement } from "#/collections/types";
import { metricLabel } from "#/components/terminal/kernel-meta";
import { Flex } from "#/components/ui/flex";
import { meterTrackVariants } from "#/components/ui/meter";
import { Panel } from "#/components/ui/panel";
import { Typography } from "#/components/ui/typography";
import { cn } from "#/lib/utils";

type MetricSample = NonNullable<Measurement["metrics"]>[string];

const display = (value: unknown): string =>
	value === undefined || value === null || value === ""
		? "not reported"
		: String(value);

const instant = (value: string | undefined): string => {
	if (!value || value.startsWith("0001-01-01")) {
		return "not reported";
	}

	return new Date(value).toISOString().replace("T", " ").replace("Z", " UTC");
};

const horizon = (measurement: Measurement): string => {
	if (
		!measurement.observedFrom ||
		measurement.observedFrom.startsWith("0001-01-01")
	) {
		return "point observation";
	}

	return typeof measurement.horizon === "number"
		? `${measurement.horizon} ns`
		: display(measurement.horizon);
};

const normalizedWidth = (value: number | null | undefined): string => {
	if (value === undefined || value === null) {
		return "0%";
	}

	return `${Math.min(Math.abs(value), 1) * 100}%`;
};

const Field = ({ label, value }: { label: string; value: unknown }) => (
	<div className="min-w-0">
		<Typography.Label size="xxs" tone="f4" weight="normal">
			{label}
		</Typography.Label>
		<Typography.Mono size="s" tone="f1" className="mt-0.5 block truncate">
			{display(value)}
		</Typography.Mono>
	</div>
);

const UnitMeter = ({ value }: { value: number | null | undefined }) => (
	<div className={meterTrackVariants({ variant: "warning", size: "xs" })}>
		<div
			className={cn(
				"h-full",
				value !== undefined && value !== null && value < 0
					? "bg-(--down)"
					: "bg-(--meter-tone)",
			)}
			style={{ width: normalizedWidth(value) }}
		/>
	</div>
);

const MetricMeter = ({
	name,
	sample,
}: {
	name: string;
	sample: MetricSample;
}) => (
	<Panel size="s" className="min-w-0 gap-2">
		<Flex.Row align="baseline" justify="between" className="gap-2">
			<Typography.Label size="xxs" tone="f2" className="truncate">
				{metricLabel(name)}
			</Typography.Label>
			<Typography.Mono size="xxs" tone="f4" className="shrink-0">
				{display(sample.unit)}
			</Typography.Mono>
		</Flex.Row>

		<div className="grid grid-cols-2 gap-3">
			<Field label="raw" value={sample.raw} />
			<Field label="normalized" value={sample.normalized} />
		</div>

		<UnitMeter value={sample.normalized} />
	</Panel>
);

export const MeasurementInspection = ({
	measurement,
}: {
	measurement: Measurement | null;
}) => {
	if (measurement === null) {
		return (
			<Panel size="m">
				<Typography.Mono size="s" tone="f4">
					No measurement has been observed for this symbol.
				</Typography.Mono>
			</Panel>
		);
	}

	const metrics = Object.entries(measurement.metrics ?? {}).sort(
		([left], [right]) => left.localeCompare(right),
	);
	const uncertainty = measurement.uncertainty;

	return (
		<Flex.Column gap={4}>
			<section>
				<Typography.Label size="xxs" tone="f3" className="mb-2 block">
					Provenance
				</Typography.Label>
				<div className="grid grid-cols-2 gap-x-5 gap-y-2.5">
					<Field label="measurement id" value={measurement.id} />
					<Field label="source" value={measurement.source} />
					<Field label="symbol" value={measurement.symbol} />
					<Field label="peer" value={measurement.peer} />
				</div>
			</section>

			<section className="border-(--line) border-t pt-3.5">
				<Typography.Label size="xxs" tone="f3" className="mb-2 block">
					Observation window
				</Typography.Label>
				<div className="grid grid-cols-2 gap-x-5 gap-y-2.5">
					<Field
						label="observed from"
						value={instant(measurement.observedFrom)}
					/>
					<Field label="observed at" value={instant(measurement.at)} />
					<Field label="horizon" value={horizon(measurement)} />
					<Field label="maturity" value={measurement.maturity} />
				</div>
				<div className="mt-2">
					<UnitMeter value={measurement.maturity} />
				</div>
			</section>

			<section className="border-(--line) border-t pt-3.5">
				<Typography.Label size="xxs" tone="f3" className="mb-2 block">
					Estimator uncertainty
				</Typography.Label>
				{uncertainty ? (
					<>
						<div className="grid grid-cols-2 gap-x-5 gap-y-2.5">
							<Field label="lower" value={uncertainty.lower} />
							<Field label="upper" value={uncertainty.upper} />
							<Field label="confidence" value={uncertainty.confidence} />
							<Field label="method" value={uncertainty.method} />
						</div>
						<div className="mt-2">
							<UnitMeter value={uncertainty.confidence} />
						</div>
					</>
				) : (
					<Typography.Mono size="s" tone="f4">
						No estimator interval was reported.
					</Typography.Mono>
				)}
			</section>

			<section className="border-(--line) border-t pt-3.5">
				<Flex.Row align="baseline" justify="between" className="mb-2 gap-2">
					<Typography.Label size="xxs" tone="f3">
						Metrics
					</Typography.Label>
					<Typography.Mono size="xxs" tone="f4">
						{metrics.length} readings
					</Typography.Mono>
				</Flex.Row>
				<div className="grid grid-cols-2 gap-2">
					{metrics.map(([name, sample]) => (
						<MetricMeter key={name} name={name} sample={sample} />
					))}
				</div>
			</section>
		</Flex.Column>
	);
};
