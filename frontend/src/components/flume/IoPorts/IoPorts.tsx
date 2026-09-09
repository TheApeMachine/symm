import React from "react";
import {
	ConnectionRecalculateContext,
	PortTypesContext,
} from "#/components/flume/context";
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

	if (!triggerRecalculation || !inputTypes) {
		return null;
	}

	return (
		<Flex.Column
			padding={1}
			fullWidth
			className="mt-auto"
			data-flume-component="ports"
		>
			{resolvedInputs.length ? (
				<Flex.Column
					align="stretch"
					data-flume-component="ports-inputs"
					fullWidth
					gap={3}
				>
					{resolvedInputs.map((input) => (
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
						/>
					))}
				</Flex.Column>
			) : null}
			{resolvedOutputs.length ? (
				<Flex.Row
					align="center"
					justify="end"
					data-flume-component="ports-outputs"
					fullWidth
				>
					{resolvedOutputs.map((output) => (
						<Output
							{...output}
							triggerRecalculation={triggerRecalculation}
							inputTypes={inputTypes}
							nodeId={nodeId}
							key={output.name}
						/>
					))}
				</Flex.Row>
			) : null}
		</Flex.Column>
	);
};

export default IoPorts;
