export type CausalPeer = {
	symbol: string;
	correlation: number;
};

export type CausalGraphRow = {
	event?: "causal_graph";
	symbol: string;
	ready: boolean;
	sample_count: number;
	macro_momentum: number;
	local_flow: number;
	liquidity: number;
	price_velocity: number;
	association: number;
	intervention: number;
	uplift: number;
	confounding_gap: number;
	confidence: number;
	reason: string;
	coef_macro: number;
	coef_liquidity: number;
	coef_flow: number;
	peers?: CausalPeer[];
};

type GraphSink = (row: CausalGraphRow) => void;

export const isCausalGraphRow = (raw: unknown): raw is CausalGraphRow => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	return (
		row.event === "causal_graph" &&
		typeof row.symbol === "string" &&
		typeof row.macro_momentum === "number"
	);
};

/*
CausalGraphDataProvider routes hub causal DAG snapshots to the force graph.
*/
class CausalGraphDataProviderImpl {
	private sinks = new Map<string, GraphSink>();
	private latest = new Map<string, CausalGraphRow>();

	registerSymbol(symbol: string, sink: GraphSink) {
		this.sinks.set(symbol, sink);

		const row = this.latest.get(symbol);

		if (row !== undefined) {
			sink(row);
		}

		return () => {
			this.sinks.delete(symbol);
		};
	}

	snapshot(symbol: string): CausalGraphRow | undefined {
		return this.latest.get(symbol);
	}

	ingest(raw: unknown) {
		if (!isCausalGraphRow(raw)) {
			return;
		}

		this.latest.set(raw.symbol, raw);
		this.sinks.get(raw.symbol)?.(raw);
	}
}

const shared = new CausalGraphDataProviderImpl();

export const CausalGraphDataProvider = {
	registerSymbol: (symbol: string, sink: GraphSink) =>
		shared.registerSymbol(symbol, sink),
	snapshot: (symbol: string) => shared.snapshot(symbol),
	ingest: (raw: unknown) => shared.ingest(raw),
};
