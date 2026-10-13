import { useSelector } from "@tanstack/react-store";
import { focusAtom, signals } from "#/collections/app";
import { Component } from "#/components/ui/component";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";

export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(focusAtom);

	return (
		<Component
			store={signals.resonance}
			symbol={focusSymbol}
			className="flex h-full flex-col"
		>
			<Flex.Column
				className="mt-2 gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]"
			>
				<Flex.Row justify="between" gap={3}>
					<Typography.Label size="xs" tone="f3" weight="normal">
						symbol
					</Typography.Label>
					<Typography.Span data-metric="symbol" className="text-right text-(--acc)">
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between" gap={3}>
					<Typography.Label size="xs" tone="f3" weight="normal">
						energy
					</Typography.Label>
					<Typography.Span
						data-metric-raw="energy"
						data-format="fixed:4"
						className="text-right text-(--f1)"
					>
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between" gap={3}>
					<Typography.Label size="xs" tone="f3" weight="normal">
						surprise
					</Typography.Label>
					<Typography.Span
						data-metric-raw="surprise"
						data-format="fixed:4"
						className="text-right text-(--f1)"
					>
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between" gap={3}>
					<Typography.Label size="xs" tone="f3" weight="normal">
						confidence
					</Typography.Label>
					<Typography.Span
						data-metric="confidence"
						data-format="pct"
						className="text-right text-(--f1)"
					>
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between" gap={3}>
					<Typography.Label size="xs" tone="f3" weight="normal">
						supported horizon
					</Typography.Label>
					<Typography.Span
						data-metric="supported_horizon"
						data-format="int"
						className="text-right text-(--f1)"
					>
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between" gap={3}>
					<Typography.Label size="xs" tone="f3" weight="normal">
						resolved steps
					</Typography.Label>
					<Typography.Span
						data-metric="resolved_steps"
						data-format="int"
						className="text-right text-(--f1)"
					>
						—
					</Typography.Span>
				</Flex.Row>
				<Flex.Row justify="between" gap={3}>
					<Typography.Label size="xs" tone="f3" weight="normal">
						maturity
					</Typography.Label>
					<Typography.Span
						data-metric="maturity"
						data-format="int"
						className="text-right text-(--f3) text-[10px]"
					>
						—
					</Typography.Span>
				</Flex.Row>
			</Flex.Column>
		</Component>
	);
};
