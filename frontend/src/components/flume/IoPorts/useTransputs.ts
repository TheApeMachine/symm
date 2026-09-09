import React from "react";
import { ContextContext, NodeActionsContext } from "#/components/flume/context";
import type {
	Connections,
	InputData,
	PortType,
	TransputBuilder,
	TransputType,
} from "#/components/flume/types";
import usePrevious from "#/hooks/usePrevious";

/*
useTransputs resolves the runtime list of input or output ports for a
node and emits destroyTransput actions when a previously-present
dynamic port disappears. Static array port definitions skip the
destroy-on-removal path because the registry guarantees those ports
exist for the lifetime of the node type.
*/

export const useTransputs = (
	transputsFn: PortType[] | TransputBuilder,
	transputType: TransputType,
	nodeId: string,
	inputData: InputData,
	connections: Connections,
) => {
	const nodeActions = React.useContext(NodeActionsContext);
	const executionContext = React.useContext(ContextContext);

	const transputs = React.useMemo(() => {
		if (Array.isArray(transputsFn)) {
			return transputsFn;
		}

		return transputsFn(inputData, connections, executionContext);
	}, [transputsFn, inputData, connections, executionContext]);

	const prevTransputs = usePrevious<PortType[]>(transputs);

	React.useEffect(() => {
		if (!prevTransputs || Array.isArray(transputsFn)) {
			return;
		}

		for (const transput of prevTransputs) {
			const current = transputs.find(({ name }) => transput.name === name);

			if (current) {
				continue;
			}

			nodeActions?.destroyTransput(
				{ nodeId, portName: `${transput.name}` },
				transputType,
			);
		}
	}, [
		transputsFn,
		transputs,
		prevTransputs,
		nodeActions,
		nodeId,
		transputType,
	]);

	return transputs;
};
