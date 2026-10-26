import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";
import { atoms, kernelMeasurement, signals } from "#/collections/app";
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
import { meterPercent } from "#/components/ui/dom";
import { Flex } from "#/components/ui/flex";
import { Meter } from "#/components/ui/meter";
import { Modal } from "#/components/ui/modal";
import { scaleSparklineValues } from "#/components/ui/sparkline";
import { Typography } from "#/components/ui/typography";
import type { Value } from "#/types/value";

const isResonance = (source: string) => source === "resonance";

/*
readings collects the accumulated values for one kernel directly from its store.
*/
const readingsFromValues = (frame: Value | null, source?: string) => {
	const points: number[] = [];
	if (!frame) return points;

	// The kernel publishes one scalar layer per metric key.
	const metrics = frame.metrics();
	const headline = source ? sourceHeadline(source) : null;
	const keys = Object.keys(metrics);

	const pick = (key: string): number | null => {
		const chosen = Number(metrics[key]);
		return Number.isFinite(chosen) ? chosen : null;
	};

	const val =
		(headline && keys.includes(headline) ? pick(headline) : null) ??
		(keys.length ? pick(keys[0] ?? "") : null);

	if (val !== null) points.push(val);
	return points;
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
	const metrics = row.metrics();
	for (const name of names) {
		const value = metrics[name];
		if (typeof value !== "number" || !Number.isFinite(value)) continue;
		out[name] = { raw: value, snm: value };
	}

	return out;
};

export const KernelInspector = () => {
	const navigate = useNavigate();
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const { closeInspect, selectSource } = terminalStore.actions;

	const active = source !== null && source !== "";
	const resonance = active && isResonance(source);

	/*
	Kernels publish into the workspace tick; resonance is the one origin
	with a store of its own. The frame is the latest reading, the sparkline
	series is chart-local history the modal folds frame by frame.
	*/
	const aggregate = useSelector(
		signals.workspace,
		(state) => state.aggregate,
	);
	const frame = useSelector(signals.resonance, (state) => {
		if (!resonance) return undefined;
		return state[focusSymbol.replace(/\//g, "-")] ?? state[focusSymbol];
	});
	const signalReading =
		!resonance && source
			? (kernelMeasurement(source, focusSymbol, aggregate) ?? null)
			: null;
	const kernelFrame = resonance ? (frame ?? null) : signalReading;

	const [signalHistory, setSignalHistory] = useState<number[]>([]);

	useEffect(() => {
		if (!active || resonance || !source) {
			setSignalHistory([]);
			return;
		}
		const sample = readingsFromValues(signalReading, source);
		const next = sample.at(-1);
		if (next === undefined) return;
		setSignalHistory((previous) => [...previous, next].slice(-50));
	}, [active, resonance, source, signalReading]);

	if (!active) {
		return null;
	}

	const copy = kernelCopy(source, "");
	const livePoints = readingsFromValues(kernelFrame, source);
	const points = resonance
		? livePoints
		: signalHistory.length > 0
			? signalHistory
			: livePoints;
	const latest = points.length > 0 ? points[points.length - 1] : null;
	const status: SignalHealthStatus = latest === null ? "waiting" : "measured";
	const badge = kernelStatusMeta(status);

	const relativePoints = resonance ? points : scaleSparklineValues(points);
	const paths = kernelSparkPaths(relativePoints, status);
	const level = meterPercent(latest);

	const metrics = resonance ? [] : sourceMetrics(source);
	const metricReadouts = metrics.map((name) => {
		let raw: number | null = null;
		let snm = 0;

		const value = kernelFrame ? metricValues(kernelFrame, [name])[name] : null;

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
		? `${((latest ?? 0) * 100).toFixed(0)}%`
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
						percent={level}
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
									percent={metric.raw === null ? 0 : meterPercent(metric.snm)}
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
