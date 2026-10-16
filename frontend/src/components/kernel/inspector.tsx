import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { focusAtom, type RingBuffer, signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
	kernelStatusVariant,
	metricLabel,
	type SignalHealthStatus,
	sourceHeadline,
	sourceMetrics,
} from "#/components/terminal/kernel-meta";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Meter } from "#/components/ui/meter";
import { Modal } from "#/components/ui/modal";
import { Typography } from "#/components/ui/typography";
import { Value } from "#/types/value";

const isResonance = (source: string) => source === "resonance";

/*
readings collects the accumulated values for one kernel directly from its store.
*/
const readingsFromMeasurements = (
	ring: RingBuffer<Value> | null | undefined,
	source?: string,
) => {
	const points: number[] = [];
	if (!ring || typeof ring.getBufferLength !== "function") return points;
	const len = ring.getBufferLength();
	const headline = source ? sourceHeadline(source) : null;

	for (let i = 0; i < len; i++) {
		const m = ring.get(i);
		if (!m) continue;
		let val: number | null = null;
		if (headline) {
			for (const metric of m.metrics ?? []) {
				if (metric?.name === headline && Number.isFinite(metric.raw)) {
					val = metric.snm || metric.raw;
					break;
				}
			}
		}
		if (val === null && m.metrics?.length) {
			const first = m.metrics[0];
			if (first && Number.isFinite(first.raw)) {
				val = first.snm || first.raw;
			}
		}
		if (val !== null && Number.isFinite(val)) {
			points.push(val);
		}
	}

	return points;
};

const relativeToOwnRange = (values: number[]): number[] => {
	if (values.length === 0) return [];

	const min = Math.min(...values);
	const max = Math.max(...values);
	const range = max - min;

	return values.map((value) => (range > 0 ? (value - min) / range : 1));
};

const formatMetricValue = (val: number | null): string => {
	if (val === null) return "—";
	if (val === 0) return "0.0000";
	const abs = Math.abs(val);
	if (abs < 0.0001) {
		return val.toExponential(2);
	}
	return val.toFixed(4);
};

const metricValues = (
	row: Value | null | undefined,
	names: string[],
): Record<string, { raw: number; snm: number } | null> => {
	const out: Record<string, { raw: number; snm: number } | null> = {};
	for (const name of names) out[name] = null;
	if (!row) return out;
	for (const m of row.metrics ?? []) {
		if (!m || !m.name) continue;
		const name = typeof m.name === "string" ? m.name : null;
		if (!name || !names.includes(name)) continue;
		out[name] = {
			raw: m.raw,
			snm: m.snm ?? m.raw,
		};
	}

	return out;
};

export const KernelInspector = () => {
	const navigate = useNavigate();
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focusSymbol = useSelector(focusAtom, (state) => state);
	const { closeInspect, selectSource } = terminalStore.actions;

	const active = source !== null && source !== "";
	const resonance = active && isResonance(source);

	const signalStore = source ? signals[source] : undefined;
	const measurementRing = useSelector(signalStore ?? signals.hawkes, (state) =>
		signalStore ? (state[focusSymbol] ?? null) : null,
	);

	if (!active) {
		return null;
	}

	const copy = kernelCopy(source, "");
	const points = readingsFromMeasurements(measurementRing, source);
	const latest = points.length > 0 ? points[points.length - 1] : null;
	const status: SignalHealthStatus = latest === null ? "waiting" : "measured";
	const badge = kernelStatusMeta(status);

	const relativePoints = resonance ? points : relativeToOwnRange(points);
	const paths = kernelSparkPaths(relativePoints, status);
	const level =
		relativePoints.length > 0 ? relativePoints[relativePoints.length - 1] : 0;

	const metrics = resonance ? [] : sourceMetrics(source);
	const metricReadouts = metrics.map((name) => {
		let raw: number | null = null;
		let snm = 0;

		const count = measurementRing?.getBufferLength
			? measurementRing.getBufferLength()
			: 0;
		let row: Value | null = null;
		for (let i = count - 1; i >= 0; i--) {
			const candidate = measurementRing?.get(i);
			if (candidate && metricValues(candidate, [name])[name] !== null) {
				row = candidate;
				break;
			}
		}
		const value = row ? metricValues(row, [name])[name] : null;

		if (value) {
			raw = value.raw;
			snm = value.snm;
		}

		return { name, raw, snm };
	});

	const headline = resonance
		? "predictive confidence"
		: (sourceHeadline(source) ?? "");

	const valueLabel = resonance
		? `${(level * 100).toFixed(0)}%`
		: latest === null
			? "—"
			: latest.toFixed(2);

	const openInSignalInsight = () => {
		selectSource(source);
		closeInspect();
		navigate({ to: "/signals" });
	};

	return (
		<Modal open onClose={closeInspect} size="m">
			<Modal.Header>
				<Flex.Column gap={1} className="min-w-0">
					<Flex.Row align="center" gap={2} className="min-w-0">
						<Typography.Display size="s" className="truncate">
							{copy.name}
						</Typography.Display>
						<Badge
							label={badge.label}
							variant={kernelStatusVariant(status)}
							size="xxs"
						/>
					</Flex.Row>
					<Typography.Mono size="xxs" tone="f4" className="truncate">
						{copy.sub}
					</Typography.Mono>
				</Flex.Column>
				<Modal.Close
					aria-label="Close kernel inspector"
					onClick={closeInspect}
				/>
			</Modal.Header>

			<Modal.Body className="flex flex-col gap-3.5">
				<Typography.Paragraph className="text-[13px] text-(--f2) leading-relaxed">
					{copy.blurb}
				</Typography.Paragraph>

				<Flex.Column gap={1}>
					<Flex.Row align="baseline" justify="between" className="gap-2">
						<Typography.Label size="xxs" tone="f4">
							Signal history
						</Typography.Label>
						<Typography.Mono size="xxs" tone="f4">
							{points.length === 0
								? "no readings yet"
								: `${points.length} ${points.length === 1 ? "reading" : "readings"}`}
						</Typography.Mono>
					</Flex.Row>
					<svg
						viewBox="0 0 150 30"
						preserveAspectRatio="none"
						className="block h-13 w-full rounded-[3px] border border-(--line) bg-(--sunken)"
					>
						<title>{`${copy.name} signal history`}</title>
						<polyline points={paths.area} fill={paths.fill} stroke="none" />
						<polyline
							points={paths.spark}
							fill="none"
							stroke={paths.line}
							strokeWidth="1.5"
							vectorEffect="non-scaling-stroke"
						/>
					</svg>
				</Flex.Column>

				<Flex.Column gap={2}>
					<Meter
						percent={level * 100}
						label={resonance ? "Confidence" : "Level"}
						value={valueLabel}
						variant={status === "measured" ? "warning" : "disabled"}
						size="s"
						animated
					/>
					<Meter
						percent={points.length === 0 ? 0 : (points.length / 50) * 100}
						label="History"
						value={`${points.length} / 50`}
						variant={points.length === 0 ? "disabled" : "warning"}
						size="s"
						animated
					/>
				</Flex.Column>

				{metrics.length === 0 ? null : (
					<Flex.Column gap={2}>
						<Flex.Row align="baseline" justify="between" className="gap-2">
							<Typography.Label size="xxs" tone="f4">
								Signal metrics
							</Typography.Label>
							<Typography.Mono size="xxs" tone="f4">
								{metricReadouts.filter((m) => m.raw !== null).length} /{" "}
								{metrics.length} read
							</Typography.Mono>
						</Flex.Row>
						<div className="grid grid-cols-2 gap-x-3 gap-y-2">
							{metricReadouts.map((metric) => (
								<Meter
									key={metric.name}
									percent={metric.raw === null ? 0 : Math.abs(metric.snm) * 100}
									label={metricLabel(metric.name)}
									value={formatMetricValue(metric.raw)}
									variant={metric.raw === null ? "disabled" : "warning"}
									size="xs"
									animated
								/>
							))}
						</div>
					</Flex.Column>
				)}
			</Modal.Body>

			<Modal.Footer>
				<Flex.Column className="min-w-0 gap-px">
					<Typography.Mono size="xxs" tone="f4" className="truncate">
						{focusSymbol}
						{headline === "" ? "" : ` · ${headline}`}
					</Typography.Mono>
					<Typography.Mono size="xxs" tone="f4">
						{latest === null
							? "awaiting first reading"
							: `latest ${resonance ? "confidence" : (sourceHeadline(source) ?? "reading")} ${latest.toFixed(resonance ? 3 : 4)}`}
					</Typography.Mono>
				</Flex.Column>
				<Button
					tone="accent"
					variant="solid"
					size="m"
					onClick={openInSignalInsight}
					className="shrink-0 whitespace-nowrap"
				>
					Open in signal insight →
				</Button>
			</Modal.Footer>
		</Modal>
	);
};
