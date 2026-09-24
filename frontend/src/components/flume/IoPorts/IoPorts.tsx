import React from "react";
import {
	ConnectionRecalculateContext,
	PortTypesContext,
} from "#/components/flume/context";
import { groupPorts } from "#/components/flume/port-families";
import type {
	Connections,
	InputData,
	PortType,
	TransputBuilder,
} from "#/components/flume/types";
import { Flex } from "#/components/ui/flex";
import Input from "./Input";
import Output from "./Output";
import { useTransputs } from "./useTransputs";

/*
IoPorts is the container that arranges a node's input and output rows.
The heavy lifting lives in the focused sibling modules:
  - Input.tsx — input row, label + per-control rendering
  - Output.tsx — output row, label + port
  - Port.tsx — port handle (anchor + portal'd button)
  - usePortDrag.ts — drag-line state machine for active connections
  - useTransputs.ts — resolves dynamic vs static port arrays + cleanup
*/

interface IoPortsProps {
	nodeId: string;
	inputs: PortType[] | TransputBuilder;
	outputs: PortType[] | TransputBuilder;
	connections: Connections;
	inputData: InputData;
	updateNodeConnections: () => void;
}

const IoPorts = ({
	nodeId,
	inputs = [],
	outputs = [],
	connections,
	inputData,
	updateNodeConnections,
}: IoPortsProps) => {
	const inputTypes = React.useContext(PortTypesContext);
	const triggerRecalculation = React.useContext(ConnectionRecalculateContext);
	const resolvedInputs = useTransputs(
		inputs,
		"input",
		nodeId,
		inputData,
		connections,
	);
	const resolvedOutputs = useTransputs(
		outputs,
		"output",
		nodeId,
		inputData,
		connections,
	);

	// A gathering port's slots are drawn as the one port they belong to,
	// unless the node is opened up to show them.
	const [openFamilies, setOpenFamilies] = React.useState<string[]>([]);

	if (!triggerRecalculation || !inputTypes) {
		return null;
	}

	const shown = (ports: typeof resolvedInputs) =>
		groupPorts(ports).flatMap((group) => {
			if (group.members.length === 1) {
				return group.members;
			}

			if (openFamilies.includes(group.base)) {
				return group.members;
			}

			return [
				{
					...group.representative,
					label: `${group.representative.label ?? group.base} (${group.members.length})`,
					family: group.base,
					slots: group.members.length,
				},
			];
		});

	const toggleFamily = (base: string) =>
		setOpenFamilies((open) =>
			open.includes(base)
				? open.filter((name) => name !== base)
				: [...open, base],
		);

	const shownInputs = shown(resolvedInputs);
	const shownOutputs = shown(resolvedOutputs);

	return (
		<Flex.Column
			padding={1}
			fullWidth
			className="mt-auto"
			data-flume-component="ports"
		>
			{shownInputs.length ? (
				<Flex.Column
					align="stretch"
					justify="start"
					data-flume-component="ports-inputs"
					fullWidth
					gap={3}
				>
					{shownInputs.map((input) => (
						<Input
							{...input}
							data={inputData[input.name] || {}}
							isConnected={!!connections.inputs[input.name]}
							triggerRecalculation={triggerRecalculation}
							updateNodeConnections={updateNodeConnections}
							inputTypes={inputTypes}
							nodeId={nodeId}
							inputData={inputData}
							key={input.name}
							onToggleFamily={
								"family" in input
									? () => toggleFamily(String(input.family))
									: undefined
							}
						/>
					))}
				</Flex.Column>
			) : null}
			{shownOutputs.length ? (
				<Flex.Column align="end" gap={2} data-flume-component="ports-outputs">
					{shownOutputs.map((output) => (
						<Output
							{...output}
							triggerRecalculation={triggerRecalculation}
							inputTypes={inputTypes}
							nodeId={nodeId}
							key={output.name}
							onToggleFamily={
								"family" in output
									? () => toggleFamily(String(output.family))
									: undefined
							}
						/>
					))}
				</Flex.Column>
			) : null}
		</Flex.Column>
	);
};

export default IoPorts;
