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
}

const Output = ({
	label,
	name,
	nodeId,
	type,
	inputTypes,
	triggerRecalculation,
}: OutputProps) => {
	const { label: defaultLabel, color } = inputTypes[type] || {};

	return (
		<Flex.Row align="center" gap={2}
			data-flume-component="port-output"
			data-controlless={true}
			onDragStart={(e) => {
				e.preventDefault();
				e.stopPropagation();
			}}
		>
			<Label>{label || defaultLabel}</Label>
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
