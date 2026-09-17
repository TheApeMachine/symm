import { useSelector } from "@tanstack/react-store";
import { terminalStore } from "#/collections/terminal";
import { EntryDecisionSnapshot } from "#/components/terminal/entry-decision-snapshot";
import { ThesisDetailRail } from "#/components/terminal/thesis-detail-rail";
import { Badge } from "#/components/ui/badge";
import { Flex } from "#/components/ui/flex";
import { Grid } from "#/components/ui/grid";
import { Modal } from "#/components/ui/modal";
import { Rail } from "#/components/ui/rail";
import { Typography } from "#/components/ui/typography";

export const openThesisShell = (symbol: string) => {
	terminalStore.actions.openThesis(symbol);
};

export const closeThesisShell = () => {
	terminalStore.actions.closeThesis();
};

export const ThesisModal = () => {
	const symbol = useSelector(terminalStore, (state) => state.thesisSymbol);

	if (symbol === null || symbol === "") {
		return null;
	}

	return (
		<Modal
			open={true}
			onClose={closeThesisShell}
			size="xl"
			panelClassName="h-[min(90vh,960px)] w-[min(1320px,96vw)] max-w-none rounded-lg shadow-[0_28px_72px_-18px_rgba(0,0,0,0.78)]"
		>
			<Modal.Header className="items-center px-5 py-3.5">
				<Flex.Row align="center" gap={3}>
					<Typography.Display size="lg">{symbol}</Typography.Display>
					<Badge
						label="Entry decision snapshot"
						variant="warning"
						size="s"
						className="font-mono tracking-wide"
					/>
				</Flex.Row>

				<Modal.Close onClick={closeThesisShell} />
			</Modal.Header>

			<Grid
				cols={2}
				responsive={false}
				className="min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(300px,360px)]"
			>
				<Flex.Column
					fullHeight
					className="min-h-0 overflow-y-auto border-(--line) border-r"
				>
					<EntryDecisionSnapshot symbol={symbol} />
				</Flex.Column>

				<Rail
					position="none"
					surface="transparent"
					className="min-h-0 overflow-y-auto p-3.5"
				>
					<ThesisDetailRail symbol={symbol} />
				</Rail>
			</Grid>
		</Modal>
	);
};
