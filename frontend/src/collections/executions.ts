import { createStore } from "@tanstack/react-store";

export type Execution = Record<string, unknown> & {
	exec_id?: string;
	order_id?: string;
	symbol?: string;
	side?: string;
	order_status?: string;
	timestamp?: string;
};

type ExecutionFrame = Record<string, unknown>;

const asFrame = (value: unknown, path: string): ExecutionFrame => {
	if (typeof value === "object" && value !== null && !Array.isArray(value)) {
		return value as ExecutionFrame;
	}

	throw new TypeError(`${path} must be an object`);
};

const requiredFinite = (value: unknown, path: string): number => {
	const number =
		typeof value === "number"
			? value
			: typeof value === "string" && value.trim().length > 0
				? Number(value)
				: Number.NaN;

	if (Number.isFinite(number)) {
		return number;
	}

	throw new TypeError(`${path} must be finite`);
};

const numericFields = [
	"last_qty",
	"last_price",
	"cost",
	"cum_qty",
	"cum_cost",
	"avg_price",
	"fee_usd_equiv",
] as const;

const normalizeRow = (
	value: unknown,
	path: string,
	context: ExecutionFrame = {},
): Execution => {
	const row = asFrame(value, path);
	const execution = { ...context, ...row };

	for (const field of numericFields) {
		if (execution[field] !== undefined) {
			execution[field] = requiredFinite(execution[field], `${path}.${field}`);
		}
	}

	if (execution.exec_type === "trade") {
		execution.last_qty = requiredFinite(execution.last_qty, `${path}.last_qty`);
		execution.last_price = requiredFinite(
			execution.last_price,
			`${path}.last_price`,
		);
	}

	return execution;
};

const normalizeEntry = (value: unknown, path: string): Execution[] => {
	if (Array.isArray(value)) {
		return value.flatMap((entry, index) =>
			normalizeEntry(entry, `${path}[${index}]`),
		);
	}

	const frame = asFrame(value, path);

	if (!Array.isArray(frame.data)) {
		return [normalizeRow(frame, path)];
	}

	const { data, ...context } = frame;

	return data.map((row, index) =>
		normalizeRow(row, `${path}.data[${index}]`, context),
	);
};

export const normalizeExecutions = (executions: unknown): Execution[] =>
	normalizeEntry(executions, "executions");

export const executionsStore = createStore(
	{
		executions: [] as Execution[],
		observed: false,
	},
	({ setState }) => ({
		updateFrame: (executions: unknown) =>
			setState(() => ({
				executions: normalizeExecutions(executions),
				observed: true,
			})),
	}),
);
