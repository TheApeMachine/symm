import type { PortTypeMap } from "#/components/flume/types";
import { Flex } from "#/components/ui";
import { Label } from "#/components/ui/label";
import Port from "./Port";

interface OutputProps {
	label: string;
	name: string;
	nodeId: string;
	type: string;
	inputTypes: PortTypeMap;
	triggerRecalculation: () => void;
	/* Present when this row stands for a gathering port's collapsed slots. */
	onToggleFamily?: () => void;
}

const Output = ({
	label,
	name,
	nodeId,
	type,
	inputTypes,
	triggerRecalculation,
	onToggleFamily,
}: OutputProps) => {
	const { label: defaultLabel, color } = inputTypes[type] || {};

	return (
		<Flex.Row
			align="center"
			gap={2}
			data-flume-component="port-output"
			data-controlless={true}
			onDragStart={(e) => {
				e.preventDefault();
				e.stopPropagation();
			}}
		>
			{onToggleFamily ? (
				<button
					type="button"
					onClick={onToggleFamily}
					className="cursor-pointer underline decoration-dotted underline-offset-2"
					data-flume-port-family={name}
				>
					<Label>{label || defaultLabel}</Label>
				</button>
			) : (
				<Label>{label || defaultLabel}</Label>
			)}
			<Port
				type={type}
				name={name}
				color={color}
				nodeId={nodeId}
				triggerRecalculation={triggerRecalculation}
			/>
		</Flex.Row>
	);
};

export default Output;
