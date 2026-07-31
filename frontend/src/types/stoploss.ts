import type { Status } from "#/types/status";

export type Stoploss = {
	status: Status;
	symbol: string;
	entry: string | null;
	peak: string | null;
	mark: string | null;
	floor: string | null;
};
