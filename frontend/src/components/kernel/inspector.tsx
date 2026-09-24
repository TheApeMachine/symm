import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { focusAtom, signals } from "#/collections/app";
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

const isResonance = (source: string) => source === "resonance";

/*
readings collects the accumulated values for one kernel directly from its store.
*/
const readingsFromMeasurements = (ring: any) => {
	const points: number[] = [];
	if (!ring || typeof ring.getBufferLength !== "function") return points;
	const len = ring.getBufferLength();

	for (let i = 0; i < len; i++) {
		const m = ring.get(i);

		if (m) {
			const snr = typeof m.snr === "function" ? m.snr() : m.snr;
			if (typeof snr === "number" && Number.isFinite(snr)) {
				points.push(snr);
			}
		}
	}

	return points;
};

const readingsFromNumbers = (ring: any) => {
	const points: number[] = [];
	if (!ring || typeof ring.getBufferLength !== "function") return points;
	const len = ring.getBufferLength();

	for (let i = 0; i < len; i++) {
		const value = ring.get(i);
		const numVal =
			typeof value === "number"
				? value
				: typeof value?.confidence === "function"
					? value.confidence()
					: value?.confidence;

		if (numVal !== undefined && Number.isFinite(numVal)) {
			points.push(numVal);
		}
	}

	return points;
};

/*
relativeToOwnRange scales unbounded SNR against the range this kernel has
actually observed — the same mapping the list row uses, for the same reason:
there is no absolute "good SNR" threshold in the domain to assert.
*/
const relativeToOwnRange = (values: number[]): number[] => {
	if (values.length === 0) return [];

	const min = Math.min(...values);
	const max = Math.max(...values);
	const range = max - min;

	return values.map((value) => (range > 0 ? (value - min) / range : 1));
};

/*
metricValues reads every value a measurement row publishes for the given metric
names, keyed by name. A sparse row simply omits a metric it does not carry that
update, and a carried metric with no value reads the same way — an absent value
stays null (the readout shows a dash) rather than being fabricated as zero.
*/
const metricValues = (
	row: any,
	names: string[],
): Record<string, { raw: number; normalized: number } | null> => {
	const out: Record<string, { raw: number; normalized: number } | null> = {};
	for (const name of names) out[name] = null;
	if (!row) return out;

	if (Array.isArray(row.metrics)) {
		for (const m of row.metrics) {
			if (!m) continue;
			const name = m.name;
			if (!name || !names.includes(name)) continue;
			out[name] = {
				raw: m.raw,
				normalized: m.normalized,
			};
		}
		return out;
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

	const resonanceReadings = useSelector(signals.resonance, (state) => state);
	const rawState = useSelector(
		signals[active && !resonance ? source : ("" as keyof typeof signals)] ||
			signals.cvd,
		(state) => state,
	);
	const measurementState = (rawState as any)?.getBufferLength
		? rawState
		: ((rawState as any)?.[focusSymbol] ??
			(rawState as any)?.[""] ??
			Object.values(rawState || {})[0] ??
			null);

	if (!active) {
		return null;
	}

	const copy = kernelCopy(source, "");

	// Resonance is a presentation surface with no measurement vocabulary, so it
	// has no metric grid. Every measurement source names the metrics it
	// publishes. Each metric's readout is its most recent value found scanning
	// the buffer newest-first, so a metric absent from the newest sparse row
	// keeps its last value instead of flickering to a dash.
	const metrics = resonance ? [] : sourceMetrics(source);
	const metricReadouts = metrics.map((name) => {
		let raw: number | null = null;
		let normalized = 0;

		const count = (measurementState as any)?.getBufferLength
			? (measurementState as any).getBufferLength()
			: 0;
		let row: any = null;
		for (let i = count - 1; i >= 0; i--) {
			const candidate = (measurementState as any).get(i);
			if (candidate && metricValues(candidate, [name])[name] !== null) {
				row = candidate;
				break;
			}
		}
		const value = row ? metricValues(row, [name])[name] : null;

		if (value) {
			raw = value.raw;
			normalized = Math.min(1, Math.max(0, value.normalized));
		}

		return { name, raw, normalized };
	});

	// The headline names the metric this kernel leads with. Resonance is not a
	// measurement source and has none, so it names its own quantity instead of
	// borrowing one it does not publish.
	const headline = resonance
		? "predictive confidence"
		: (sourceHeadline(source) ?? "");

	const points = resonance
		? readingsFromNumbers(resonanceReadings)
		: readingsFromMeasurements(measurementState);
	const latest = points.length > 0 ? points[points.length - 1] : null;
	const status: SignalHealthStatus = latest === null ? "waiting" : "measured";
	const badge = kernelStatusMeta(status);

	// Confidence is already a real [0,1] quantity; only unbounded SNR needs
	// scaling against its own observed range before it reads as a trace.
	const relativePoints = resonance ? points : relativeToOwnRange(points);
	const paths = kernelSparkPaths(relativePoints, status);
	const level =
		relativePoints.length > 0 ? relativePoints[relativePoints.length - 1] : 0;

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
						variant={status === "measured" ? "info" : "disabled"}
						size="s"
						animated
					/>
					<Meter
						percent={points.length === 0 ? 0 : (points.length / 50) * 100}
						label="History"
						value={`${points.length} / 50`}
						variant={points.length === 0 ? "disabled" : "success"}
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
							{metricReadouts.map((metric: any) => (
								<Meter
									key={metric.name}
									percent={metric.raw === null ? 0 : metric.normalized * 100}
									label={metricLabel(metric.name)}
									value={metric.raw === null ? "—" : metric.raw.toFixed(4)}
									variant={metric.raw === null ? "disabled" : "info"}
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
							: `latest ${resonance ? "confidence" : "SNR"} ${latest.toFixed(resonance ? 3 : 2)}`}
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
