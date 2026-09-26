import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { selectPositions } from "#/components/dashboard/positions";
import { sendPositionExit } from "#/providers/websocket";
import { type OpenPosition, PositionList } from "./position-list";

export type PositionsProps = {
	positions?: OpenPosition[] | any;
	className?: string;
};

export const Positions = ({
	positions: propPositions,
	className,
}: PositionsProps = {}) => {
	const streamPositions = useSelector(signals.position, selectPositions);
	const positions =
		propPositions !== undefined
			? Array.isArray(propPositions)
				? propPositions
				: propPositions?.toArray
					? propPositions.toArray()
					: []
			: streamPositions;

	const handleExit = (symbol: string) => {
		sendPositionExit(symbol);
	};

	const handleInspect = (symbol: string) => {
		terminalStore.actions.openThesis(symbol);
	};

	return (
		<PositionList
			positions={positions}
			onExit={handleExit}
			onInspect={handleInspect}
			className={className}
		/>
	);
};
