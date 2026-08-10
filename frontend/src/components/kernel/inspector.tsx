import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import type { Measurement } from "#/collections/types";
import { MeasurementInspection } from "#/components/kernel/measurement-inspection";
import {
	kernelCopy,
	metricLabel,
	sourceHeadlineMetric,
} from "#/components/terminal/kernel-meta";
import { Component } from "#/components/ui/component";
import type { JSONSerializable } from "#/components/ui/paint";
import { getLastFrame, registerPainter } from "#/providers/ws-stores";
import { Button } from "@/components/ui/button";
import { Flex } from "@/components/ui/flex";
import { Gate } from "@/components/ui/gate";
import { Modal } from "@/components/ui/modal";
import { Panel } from "@/components/ui/panel";
import { Sparkline } from "@/components/ui/sparkline";
import { Typography } from "@/components/ui/typography";

/*
KernelInspector shows one kernel's live reading over the focused symbol.

The trace is the shared Sparkline, so the curve here and the one on the kernel
row are the same shape drawn from the same appended series rather than two
sparklines that drifted apart. The decomposition reads the complete focused
measurement rather than a curated headline subset.
*/
const measurementRows = (frame: JSONSerializable): Measurement[] => {
	const rows = Array.isArray(frame)
		? frame
		: frame !== null && typeof frame === "object" && "source" in frame
			? [frame]
			: frame !== null && typeof frame === "object"
				? Object.values(frame)
				: [];

	return rows.filter(
		(row): row is Measurement =>
			row !== undefined &&
			row !== null &&
			typeof row === "object" &&
			!Array.isArray(row) &&
			typeof row.source === "string" &&
			typeof row.symbol === "string",
	);
};

const focusedMeasurement = (
	frame: JSONSerializable | undefined,
	source: string,
	symbol: string,
): Measurement | null => {
	if (frame === undefined) {
		return null;
	}

	return (
		measurementRows(frame).find(
			(measurement) =>
				measurement.source === source && measurement.symbol === symbol,
		) ?? null
	);
};

const useFocusedMeasurement = (source: string, symbol: string) => {
	const [measurement, setMeasurement] = useState<Measurement | null>(() =>
		focusedMeasurement(getLastFrame("measurements"), source, symbol),
	);

	useEffect(() => {
		const paint = (frame: JSONSerializable) => {
			const focused = focusedMeasurement(frame, source, symbol);

			if (focused !== null) {
				setMeasurement(focused);
			}
		};
		const unregister = registerPainter("measurements", paint);
		const retained = focusedMeasurement(
			getLastFrame("measurements"),
			source,
			symbol,
		);

		setMeasurement(retained);

		return unregister;
	}, [source, symbol]);

	return measurement;
};

export const KernelInspector = () => {
	const navigate = useNavigate();
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const { closeInspect, selectSource } = terminalStore.actions;
	const measurement = useFocusedMeasurement(source ?? "", focusSymbol);

	if (source === null || source === "") {
		return null;
	}

	const copy = kernelCopy(source, "");
	const headline = sourceHeadlineMetric(source).slice("metrics.".length);

	/*
		Opening the kernel on its own surface is a navigation, not a selection. It
		pinned the source and left the modal sitting over the page it had just
		asked to be taken to.
	*/
	const openInSignalInsight = () => {
		selectSource(source);
		closeInspect();
		navigate({ to: "/signals" });
	};

	return (
		<Modal open onClose={closeInspect} size="xl">
			<Component registerKey="measurements">
				{({ ref }) => (
					<Flex.Column
						ref={ref}
						data-scope="source,symbol"
						data-filter={`${source},${focusSymbol}`}
						/*
						The panel lays its chrome out as a column, so the wrapper this
						Component needs for its ref has to be that column rather than a
						plain box — otherwise the header stops shrinking and the body
						stops scrolling.
					*/
						className="min-h-0 flex-1 gap-0"
					>
						<Modal.Header>
							<Flex.Row align="center" gap={2} className="min-w-0">
								<Typography.Display size="s" className="truncate">
									{copy.name}
								</Typography.Display>
								<Gate bind="symbol" presence />
							</Flex.Row>
							<Modal.Close
								aria-label="Close kernel inspector"
								onClick={closeInspect}
							/>
						</Modal.Header>

						<Modal.Body className="flex flex-col gap-4">
							<Flex.Column gap={1}>
								<Flex.Row align="baseline" justify="between" className="gap-2">
									<Typography.Label size="xxs" tone="f4">
										Signal history
									</Typography.Label>
									<Typography.Mono
										size="xxs"
										tone="f4"
										data-paint={`metrics.${headline}.unit`}
									/>
								</Flex.Row>
								<Panel size="bare" className="px-0 py-0">
									<Sparkline
										bind={`metrics.${headline}.normalized`}
										title={`${metricLabel(headline)} history`}
										limit={150}
										className="h-13"
									/>
								</Panel>
							</Flex.Column>

							<Typography.Paragraph className="text-[11px] text-(--f3) leading-relaxed">
								{copy.blurb}
							</Typography.Paragraph>

							<MeasurementInspection measurement={measurement} />
						</Modal.Body>

						<Modal.Footer>
							<Flex.Column className="min-w-0 gap-px">
								<Typography.Mono size="xxs" tone="f4" data-paint="symbol" />
								<Typography.Mono
									size="xxs"
									tone="f4"
									data-paint="at"
									data-paint-format="time"
								/>
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
					</Flex.Column>
				)}
			</Component>
		</Modal>
	);
};
