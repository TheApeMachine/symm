import { createStore } from "@tanstack/react-store";

export type Execution = Record<string, unknown> & {
	exec_id?: string;
	order_id?: string;
	symbol?: string;
	side?: string;
	order_status?: string;
	timestamp?: string;
};

export const executionsStore = createStore(
	{
		executions: [] as Execution[][],
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (executions: Execution[][]) =>
			setState(() => ({
				executions,
				observed: true,
			})),
	}),
);
