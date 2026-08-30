import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import {
	focusStore,
	getKernelReadingStore,
	getResonanceReadingStore,
} from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelSparkPaths,
	kernelStatusMeta,
	kernelStatusVariant,
	type SignalHealthStatus,
	sourceHeadline,
} from "#/components/terminal/kernel-meta";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Meter } from "#/components/ui/meter";
import { Modal } from "#/components/ui/modal";
import { Typography } from "#/components/ui/typography";

const isResonance = (source: string) => source === "resonance";

/*
readings collects the accumulated values for one kernel from the same rings the
kernel list reads, so the panel that opens on a row agrees with the row itself
rather than deriving a second, differently-shaped history.
*/
const readings = (ring: { getBufferLength: () => number; get: (index: number) => number | undefined }) => {
	const points: number[] = [];
	const len = ring.getBufferLength();

	for (let i = 0; i < len; i++) {
		const value = ring.get(i);

		if (value !== undefined && Number.isFinite(value)) {
			points.push(value);
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

export const KernelInspector = () => {
	const navigate = useNavigate();
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focusSymbol = useSelector(focusStore, (state) => state);
	const { closeInspect, selectSource } = terminalStore.actions;

	const active = source !== null && source !== "";
	const resonance = active && isResonance(source);

	// Both selectors run unconditionally (hooks cannot be conditional); only
	// one of their results is used. An inactive panel reads an empty ring under
	// a harmless placeholder key rather than skipping the hook.
	const measurementReadings = useSelector(
		getKernelReadingStore(active ? source : ""),
		(state) => state,
	);
	const resonanceReadings = useSelector(
		getResonanceReadingStore(focusSymbol),
		(state) => state,
	);

	if (!active) {
		return null;
	}

	const copy = kernelCopy(source, "");
	const points = readings(resonance ? resonanceReadings : measurementReadings);
	const latest = points.length > 0 ? points[points.length - 1] : null;
	const status: SignalHealthStatus = latest === null ? "waiting" : "measured";
	const badge = kernelStatusMeta(status);

	// Confidence is already a real [0,1] quantity; only unbounded SNR needs
	// scaling against its own observed range before it reads as a trace.
	const relativePoints = resonance ? points : relativeToOwnRange(points);
	const paths = kernelSparkPaths(relativePoints, status);
	const level =
		relativePoints.length > 0 ? relativePoints[relativePoints.length - 1] : 0;

	// The headline names the metric this kernel leads with. Resonance is not a
	// measurement source and has none, so it names its own quantity instead of
	// borrowing one it does not publish.
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
				<Modal.Close aria-label="Close kernel inspector" onClick={closeInspect} />
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
