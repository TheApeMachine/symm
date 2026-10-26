import { useSelector } from "@tanstack/react-store";
import { atoms } from "#/collections/app";
import {
	kernelCopy,
	metricLabel,
	sourceMetrics,
} from "#/components/terminal/kernel-meta";
import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { useKernelReading } from "#/components/ui/metric";
import { Typography } from "#/components/ui/typography";

export const SignalDetail = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const selected = useSelector(atoms.kernelDetail, (state) => state);
	const symbols = useSelector(atoms.symbols, (state) => state) as string[];

	const source = selected || "cvd";
	const reading = useKernelReading(source);
	const copy =
		source === ""
			? { name: "Signal detail", sub: "", blurb: "" }
			: kernelCopy(source, "");
	const metrics = source === "" ? [] : sourceMetrics(source);

	if (source === "") {
		return (
			<Flex.Column className="min-h-0 px-5 py-4.5">
				<Typography.Display size="xl">{copy.name}</Typography.Display>
				<Typography.Paragraph className="mt-3.5 text-[12px] text-(--f3)">
					Select a kernel to inspect its live measurements.
				</Typography.Paragraph>
			</Flex.Column>
		);
	}

	return (
		<Component
			key={`${source}:${focusSymbol}`}
			value={reading}
			className="min-h-0 overflow-auto px-5 py-4.5"
		>
			<Flex.Row className="items-start justify-between gap-3">
				<Flex.Column className="min-w-0 gap-1">
					<Typography.Display size="xl">{copy.name}</Typography.Display>
					<Typography.Mono size="s" tone="f4">
						{copy.sub}
					</Typography.Mono>
				</Flex.Column>
			</Flex.Row>

			<Typography.Paragraph className="mt-3.5 max-w-prose text-[12px] text-(--f3) leading-relaxed">
				{copy.blurb}
			</Typography.Paragraph>

			<Grid cols={2} gapY={3} className="mt-4.5 gap-x-5.5">
				{metrics.map((metric) => (
					<div key={metric}>
						<Flex.Row justify="between" align="center" className="mb-1.5 gap-2">
							<Typography.Label size="xxs" tone="f3" weight="normal">
								{metricLabel(metric)}
							</Typography.Label>
							<Typography.Mono
								size="s"
								tone="f1"
								data-metric-raw={metric}
								data-format="fixed:4"
							>
								—
							</Typography.Mono>
						</Flex.Row>
						<div className="h-1.5 overflow-hidden rounded-xs bg-(--line)">
							<div
								data-mbar={metric}
								className="h-full bg-(--acc) transition-[width] duration-500 ease-out"
								style={{ width: "0%" }}
							/>
						</div>
					</div>
				))}
			</Grid>

			<Grid
				cols={2}
				gapY={2}
				className="mt-5 gap-x-5.5 border-(--line) border-t pt-3.5 font-mono text-xs"
			>
				<Flex.Row justify="between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Symbol
					</Typography.Label>
					<Typography.Span data-metric="symbol" className="text-(--f1)">
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Observed
					</Typography.Label>
					<Typography.Span data-metric="at" className="text-(--f1)">
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Maturity
					</Typography.Label>
					<Typography.Span
						data-metric="maturity"
						data-format="fixed:3"
						className="text-(--f1)"
					>
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Peer
					</Typography.Label>
					<Typography.Span data-metric="peer" className="text-(--f1)">
						—
					</Typography.Span>
				</Flex.Row>
			</Grid>

			<Grid cols={2} gapY={2} className="mt-3.5 gap-x-5.5 font-mono text-xs">
				<Flex.Row justify="between">
					<Typography.Label size="xxs" tone="f3" weight="normal">
						Readings this epoch
					</Typography.Label>
					<Typography.Span data-metric="epoch" className="text-(--f1)">
						—
					</Typography.Span>
				</Flex.Row>
			</Grid>

			{symbols.length === 0 ? null : (
				<div className="mt-5">
					<Typography.Label
						size="xxs"
						tone="f3"
						className="mb-2 block tracking-[0.13em]"
					>
						Cross-section · {source} headline
					</Typography.Label>
					<div className="grid grid-cols-12 gap-0.75">
						{symbols.slice(0, 24).map((symbol) => (
							<Flex.Row
								key={symbol}
								title={`${symbol} · ${source}`}
								align="center"
								justify="center"
								className="aspect-square overflow-hidden rounded-xs border border-(--line) font-mono text-[8px] text-(--f3)"
							>
								{symbol.replace(/\/.*$/, "")}
							</Flex.Row>
						))}
					</div>
				</div>
			)}
		</Component>
	);
};
