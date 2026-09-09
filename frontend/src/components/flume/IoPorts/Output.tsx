import type { PortTypeMap } from "#/components/flume/types";
import { Field } from "#/components/ui/field";
import { Fieldset } from "#/components/ui/fieldset";
import Port from "./Port";

/*
Output renders a single output row: legend with the resolved label
and the port handle aligned to the right edge. Outputs never carry
controls — the value is computed by the engine, not entered by the
user — so this stays a thin wrapper around Port.
*/

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
		<Fieldset
			data-flume-component="port-output"
			className="flex align-center"
			data-controlless={true}
			onDragStart={(e) => {
				e.preventDefault();
				e.stopPropagation();
			}}
		>
			<Fieldset.Legend>{label || defaultLabel}</Fieldset.Legend>
			<Field className="flex align-center">
				<Port
					type={type}
					name={name}
					color={color}
					nodeId={nodeId}
					triggerRecalculation={triggerRecalculation}
				/>
			</Field>
		</Fieldset>
	);
};

export default Output;
