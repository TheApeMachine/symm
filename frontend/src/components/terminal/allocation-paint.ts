import type { AllocationSummary } from "#/components/terminal/allocation-side";
import {
	allocatedSymbols,
	money,
	visibleRowSymbols,
} from "#/components/terminal/allocation-side";

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

/*
paintAllocationSurface writes live ladder numbers into a mounted shell so
websocket cadence never re-renders AllocationMain / SidePanel React trees.
*/
export const paintAllocationSurface = (
	root: HTMLElement | null,
	alloc: AllocationSummary,
): void => {
	if (root === null) {
		return;
	}

	const deployedPercent =
		alloc.deployable > 0
			? Math.min(100, Math.round((alloc.deployed / alloc.deployable) * 100))
			: 0;

	setText(root.querySelector("[data-alloc='median']"), alloc.median.toFixed(3));
	setText(root.querySelector("[data-alloc='mad']"), alloc.mad.toFixed(3));
	setText(
		root.querySelector("[data-alloc='threshold']"),
		alloc.threshold.toFixed(3),
	);
	setText(
		root.querySelector("[data-alloc='deployable']"),
		money(alloc.deployable, alloc.quote),
	);
	setText(
		root.querySelector("[data-alloc='deployed']"),
		money(alloc.deployed, alloc.quote),
	);
	setText(
		root.querySelector("[data-alloc='positions']"),
		String(alloc.positionCount),
	);
	setText(
		root.querySelector("[data-alloc='deployed-pct']"),
		`${deployedPercent}%`,
	);
	setText(
		root.querySelector("[data-alloc='deployed-label']"),
		`deployed ${money(alloc.deployed, alloc.quote)}`,
	);
	setText(
		root.querySelector("[data-alloc='reserved-label']"),
		`reserved ${money(alloc.reserved, alloc.quote)}`,
	);
	setText(
		root.querySelector("[data-alloc='quote-label']"),
		`notional ${alloc.quote} per allocated symbol`,
	);

	const deployFill = root.querySelector("[data-alloc='deploy-fill']");

	if (deployFill instanceof HTMLElement) {
		deployFill.style.width = `${deployedPercent}%`;
	}

	for (const marker of root.querySelectorAll("[data-alloc='median-mark']")) {
		if (marker instanceof HTMLElement) {
			marker.style.left = `${alloc.medianPct}%`;
		}
	}

	for (const marker of root.querySelectorAll("[data-alloc='threshold-mark']")) {
		if (marker instanceof HTMLElement) {
			marker.style.left = `${alloc.thresholdPct}%`;
		}
	}

	const waiting = root.querySelector("[data-alloc='waiting']");

	if (waiting instanceof HTMLElement) {
		waiting.style.display = visibleRowSymbols(alloc).length === 0 ? "" : "none";
	}

	const sizingEmpty = root.querySelector("[data-alloc='sizing-empty']");

	if (sizingEmpty instanceof HTMLElement) {
		sizingEmpty.style.display =
			allocatedSymbols(alloc).length === 0 ? "" : "none";
	}

	for (const row of alloc.rows) {
		const escaped = CSS.escape(row.symbol);
		const main = root.querySelector(`[data-alloc-row='${escaped}']`);

		if (main instanceof HTMLElement) {
			main.style.display = row.allocated || row.inPlay ? "" : "none";
			setText(
				main.querySelector("[data-alloc='edge']"),
				[row.edge >= 0 ? "+" : "-", Math.abs(row.edge).toFixed(3)].join(""),
			);
			setText(
				main.querySelector("[data-alloc='share']"),
				`${(row.share * 100).toFixed(1)}%`,
			);
			setText(
				main.querySelector("[data-alloc='notional']"),
				row.allocated ? money(row.notional, alloc.quote) : "-",
			);

			const name = main.querySelector("[data-alloc='name']");

			if (name instanceof HTMLElement) {
				name.style.color = row.dotColor;
			}

			const edge = main.querySelector("[data-alloc='edge']");

			if (edge instanceof HTMLElement) {
				edge.style.color = row.edge > 0 ? "var(--up)" : "var(--f4)";
			}

			const notional = main.querySelector("[data-alloc='notional']");

			if (notional instanceof HTMLElement) {
				notional.style.color = row.allocated ? "var(--f1)" : "var(--f4)";
			}

			const bar = main.querySelector("[data-alloc='edge-bar']");

			if (bar instanceof HTMLElement) {
				bar.style.left = `${row.edgeLeft}%`;
				bar.style.width = `${row.edgeWidth}%`;
			}

			const dot = main.querySelector("[data-alloc='dot']");

			if (dot instanceof HTMLElement) {
				dot.style.left = `${row.xPct}%`;
				dot.style.background = row.dotColor;
			}
		}

		const sizing = root.querySelector(`[data-alloc-size='${escaped}']`);

		if (sizing instanceof HTMLElement) {
			sizing.style.display = row.allocated ? "" : "none";
			setText(
				sizing.querySelector("[data-alloc='size-notional']"),
				money(row.notional, alloc.quote),
			);
			setText(
				sizing.querySelector("[data-alloc='size-share']"),
				`${(row.share * 100).toFixed(1)}%`,
			);

			const sizeFill = sizing.querySelector("[data-alloc='size-fill']");

			if (sizeFill instanceof HTMLElement) {
				sizeFill.style.width = `${Math.round(row.share * 100)}%`;
			}
		}
	}
};
