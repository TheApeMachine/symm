import { useSelector } from "@tanstack/react-store";
import { focusAtom, signals } from "#/collections/app";
import { DataRow } from "#/components/ui/data-row";
import { Flex } from "#/components/ui/flex";
import { usePaintStore } from "#/components/ui/paint";

export const XrayFactsPanel = () => {
	const focusSymbol = useSelector(focusAtom, (state) => state);

	const rootRef = usePaintStore(
		signals.cognition,
		(state) => {
			const targetRow = state[focusSymbol]?.getLast();
			return {
				fields: {
					winner: targetRow ? targetRow.winner() || "none named" : "—",
					confidence: targetRow
						? `${(targetRow.confidence() * 100).toFixed(1)}%`
						: "—",
					contrast: targetRow ? targetRow.contrast().toFixed(3) : "—",
					entropy: targetRow ? targetRow.entropyBits().toFixed(3) : "—",
					ambiguous: targetRow ? String(targetRow.ambiguous()) : "—",
					sequence: targetRow ? targetRow.sequence() || "none" : "—",
				},
			};
		},
		[focusSymbol],
	);

	return (
		<Flex.Column ref={rootRef} fullHeight>
			<DataRow.Group
				border={false}
				className="mt-2 border-(--line) border-t px-3.5 py-3 gap-2.5 font-mono text-[12px]"
			>
				<DataRow
					label="regime class"
					value="—"
					paintKey="winner"
					tone="accent"
					density="bare"
				/>
				<DataRow
					label="coherence"
					value="—"
					paintKey="confidence"
					tone="f1"
					density="bare"
				/>
				<DataRow
					label="class contrast"
					value="—"
					paintKey="contrast"
					tone="f1"
					density="bare"
				/>
				<DataRow
					label="entropy bits"
					value="—"
					paintKey="entropy"
					tone="f1"
					density="bare"
				/>
				<DataRow
					label="ambiguous"
					value="—"
					paintKey="ambiguous"
					tone="default"
					density="bare"
				/>
				<DataRow
					label="sequence"
					value="—"
					paintKey="sequence"
					tone="f3"
					size="xs"
					valueClassName="max-w-42 truncate"
					density="bare"
					title="DMT token sequence the classifier is reading"
				/>
			</DataRow.Group>
		</Flex.Column>
	);
};
