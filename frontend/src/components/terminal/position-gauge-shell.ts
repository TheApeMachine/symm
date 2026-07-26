import { openThesisShell } from "#/components/terminal/thesis-modal";

export const gaugePanelClassName =
	"mb-[5px] cursor-pointer rounded-[3px] border border-(--line) bg-(--sunken) px-2 py-1.5 font-mono text-[11px] transition-colors hover:border-[color-mix(in_srgb,var(--acc)_35%,transparent)]";

const attachGaugeInteractions = (
	symbol: string,
	panel: HTMLElement,
): void => {
	const openThesis = () => openThesisShell(symbol);

	panel.addEventListener("click", openThesis);
	panel.addEventListener("keydown", (event) => {
		if (event.key === "Enter" || event.key === " ") {
			event.preventDefault();
			openThesis();
		}
	});
};

/*
buildPositionGaugePanel builds one open-lot gauge panel with paint target slots.
*/
export const buildPositionGaugePanel = (symbol: string): HTMLElement => {
	const panel = document.createElement("div");
	panel.dataset.symbol = symbol;
	panel.className = gaugePanelClassName;
	panel.setAttribute("role", "button");
	panel.tabIndex = 0;
	panel.setAttribute("aria-label", `Open thesis for ${symbol}`);
	attachGaugeInteractions(symbol, panel);

	const head = document.createElement("div");
	head.className = "flex items-start justify-between gap-3";

	const symbolEl = document.createElement("span");
	symbolEl.className = "font-semibold text-(--f1)";
	symbolEl.textContent = symbol;

	const pnl = document.createElement("span");
	pnl.dataset.gauge = "pnl";
	pnl.className = "text-right font-semibold";
	head.append(symbolEl, pnl);

	const summaryRow = document.createElement("div");
	summaryRow.className =
		"mt-1 flex items-center justify-between gap-3 text-[10px] text-(--f4)";

	const summary = document.createElement("span");
	summary.dataset.gauge = "summary";

	const returnPct = document.createElement("span");
	returnPct.dataset.gauge = "return";
	summaryRow.append(summary, returnPct);

	const track = document.createElement("div");
	track.dataset.gauge = "track";
	track.className =
		"relative mt-1.5 h-[3px] overflow-visible rounded-full bg-[color-mix(in_srgb,var(--f4)_18%,transparent)]";

	const progress = document.createElement("div");
	progress.dataset.gauge = "progress";
	progress.className =
		"pointer-events-none absolute inset-y-0 rounded-full";

	const stopMarker = document.createElement("div");
	stopMarker.dataset.gauge = "stop";
	stopMarker.className =
		"pointer-events-none absolute top-1/2 h-3 w-[2px] -translate-x-1/2 -translate-y-1/2 rounded-full";
	stopMarker.title = "Stop";
	stopMarker.style.background = "color-mix(in srgb, var(--down) 42%, transparent)";

	const peakMarker = document.createElement("div");
	peakMarker.dataset.gauge = "peak";
	peakMarker.className =
		"pointer-events-none absolute top-1/2 h-3 w-[2px] -translate-x-1/2 -translate-y-1/2 rounded-full";
	peakMarker.title = "Peak";
	peakMarker.style.background =
		"color-mix(in srgb, var(--up) 42%, transparent)";

	const entryMarker = document.createElement("div");
	entryMarker.dataset.gauge = "entry";
	entryMarker.className =
		"pointer-events-none absolute top-1/2 h-1.5 w-px -translate-x-1/2 -translate-y-1/2";
	entryMarker.title = "Entry";
	entryMarker.style.background =
		"color-mix(in srgb, var(--f4) 38%, transparent)";

	const markMarker = document.createElement("div");
	markMarker.dataset.gauge = "mark";
	markMarker.className =
		"pointer-events-none absolute top-1/2 h-[7px] w-[7px] -translate-x-1/2 -translate-y-1/2 rounded-full border border-[color-mix(in_srgb,var(--surface)_70%,transparent)]";
	markMarker.title = "Mark";

	track.append(progress, stopMarker, peakMarker, entryMarker, markMarker);

	const stopMeta = document.createElement("div");
	stopMeta.className =
		"mt-1 flex items-center justify-between gap-2 font-mono text-[9px] text-(--f4)";

	const floor = document.createElement("span");
	floor.dataset.gauge = "floor-label";

	const peak = document.createElement("span");
	peak.dataset.gauge = "peak-label";

	const mark = document.createElement("span");
	mark.dataset.gauge = "mark-label";

	stopMeta.append(floor, peak, mark);

	const momentumWrap = document.createElement("div");
	momentumWrap.dataset.gauge = "momentum-wrap";
	momentumWrap.className = "mt-1.5 flex items-center gap-1.5";

	const momentumLabel = document.createElement("span");
	momentumLabel.className =
		"text-[8px] text-(--f4) uppercase tracking-wide";
	momentumLabel.textContent = "mom";

	const momentumTrack = document.createElement("div");
	momentumTrack.className =
		"h-[3px] flex-1 overflow-hidden rounded-full bg-[color-mix(in_srgb,var(--f4)_25%,transparent)]";

	const momentumBar = document.createElement("div");
	momentumBar.dataset.gauge = "momentum-bar";
	momentumBar.className = "h-full rounded-full transition-[width]";
	momentumTrack.append(momentumBar);
	momentumWrap.append(momentumLabel, momentumTrack);

	const stagnationWrap = document.createElement("div");
	stagnationWrap.dataset.gauge = "stagnation-wrap";
	stagnationWrap.className = "mt-1.5 flex items-center gap-1.5";

	const stagnationLabel = document.createElement("span");
	stagnationLabel.className =
		"text-[8px] text-(--f4) uppercase tracking-wide";
	stagnationLabel.textContent = "stall";

	const stagnationTrack = document.createElement("div");
	stagnationTrack.className =
		"h-[3px] flex-1 overflow-hidden rounded-full bg-[color-mix(in_srgb,var(--f4)_25%,transparent)]";

	const stagnationBar = document.createElement("div");
	stagnationBar.dataset.gauge = "stagnation-bar";
	stagnationBar.className = "h-full rounded-full transition-[width]";
	stagnationTrack.append(stagnationBar);

	const stagnationFlash = document.createElement("span");
	stagnationFlash.dataset.gauge = "stagnation-flash";
	stagnationFlash.className =
		"text-[8px] font-semibold text-(--acc) uppercase tracking-wide";
	stagnationFlash.textContent = "⚡";
	stagnationWrap.append(stagnationLabel, stagnationTrack, stagnationFlash);

	panel.append(head, summaryRow, track, stopMeta, momentumWrap, stagnationWrap);

	return panel;
};
