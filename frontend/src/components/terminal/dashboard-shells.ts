import type { Holding } from "#/collections/types";
import {
	createDecisionRowElement,
	removeDecisionRow,
} from "#/components/terminal/decision-row";
import {
	createPositionGaugeElement,
	positionGauges,
	removePositionGauge,
} from "#/components/terminal/position-gauge";

/*
DashboardShellSync creates and orders decision-row and position-gauge shells
for the dashboard rail list hosts without React re-renders.
*/
export class DashboardShellSync {
	private decisionRowRoots = new Map<string, HTMLElement>();
	private positionGaugeRoots = new Map<string, HTMLElement>();
	private lastDecisionSymbols: string[] = [];
	private lastPositionSymbols: string[] = [];
	private lastQuote = "USD";

	/*
	syncDecisionShells ensures one decision row exists per symbol in list order.
	Returns true when the symbol set changed.
	*/
	syncDecisionShells(
		symbols: string[],
		list: HTMLElement | null,
	): boolean {
		if (list === null) {
			return false;
		}

		const changed =
			symbols.length !== this.lastDecisionSymbols.length ||
			symbols.some(
				(symbol, index) => symbol !== this.lastDecisionSymbols[index],
			);

		if (!changed) {
			return false;
		}

		this.lastDecisionSymbols = symbols;
		this.writeDecisionShells(symbols, list);

		return true;
	}

	/*
	syncPositionShells ensures position gauges exist for the visible symbol tail.
	Returns true when the symbol set changed.
	*/
	syncPositionShells(
		symbols: string[],
		quote: string,
		list: HTMLElement | null,
	): boolean {
		if (list === null) {
			return false;
		}

		const changed =
			symbols.length !== this.lastPositionSymbols.length ||
			symbols.some(
				(symbol, index) => symbol !== this.lastPositionSymbols[index],
			);

		if (!changed) {
			return false;
		}

		this.lastPositionSymbols = symbols;
		this.writePositionShells(symbols, quote, list);

		return true;
	}

	/*
	refreshQuote updates cached quote labels when open holdings change denomination.
	*/
	refreshQuote(open: Holding[]): string {
		const quote = open[0]?.symbol.split("/")[1] ?? "USD";

		if (quote === this.lastQuote) {
			return quote;
		}

		this.lastQuote = quote;

		for (const symbol of open.map((holding) => holding.symbol).slice(-8)) {
			const parts = positionGauges.get(symbol);

			if (parts !== undefined) {
				parts.quote = quote;
			}
		}

		return quote;
	}

	private writeDecisionShells(
		symbols: string[],
		list: HTMLElement,
	): void {
		const nextSymbols = new Set(symbols);
		const ordered: HTMLElement[] = [];

		for (const symbol of symbols) {
			let row = this.decisionRowRoots.get(symbol);

			if (row === undefined) {
				row = createDecisionRowElement(symbol);
				this.decisionRowRoots.set(symbol, row);
			}

			ordered.push(row);
		}

		for (const [symbol, row] of this.decisionRowRoots) {
			if (nextSymbols.has(symbol)) {
				continue;
			}

			row.remove();
			this.decisionRowRoots.delete(symbol);
			removeDecisionRow(symbol);
		}

		const orderMatches =
			ordered.length === list.children.length &&
			ordered.every((row, index) => list.children[index] === row);

		if (!orderMatches) {
			list.replaceChildren(...ordered);
		}
	}

	private writePositionShells(
		symbols: string[],
		quote: string,
		list: HTMLElement,
	): void {
		const visible = symbols.slice(-8);
		const nextSymbols = new Set(visible);
		const ordered: HTMLElement[] = [];

		for (const symbol of visible) {
			let gauge = this.positionGaugeRoots.get(symbol);

			if (gauge === undefined) {
				gauge = createPositionGaugeElement(symbol, quote);
				this.positionGaugeRoots.set(symbol, gauge);
			}

			const parts = positionGauges.get(symbol);

			if (parts !== undefined && parts.quote !== quote) {
				parts.quote = quote;
			}

			ordered.push(gauge);
		}

		for (const [symbol, gauge] of this.positionGaugeRoots) {
			if (nextSymbols.has(symbol)) {
				continue;
			}

			gauge.remove();
			this.positionGaugeRoots.delete(symbol);
			removePositionGauge(symbol);
		}

		const orderMatches =
			ordered.length === list.children.length &&
			ordered.every((gauge, index) => list.children[index] === gauge);

		if (!orderMatches) {
			list.replaceChildren(...ordered);
		}
	}
}
