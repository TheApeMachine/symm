import type { Holding } from "#/collections/types";
import { fixed } from "#/components/terminal/decision-format";

export type AuditRow = {
	reason: string;
	reference: string;
	meta: string;
};

type AuditParts = {
	card: HTMLElement;
	reason: HTMLElement;
	reference: HTMLElement;
	meta: HTMLElement;
};

export const isClosedLot = (position: Holding): boolean => {
	const status = position.status;

	return typeof status === "string" && status === "closed";
};

/*
holdingAuditRow formats one retained holding for the dashboard audit rail.
*/
export const holdingAuditRow = (
	position: Holding,
	lifecycle?: string,
): AuditRow => {
	const phase =
		lifecycle ??
		(typeof position.status === "string" ? position.status : "closed");
	const pnl = Number(position.pnl);
	const ret = Number(position.return_pct);
	const pnlText = Number.isFinite(pnl) ? fixed(pnl) : "—";
	const retText = Number.isFinite(ret)
		? `${(ret * 100).toFixed(2)}%`
		: "—";

	return {
		reason: phase,
		reference: position.symbol,
		meta: `pnl ${pnlText} · return ${retText}`,
	};
};

export const auditHoldings = (holdings: Holding[]): Holding[] =>
	holdings.filter(isClosedLot);

const auditKey = (holding: Holding): string =>
	[
		holding.symbol,
		holding.entry_price,
		holding.exit_price ?? "",
		holding.pnl,
		holding.return_pct,
	].join("\0");

const bindAuditCard = (): AuditParts => {
	const card = document.createElement("div");
	card.className = "border-(--line) border-b px-3 py-1.5";

	const head = document.createElement("div");
	head.className = "flex items-start justify-between gap-2";

	const reason = document.createElement("span");
	reason.className = "font-medium text-[11px] text-(--f1)";

	const reference = document.createElement("span");
	reference.className = "shrink-0 font-mono text-[9px] text-(--f4)";
	head.append(reason, reference);

	const meta = document.createElement("div");
	meta.className = "mt-px truncate font-mono text-[9px] text-(--f4)";
	card.append(head, meta);

	return { card, reason, reference, meta };
};

/*
DashboardAuditSync maintains closed-lot audit cards in the dashboard rail.
It creates DOM shells once per unique holding identity and updates text in place.
*/
export class DashboardAuditSync {
	private auditCards = new Map<string, AuditParts>();

	/*
	writeAudit reconciles audit cards against closed holdings and lifecycle labels.
	*/
	writeAudit(
		holdings: Holding[],
		lifecycle: Record<string, string>,
		list: HTMLElement | null,
		empty: HTMLElement | null,
	): void {
		const closed = [...auditHoldings(holdings)].reverse();

		if (empty !== null) {
			empty.style.display = closed.length === 0 ? "" : "none";
			empty.textContent =
				holdings.length === 0
					? "waiting for position frames"
					: "no closed lots yet";
		}

		if (list === null) {
			return;
		}

		const nextKeys = new Set<string>();
		const ordered: HTMLElement[] = [];

		for (const holding of closed) {
			const key = auditKey(holding);
			nextKeys.add(key);

			let parts = this.auditCards.get(key);

			if (parts === undefined) {
				parts = bindAuditCard();
				this.auditCards.set(key, parts);
			}

			const row = holdingAuditRow(holding, lifecycle[holding.symbol]);

			if (parts.reason.textContent !== row.reason) {
				parts.reason.textContent = row.reason;
			}

			if (parts.reference.textContent !== row.reference) {
				parts.reference.textContent = row.reference;
			}

			if (parts.meta.textContent !== row.meta) {
				parts.meta.textContent = row.meta;
			}

			ordered.push(parts.card);
		}

		for (const [key, parts] of this.auditCards) {
			if (nextKeys.has(key)) {
				continue;
			}

			parts.card.remove();
			this.auditCards.delete(key);
		}

		const orderMatches =
			ordered.length === list.children.length &&
			ordered.every((card, index) => list.children[index] === card);

		if (!orderMatches) {
			list.replaceChildren(...ordered);
		}
	}
}
