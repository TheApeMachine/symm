import { useSelector } from "@tanstack/react-store";
import { atoms, signals } from "#/collections/app";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { asFinite } from "#/lib/format";
import { parseResonanceData, resonanceAt } from "#/lib/resonance";

const cell = (value: unknown, digits: number): string => {
	const n = asFinite(value);
	return n === null ? "—" : n.toFixed(digits);
};

export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(atoms.focus, (state) => state);
	const row = useSelector(signals.resonance, (state) =>
		resonanceAt(state, focusSymbol),
	);
	const res = row ? parseResonanceData(row) : undefined;

	return (
		<Flex.Column className="mt-2 gap-2.5 border-(--line) border-t px-3.5 py-3 font-mono text-[12px]">
			<Flex.Row justify="between" gap={3}>
				<Typography.Label size="xs" tone="f3" weight="normal">
					symbol
				</Typography.Label>
				<Typography.Span className="text-right text-(--acc)">
					{focusSymbol || "—"}
				</Typography.Span>
			</Flex.Row>
			<Flex.Row justify="between" gap={3}>
				<Typography.Label size="xs" tone="f3" weight="normal">
					energy
				</Typography.Label>
				<Typography.Span className="text-right text-(--f1)">
					{cell(res?.energy, 4)}
				</Typography.Span>
			</Flex.Row>
			<Flex.Row justify="between" gap={3}>
				<Typography.Label size="xs" tone="f3" weight="normal">
					surprise
				</Typography.Label>
				<Typography.Span className="text-right text-(--f1)">
					{cell(res?.surprise, 4)}
				</Typography.Span>
			</Flex.Row>
			<Flex.Row justify="between" gap={3}>
				<Typography.Label size="xs" tone="f3" weight="normal">
					task precision
				</Typography.Label>
				<Typography.Span className="text-right text-(--f1)">
					{cell(res?.taskRelativePrecision, 4)}
				</Typography.Span>
			</Flex.Row>
			<Flex.Row justify="between" gap={3}>
				<Typography.Label size="xs" tone="f3" weight="normal">
					supported horizon
				</Typography.Label>
				<Typography.Span className="text-right text-(--f1)">
					{res?.supportedHorizon != null
						? String(Math.trunc(res.supportedHorizon))
						: "—"}
				</Typography.Span>
			</Flex.Row>
			<Flex.Row justify="between" gap={3}>
				<Typography.Label size="xs" tone="f3" weight="normal">
					reach
				</Typography.Label>
				<Typography.Span className="text-right text-(--f1)">
					{res ? String(res.forwardCurve.length) : "—"}
				</Typography.Span>
			</Flex.Row>
		</Flex.Column>
	);
};