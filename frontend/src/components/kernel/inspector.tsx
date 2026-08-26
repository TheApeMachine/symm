import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { MeasurementInspection } from "#/components/kernel/measurement-inspection";
import {
	kernelCopy,
	sourceHeadlineMetric,
} from "#/components/terminal/kernel-meta";
import { Button } from "@/components/ui/button";
import { Flex } from "@/components/ui/flex";
import { Modal } from "@/components/ui/modal";
import { Typography } from "@/components/ui/typography";
import { measurementsStore, useSubscribe } from "#/providers/ws-stores";

export const KernelInspector = () => {
	const navigate = useNavigate();
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const { closeInspect, selectSource } = terminalStore.actions;

	const active = source !== null && source !== "";
	const headline = active
		? sourceHeadlineMetric(source).slice("metrics.".length)
		: "";

	const root = useSubscribe(measurementsStore, (state) => {
		const row = active
			? state.measurements[`${source}\u0000${focusSymbol}`]?.latest()
			: undefined;

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);

			if (el instanceof HTMLElement) {
				el.textContent = value;
			}
		};

		set("unit", headline === "" ? "" : row?.metrics?.[headline]?.unit ?? "");
		set("symbol", row?.symbol ?? "");
		set("at", row?.at === undefined ? "" : new Date(row.at).toISOString().slice(11, 19));
	}, [source, focusSymbol, headline]);

	if (!active) {
		return null;
	}

	const copy = kernelCopy(source, "");
	const measurement =
		measurementsStore.state.measurements[`${source}\u0000${focusSymbol}`]?.latest() ?? null;

	const openInSignalInsight = () => {
		selectSource(source);
		closeInspect();
		navigate({ to: "/signals" });
	};

	return (
		<Modal open onClose={closeInspect} size="xl">
			<Flex.Column ref={root} className="min-h-0 flex-1 gap-0">
				<Modal.Header>
					<Flex.Row align="center" gap={2} className="min-w-0">
						<Typography.Display size="s" className="truncate">{copy.name}</Typography.Display>
					</Flex.Row>
					<Modal.Close aria-label="Close kernel inspector" onClick={closeInspect} />
				</Modal.Header>

				<Modal.Body className="flex flex-col gap-4">
					<Flex.Column gap={1}>
						<Flex.Row align="baseline" justify="between" className="gap-2">
							<Typography.Label size="xxs" tone="f4">Signal history</Typography.Label>
							<Typography.Mono size="xxs" tone="f4" data-f="unit" />
						</Flex.Row>
					</Flex.Column>

					<Typography.Paragraph className="text-[11px] text-(--f3) leading-relaxed">
						{copy.blurb}
					</Typography.Paragraph>

					<MeasurementInspection measurement={measurement} />
				</Modal.Body>

				<Modal.Footer>
					<Flex.Column className="min-w-0 gap-px">
						<Typography.Mono size="xxs" tone="f4" data-f="symbol" />
						<Typography.Mono size="xxs" tone="f4" data-f="at" />
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
		</Modal>
	);
};
