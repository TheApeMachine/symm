import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { useRef } from "react";
import { focusStore, measurementStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	sourceHeadlineMetric,
} from "#/components/terminal/kernel-meta";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Modal } from "#/components/ui/modal";
import { Typography } from "#/components/ui/typography";

export const KernelInspector = () => {
	const navigate = useNavigate();
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focusSymbol = useSelector(focusStore, (state) => state);
	const { closeInspect, selectSource } = terminalStore.actions;
	const root = useRef<HTMLDivElement>(null);

	const active = source !== null && source !== "";
	const headline = active
		? sourceHeadlineMetric(source).slice("metrics.".length)
		: "";

	measurementStore.subscribe((state) => {
		if (!root.current) return;
		const ring = active ? state[source]?.[focusSymbol] : undefined;
		const row = ring?.getLast();

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f="${q}"]`);
			if (el) el.textContent = value;
		};

		set("unit", headline === "" ? "" : headline);
		set("symbol", row?.symbol() ?? focusSymbol);
		set("at", row?.at() === undefined ? "" : new Date(Number(row.at())).toISOString().slice(11, 19));
	});

	if (!active) {
		return null;
	}

	const copy = kernelCopy(source, "");

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


