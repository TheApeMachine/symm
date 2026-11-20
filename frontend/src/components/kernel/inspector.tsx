import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { atoms, signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	kernelStatusMeta,
	kernelStatusVariant,
	metricLabel,
	sourceHeadline,
	sourceMetrics,
} from "#/components/terminal/kernel-meta";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { meterPercent } from "#/components/ui/dom";
import { Flex } from "#/components/ui/flex";
import { Meter } from "#/components/ui/meter";
import { Modal } from "#/components/ui/modal";
import { Sparkline } from "#/components/ui/sparkline";
import { Typography } from "#/components/ui/typography";
import { asFinite } from "#/lib/format";

export const KernelInspector = () => {
	const navigate = useNavigate();
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focus = useSelector(atoms.focus, (state) => state);
	const { closeInspect, selectSource } = terminalStore.actions;

	const resonance = source === "resonance";
	const store =
		source && signals[source] ? signals[source] : signals.resonance;
	const headline = source ? sourceHeadline(source) : null;

	const row = useSelector(store, (state) =>
		source ? state?.[focus] : undefined,
	);

	if (!source) {
		return null;
	}

	const record =
		row && typeof row === "object" && !Array.isArray(row)
			? (row as Record<string, unknown>)
			: null;

	let sample: number | null = null;
	if (record) {
		if (!resonance && headline) {
			sample = asFinite(record[headline]);
		} else {
			for (const value of Object.values(record)) {
				sample = asFinite(value);
				if (sample !== null) break;
			}
		}
	}

	const copy = kernelCopy(source, "");
	const live = sample !== null;
	const status = live ? "measured" : "waiting";
	const badge = kernelStatusMeta(status);

	const metricNames = resonance ? [] : sourceMetrics(source);
	const metricReadouts = metricNames.map((name) => ({
		name,
		raw: record ? asFinite(record[name]) : null,
	}));

	const headlineLabel = resonance
		? "predictive confidence"
		: (headline ?? "");

	const valueLabel = resonance
		? `${((sample ?? 0) * 100).toFixed(0)}%`
		: sample !== null
			? sample.toFixed(2)
			: "—";

	const openInSignalInsight = () => {
		selectSource(source);
		closeInspect();
		navigate({ to: "/" });
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
					<Typography.Label size="xxs" tone="f4">
						Latest reading
					</Typography.Label>
					<Sparkline
						points={sample !== null ? [sample] : []}
						active={live}
						title={`${copy.name} reading`}
						className="block h-13 w-full rounded-[3px] border border-(--line) bg-(--sunken)"
					/>
				</Flex.Column>

				<Meter
					percent={meterPercent(sample)}
					label={resonance ? "Confidence" : "Level"}
					value={valueLabel}
					variant={live ? "warning" : "disabled"}
					size="s"
					animated
				/>

				{metricNames.length === 0 ? null : (
					<Flex.Column gap={2}>
						<Typography.Label size="xxs" tone="f4">
							Signal metrics
						</Typography.Label>
						<div className="grid grid-cols-2 gap-x-3 gap-y-2">
							{metricReadouts.map((metric) => (
								<Meter
									key={metric.name}
									percent={
										metric.raw === null ? 0 : meterPercent(metric.raw)
									}
									label={metricLabel(metric.name)}
									value={
										metric.raw === null ? "—" : metric.raw.toFixed(4)
									}
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
						{focus}
						{headlineLabel === "" ? "" : ` · ${headlineLabel}`}
					</Typography.Mono>
					<Typography.Mono size="xxs" tone="f4">
						{sample !== null
							? `latest ${resonance ? "confidence" : (headline ?? "reading")} ${sample.toFixed(resonance ? 3 : 4)}`
							: "awaiting first reading"}
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