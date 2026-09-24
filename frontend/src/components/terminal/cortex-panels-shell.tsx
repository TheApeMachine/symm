import { signals } from "#/collections/app";
import { Badge } from "#/components/ui/badge";
import { Chip } from "#/components/ui/chip";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Meter } from "#/components/ui/meter";
import { usePaintStore } from "#/components/ui/paint";
import { Panel } from "#/components/ui/panel";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";

export const CortexPanelsShell = ({ symbol }: { symbol: string }) => {
	const rootRef = usePaintStore(
		signals.cognition,
		(state) => {
			const targetRow = state[symbol]?.getLast() ?? null;
			const conf = targetRow?.confidence();
			const entropyBits = targetRow?.entropyBits();

			return {
				fields: {
					winner: targetRow?.winner() ?? "—",
					confidence: targetRow
						? `${(targetRow.confidence() * 100).toFixed(1)}%`
						: "—",
					contrast: targetRow ? targetRow.contrast().toFixed(3) : "—",
					entropy: targetRow ? targetRow.entropyBits().toFixed(3) : "—",
					ambiguous: targetRow ? String(targetRow.ambiguous()) : "—",
					remFrom: targetRow ? String(targetRow.remFrom()) : "—",
					remThrough: targetRow ? String(targetRow.remThrough()) : "—",
					remReplays: targetRow ? String(targetRow.remReplays()) : "—",
					replays: targetRow ? String(targetRow.remReplays()) : "—",
				},
				meters: {
					basin:
						typeof conf === "number"
							? Math.min(100, Math.max(0, conf * 100))
							: 0,
					entropy:
						typeof entropyBits === "number"
							? Math.min(100, Math.max(0, entropyBits * 100))
							: 0,
				},
			};
		},
		[symbol],
	);

	return (
		<Flex.Column ref={rootRef} gap={3.5 as any}>
			<Panel>
				<Panel.Header
					title="Attractor basin · classify"
					meta={<Badge label="classify" variant="warning" />}
				/>
				<Panel.Caption>softmax posterior · b/[class]/[sequence]</Panel.Caption>
				<Flex.Column gap={2}>
					<Flex.Row justify="between" align="center">
						<Typography.Span
							data-f="winner"
							variant="f3"
							className="text-[10px]"
						/>
						<Typography.Span
							data-f="confidence"
							variant="f1"
							className="text-[10px]"
						/>
					</Flex.Row>
					<Meter
						layout="bar"
						variant="info"
						size="m"
						percent={0}
						data-meter="basin"
						data-basin
					/>
				</Flex.Column>
			</Panel>

			<Panel>
				<Panel.Header title="Contrastive evidence" />
				<Panel.Caption>routing margin · winner vs runner-up</Panel.Caption>
				<Grid cols={2} gap={2} responsive={false} className="text-center">
					<Stat
						layout="feature"
						label="contrast"
						value={<Typography.Span data-f="contrast" />}
						variant="warning"
					/>
					<Stat
						layout="feature"
						label="entropy bits"
						value={<Typography.Span data-f="entropy" />}
						variant="warning"
					/>
				</Grid>
			</Panel>

			<Panel>
				<Panel.Header
					title="Branch entropy gate"
					meta={<Typography.Label size="xs" tone="f3" data-f="ambiguous" />}
				/>
				<Panel.Caption>shannon H vs uniform threshold</Panel.Caption>
				<Meter
					layout="bar"
					variant="success"
					size="m"
					percent={0}
					data-meter="entropy"
					data-entropy
				/>
			</Panel>

			<Panel>
				<Panel.Header
					title="REM consolidation"
					meta={
						<Chip data-consolidating label={<Typography.Span data-replays />} />
					}
				/>
				<Panel.Caption>
					episodic replay · decay · retroactive inhibition
				</Panel.Caption>
				<Grid cols={3} gap={2} responsive={false}>
					<Stat
						layout="feature"
						label="from"
						value={<Typography.Span data-f="remFrom" />}
					/>
					<Stat
						layout="feature"
						label="replays"
						value={<Typography.Span data-f="remReplays" />}
					/>
					<Stat
						layout="feature"
						label="through"
						value={<Typography.Span data-f="remThrough" />}
					/>
				</Grid>
			</Panel>
		</Flex.Column>
	);
};
