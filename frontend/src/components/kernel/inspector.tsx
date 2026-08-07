import { useNavigate } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import {
	kernelCopy,
	readinessGate,
	sourceHeadlineMetric,
} from "#/components/terminal/kernel-meta";
import { Component } from "#/components/ui/component";
import { Button } from "@/components/ui/button";
import { Flex } from "@/components/ui/flex";
import { Gate } from "@/components/ui/gate";
import { meterTrackVariants } from "@/components/ui/meter";
import { Modal } from "@/components/ui/modal";
import { Panel } from "@/components/ui/panel";
import { Sparkline } from "@/components/ui/sparkline";
import { Typography } from "@/components/ui/typography";

/*
KernelInspector shows one kernel's live reading over the focused symbol.

The trace is the shared Sparkline, so the curve here and the one on the kernel
row are the same shape drawn from the same appended series rather than two
sparklines that drifted apart. Every reading below it carries its own meter,
because a bare figure says what the kernel measured but not where that sits on
its own scale — which is the entire question the panel is open to answer.
*/
const METRICS = [
	{ label: "raw", suffix: ".raw", format: ".4f", meter: false },
	{ label: "normalized", suffix: ".normalized", format: ".4f", meter: true },
] as const;

const ROW_METRICS = [
	{ label: "maturity", bind: "maturity", format: ".3f" },
	{ label: "confidence", bind: "uncertainty.confidence", format: ".4f" },
] as const;

const Reading = ({
	label,
	bind,
	format,
	meter,
}: {
	label: string;
	bind: string;
	format: string;
	meter: boolean;
}) => (
	<Flex.Column className="min-w-0 flex-1 gap-1">
		<Flex.Row align="baseline" justify="between" className="gap-2">
			<Typography.Label size="xxs" tone="f4" weight="normal">
				{label}
			</Typography.Label>
			<Typography.Mono
				size="s"
				tone="f1"
				data-paint={bind}
				data-paint-format={format}
			/>
		</Flex.Row>
		{meter ? (
			<div className={meterTrackVariants({ variant: "warning", size: "xs" })}>
				{/*
					The fill is the reading itself, clamped in CSS: a kernel that
					reports past its own scale saturates rather than running out of
					the track.
				*/}
				<div
					data-set={bind}
					data-target="style.--reading"
					className="h-full bg-(--meter-tone)"
					style={{ width: "clamp(0%, calc(var(--reading, 0) * 100%), 100%)" }}
				/>
			</div>
		) : null}
	</Flex.Column>
);

export const KernelInspector = () => {
	const navigate = useNavigate();
	const source = useSelector(terminalStore, (state) => state.inspectorSource);
	const focusSymbol = useSelector(appStore, (state) => state.focusSymbol);
	const { closeInspect, selectSource } = terminalStore.actions;

	if (source === null || source === "") {
		return null;
	}

	const copy = kernelCopy(source, "");
	const headline = sourceHeadlineMetric(source);

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
		<Modal open onClose={closeInspect} size="m">
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
								<Component registerKey="readiness">
									{({ ref: gateRef }) => (
										<span ref={gateRef} className="contents">
											<Gate bind={readinessGate(source)} />
										</span>
									)}
								</Component>
							</Flex.Row>
							<Modal.Close
								aria-label="Close kernel inspector"
								onClick={closeInspect}
							/>
						</Modal.Header>

						<Modal.Body className="flex flex-col gap-3.5">
							<Flex.Column gap={1}>
								<Flex.Row align="baseline" justify="between" className="gap-2">
									<Typography.Label size="xxs" tone="f4">
										Signal history
									</Typography.Label>
									<Typography.Mono
										size="xxs"
										tone="f4"
										data-paint={`${headline}.unit`}
									/>
								</Flex.Row>
								<Panel size="bare" className="px-0 py-0">
									<Sparkline
										bind={`${headline}.normalized`}
										title="Signal history"
										limit={150}
										className="h-13"
									/>
								</Panel>
							</Flex.Column>

							<Flex.Row gap={4} className="items-start">
								{METRICS.map((metric) => (
									<Reading
										key={metric.label}
										label={metric.label}
										bind={`${headline}${metric.suffix}`}
										format={metric.format}
										meter={metric.meter}
									/>
								))}
							</Flex.Row>

							<Flex.Row gap={4} className="items-start">
								{ROW_METRICS.map((metric) => (
									<Reading
										key={metric.label}
										label={metric.label}
										bind={metric.bind}
										format={metric.format}
										meter
									/>
								))}
							</Flex.Row>

							<Typography.Paragraph className="text-[11px] text-(--f3) leading-relaxed">
								{copy.blurb}
							</Typography.Paragraph>
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
