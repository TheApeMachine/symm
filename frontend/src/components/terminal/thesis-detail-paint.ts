import { fixed } from "#/components/terminal/decision-format";
import { paintLifecycleTrack } from "#/components/terminal/lifecycle-track";
import type { ThesisSnapshot } from "#/components/terminal/thesis-snapshot";
import { badgeVariants } from "@/components/ui/badge";

/*
evidenceLabels turns measurement evidence keys into compact source/metric labels.
*/
const evidenceLabels = (keys: string[] | undefined): string[] => {
	if (keys === undefined) {
		return [];
	}

	const labels: string[] = [];
	const seen = new Set<string>();

	for (const key of keys) {
		const parts = key.split("/");
		const source = parts[0] ?? key;
		const metric = parts[2] ?? source;
		const label = metric === source ? source : `${source}/${metric}`;

		if (!seen.has(label)) {
			seen.add(label);
			labels.push(label);
		}
	}

	return labels.slice(0, 4);
};

const setText = (node: Element | null | undefined, value: string) => {
	if (node instanceof HTMLElement && node.textContent !== value) {
		node.textContent = value;
	}
};

const setHidden = (node: Element | null | undefined, hidden: boolean) => {
	if (node instanceof HTMLElement) {
		node.style.display = hidden ? "none" : "";
	}
};

const syncRows = (
	host: HTMLElement | null,
	keys: string[],
	create: (key: string) => HTMLElement,
	write: (row: HTMLElement, key: string, index: number) => void,
) => {
	if (host === null) {
		return;
	}

	const existing = new Map<string, HTMLElement>();

	for (const child of host.children) {
		if (!(child instanceof HTMLElement)) {
			continue;
		}

		const key = child.dataset.thesisRow;

		if (key !== undefined) {
			existing.set(key, child);
		}
	}

	const ordered: HTMLElement[] = [];
	const next = new Set(keys);

	for (const [index, key] of keys.entries()) {
		let row = existing.get(key);

		if (row === undefined) {
			row = create(key);
			row.dataset.thesisRow = key;
			existing.set(key, row);
		}

		write(row, key, index);
		ordered.push(row);
	}

	for (const [key, row] of existing) {
		if (!next.has(key)) {
			row.remove();
		}
	}

	const orderMatches =
		ordered.length === host.children.length &&
		ordered.every((row, index) => host.children[index] === row);

	if (!orderMatches) {
		host.replaceChildren(...ordered);
	}
};

/*
writeThesisDetailRail paints one snapshot into a mounted rail shell. Rows are
created once and updated in place.
*/
export const writeThesisDetailRail = (
	root: HTMLElement,
	snapshot: ThesisSnapshot,
) => {
	const lifecycle = snapshot.lifecycle ?? "observing";
	const track = root.querySelector<HTMLElement>("[data-lifecycle-track]");

	setText(root.querySelector("[data-thesis='lifecycle-meta']"), lifecycle);
	setText(track?.querySelector("[data-lifecycle='symbol']"), snapshot.symbol);

	if (track !== null) {
		track.dataset.lifecycleTrack = snapshot.symbol;
		paintLifecycleTrack(track, lifecycle);
	}

	const decision = snapshot.decision;
	setText(
		root.querySelector("[data-thesis='decision-meta']"),
		decision?.action ?? "none",
	);
	setHidden(
		root.querySelector("[data-thesis='decision-empty']"),
		decision !== null,
	);
	setHidden(
		root.querySelector("[data-thesis='decision-body']"),
		decision === null,
	);

	if (decision !== null) {
		setText(
			root.querySelector("[data-thesis='decision-utility']"),
			fixed(decision.utility),
		);
		setText(
			root.querySelector("[data-thesis='decision-proposed']"),
			fixed(decision.proposedNotional),
		);
		setText(
			root.querySelector("[data-thesis='decision-return']"),
			fixed(decision.expectedReturn),
		);
		setText(
			root.querySelector("[data-thesis='decision-confidence']"),
			fixed(decision.confidence),
		);
		setText(
			root.querySelector("[data-thesis='decision-cause']"),
			decision.cause,
		);
	}

	const forecasts = snapshot.forecasts.slice(-4);
	setText(
		root.querySelector("[data-thesis='forecasts-meta']"),
		`${snapshot.forecasts.length} rows`,
	);
	setHidden(
		root.querySelector("[data-thesis='forecasts-empty']"),
		forecasts.length > 0,
	);
	syncRows(
		root.querySelector("[data-thesis='forecasts-list']"),
		forecasts.map(
			(forecast) => `${forecast.source}:${forecast.at}:${forecast.target}`,
		),
		() => {
			const row = document.createElement("div");
			row.className =
				"border-(--line) border-t py-1.5 font-mono text-[10px] first:border-t-0 first:pt-0";
			row.innerHTML =
				'<div data-thesis="forecast-title" class="text-(--f1)"></div><div data-thesis="forecast-body" class="text-(--f3)"></div>';
			return row;
		},
		(row, _key, index) => {
			const forecast = forecasts[index];

			if (forecast === undefined) {
				return;
			}

			setText(
				row.querySelector("[data-thesis='forecast-title']"),
				`${forecast.source} · ${forecast.target}`,
			);
			setText(
				row.querySelector("[data-thesis='forecast-body']"),
				`return ${forecast.expectedReturn.toFixed(4)} · conf ${forecast.confidence.toFixed(3)}`,
			);
		},
	);

	const hypotheses = snapshot.hypotheses.slice(-3);
	setText(
		root.querySelector("[data-thesis='hypotheses-meta']"),
		`${snapshot.hypotheses.length} rows`,
	);
	setHidden(
		root.querySelector("[data-thesis='hypotheses-empty']"),
		hypotheses.length > 0,
	);
	syncRows(
		root.querySelector("[data-thesis='hypotheses-list']"),
		hypotheses.map(
			(hypothesis) =>
				`${hypothesis.source}:${hypothesis.at}:${hypothesis.claim}`,
		),
		() => {
			const row = document.createElement("div");
			row.className =
				"border-(--line) border-t py-1.5 font-mono text-[10px] first:border-t-0 first:pt-0";
			row.innerHTML =
				'<div data-thesis="hypothesis-claim" class="text-(--f1)"></div><div data-thesis="hypothesis-body" class="text-(--f3)"></div>';
			return row;
		},
		(row, _key, index) => {
			const hypothesis = hypotheses[index];

			if (hypothesis === undefined) {
				return;
			}

			setText(
				row.querySelector("[data-thesis='hypothesis-claim']"),
				hypothesis.claim,
			);
			setText(
				row.querySelector("[data-thesis='hypothesis-body']"),
				`do ${hypothesis.doExpectation.toFixed(4)} · strength ${hypothesis.strength.toFixed(3)}`,
			);
		},
	);

	const categories = snapshot.categories.slice(-6);
	setText(
		root.querySelector("[data-thesis='categories-meta']"),
		`${snapshot.categories.length} rows`,
	);
	setHidden(
		root.querySelector("[data-thesis='categories-empty']"),
		categories.length > 0,
	);
	syncRows(
		root.querySelector("[data-thesis='categories-list']"),
		categories.map((category) => `${category.type}:${category.confidence}`),
		() => {
			const row = document.createElement("div");
			row.className = "font-mono text-[10px]";
			row.innerHTML =
				'<div class="flex items-center justify-between gap-2"><span data-thesis="category-type" class="text-(--f1)"></span><span data-thesis="category-conf" class="text-(--acc)"></span></div><div data-thesis="category-support" class="text-(--f3)"></div><div data-thesis="category-oppose" class="text-(--f4)"></div>';
			return row;
		},
		(row, _key, index) => {
			const category = categories[index];

			if (category === undefined) {
				return;
			}

			setText(
				row.querySelector("[data-thesis='category-type']"),
				category.type,
			);
			setText(
				row.querySelector("[data-thesis='category-conf']"),
				category.confidence.toFixed(3),
			);

			const support = evidenceLabels(category.supporting);
			const oppose = evidenceLabels(category.opposing);
			const supportNode = row.querySelector(
				"[data-thesis='category-support']",
			);
			const opposeNode = row.querySelector(
				"[data-thesis='category-oppose']",
			);

			setHidden(supportNode, support.length === 0);
			setHidden(opposeNode, oppose.length === 0);
			setText(
				supportNode,
				support.length === 0 ? "" : `+ ${support.join(" · ")}`,
			);
			setText(
				opposeNode,
				oppose.length === 0 ? "" : `− ${oppose.join(" · ")}`,
			);
		},
	);

	setText(
		root.querySelector("[data-thesis='holdings-meta']"),
		`${snapshot.holdings.length} lots`,
	);
	setHidden(
		root.querySelector("[data-thesis='holdings-empty']"),
		snapshot.holdings.length > 0,
	);
	syncRows(
		root.querySelector("[data-thesis='holdings-list']"),
		snapshot.holdings.map(
			(holding) =>
				`${holding.symbol}:${String(holding.status)}:${holding.qty}`,
		),
		() => {
			const row = document.createElement("div");
			row.className =
				"border-(--line) border-t py-1.5 font-mono text-[10px] first:border-t-0 first:pt-0";
			row.innerHTML =
				'<div class="flex items-center justify-between gap-2"><span data-thesis="holding-status" class="text-(--f1)"></span><span data-thesis="holding-qty" class="text-(--f4)"></span></div><div data-thesis="holding-body" class="text-(--f3)"></div>';
			return row;
		},
		(row, _key, index) => {
			const holding = snapshot.holdings[index];

			if (holding === undefined) {
				return;
			}

			setText(
				row.querySelector("[data-thesis='holding-status']"),
				typeof holding.status === "string" ? holding.status : "unknown",
			);
			setText(
				row.querySelector("[data-thesis='holding-qty']"),
				`qty ${fixed(holding.qty)}`,
			);
			setText(
				row.querySelector("[data-thesis='holding-body']"),
				`bid ${fixed(holding.mark)} · pnl ${fixed(holding.pnl)} · return ${fixed(holding.return_pct)}`,
			);
		},
	);

	const findings = snapshot.findings.slice(-3);
	setText(
		root.querySelector("[data-thesis='findings-meta']"),
		`${snapshot.findings.length} rows`,
	);
	setHidden(
		root.querySelector("[data-thesis='findings-empty']"),
		findings.length > 0,
	);
	syncRows(
		root.querySelector("[data-thesis='findings-list']"),
		findings.map(
			(finding) =>
				`${finding.component}:${finding.condition}:${finding.estimatedEffect}`,
		),
		() => {
			const row = document.createElement("div");
			row.className =
				"border-(--line) border-t py-1.5 first:border-t-0 first:pt-0";
			const badge = document.createElement("span");
			badge.dataset.thesis = "finding-component";
			const condition = document.createElement("div");
			condition.dataset.thesis = "finding-condition";
			condition.className = "mt-1 font-mono text-[10px] text-(--f1)";
			row.append(badge, condition);
			return row;
		},
		(row, _key, index) => {
			const finding = findings[index];

			if (finding === undefined) {
				return;
			}

			const badge = row.querySelector("[data-thesis='finding-component']");

			if (badge instanceof HTMLElement) {
				badge.textContent = finding.component;
				badge.className = badgeVariants({
					variant: "warning",
					size: "xs",
				});
			}

			setText(
				row.querySelector("[data-thesis='finding-condition']"),
				finding.condition,
			);
		},
	);
};
