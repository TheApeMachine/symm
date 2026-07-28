import {
	cognitiveBeamsFromReading,
	cognitivePosteriorFromReading,
} from "#/components/terminal/cognitive-viz";
import { badgeVariants } from "@/components/ui/badge";
import type { Variant } from "@/components/ui/types";
import { meterTrackVariants } from "@/components/ui/meter";

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

const clampPercent = (percent: number): number =>
	Math.max(0, Math.min(100, percent));

const setMeter = (
	root: HTMLElement,
	trackHook: string,
	fillHook: string,
	variant: Variant,
	size: "s" | "m",
	percent: number,
): void => {
	const track = root.querySelector(`[data-cortex='${trackHook}']`);
	const fill = root.querySelector(`[data-cortex='${fillHook}']`);

	if (track instanceof HTMLElement) {
		track.className = meterTrackVariants({ variant, size });
	}

	if (fill instanceof HTMLElement) {
		fill.style.width = `${clampPercent(percent)}%`;
	}
};

const setBadge = (
	node: Element | null | undefined,
	label: string,
	variant: Variant,
): void => {
	if (!(node instanceof HTMLElement)) {
		return;
	}

	node.textContent = label;
	node.className = badgeVariants({ variant, size: "s" });
};

const setStatValue = (
	node: Element | null | undefined,
	value: string,
): void => {
	if (!(node instanceof HTMLElement)) {
		return;
	}

	setText(node.querySelector("[data-stat-value='true']"), value);
};

/*
paintCortexBeams writes live cognitive beam readings into a mounted
CortexBeamShell without React reconciliation on each websocket tick.
*/
export const paintCortexBeams = (
	root: HTMLElement | null,
	reading: Record<string, unknown> | null,
): void => {
	if (root === null) {
		return;
	}

	const beams = cognitiveBeamsFromReading(reading);
	const waiting = root.querySelector("[data-cortex='waiting']");
	const content = root.querySelector("[data-cortex='content']");

	if (waiting instanceof HTMLElement) {
		waiting.style.display = beams.length === 0 ? "" : "none";
	}

	if (content instanceof HTMLElement) {
		content.style.display = beams.length === 0 ? "none" : "";
	}

	for (let index = 0; index < 8; index++) {
		const row = root.querySelector(`[data-cortex-beam='${index}']`);

		if (!(row instanceof HTMLElement)) {
			continue;
		}

		const beam = beams[index];

		if (beam === undefined) {
			row.style.display = "none";
			continue;
		}

		row.style.display = "";

		const rank = row.querySelector("[data-cortex='rank']");

		if (rank instanceof HTMLElement) {
			rank.textContent = `#${beam.rank}`;
			rank.className =
				beam.variant === "warning"
					? "w-4 shrink-0 font-mono text-[10px] text-(--acc)"
					: "w-4 shrink-0 font-mono text-[10px] text-(--info)";
		}

		setText(
			row.querySelector("[data-cortex='sequence']"),
			beam.sequence || "root",
		);
		setText(row.querySelector("[data-cortex='score']"), beam.score);

		const track = row.querySelector("[data-cortex='meter-track']");
		const fill = row.querySelector("[data-cortex='meter-fill']");

		if (track instanceof HTMLElement) {
			track.className = meterTrackVariants({
				variant: beam.variant,
				size: "xs",
			});
			track.style.width = "70px";
		}

		if (fill instanceof HTMLElement) {
			fill.style.width = `${clampPercent(beam.percent)}%`;
		}
	}
};

/*
paintCortexPanels writes live posterior + REM readings into a mounted
CortexPanelsShell without React reconciliation on each websocket tick.
*/
export const paintCortexPanels = (
	root: HTMLElement | null,
	reading: Record<string, unknown> | null,
): void => {
	if (root === null) {
		return;
	}

	const posterior = cognitivePosteriorFromReading(reading);

	setBadge(
		root.querySelector("[data-cortex='basin-badge']"),
		posterior.classes.length === 0
			? `waiting ${posterior.winnerPercent}`
			: `${posterior.winner} ${posterior.winnerPercent}`,
		"warning",
	);

	const basinWaiting = root.querySelector("[data-cortex='basin-waiting']");

	if (basinWaiting instanceof HTMLElement) {
		basinWaiting.style.display = posterior.classes.length === 0 ? "" : "none";
	}

	for (let index = 0; index < 6; index++) {
		const row = root.querySelector(`[data-cortex-class='${index}']`);

		if (!(row instanceof HTMLElement)) {
			continue;
		}

		const item = posterior.classes[index];

		if (item === undefined) {
			row.style.display = "none";
			continue;
		}

		row.style.display = "";

		const name = row.querySelector("[data-cortex='class-name']");

		if (name instanceof HTMLElement) {
			name.textContent = item.name;
			name.className = item.emphasis
				? "w-16 font-mono text-[10px] text-(--f1)"
				: "w-16 font-mono text-[10px] text-(--f3)";
		}

		setText(
			row.querySelector("[data-cortex='class-percent']"),
			`${item.percent}%`,
		);

		const track = row.querySelector("[data-cortex='meter-track']");
		const fill = row.querySelector("[data-cortex='meter-fill']");

		if (track instanceof HTMLElement) {
			track.className = meterTrackVariants({
				variant: item.variant,
				size: "m",
			});
			track.style.flex = "1 1 0%";
		}

		if (fill instanceof HTMLElement) {
			fill.style.width = `${clampPercent(item.percent)}%`;
		}
	}

	setStatValue(
		root.querySelector("[data-cortex='winner-bits']"),
		posterior.winnerBits,
	);
	setStatValue(
		root.querySelector("[data-cortex='runner-bits']"),
		posterior.runnerBits,
	);
	setStatValue(root.querySelector("[data-cortex='kl']"), posterior.kl);
	setText(
		root.querySelector("[data-cortex='margin-value']"),
		`${posterior.marginPercent}%`,
	);
	setMeter(
		root,
		"margin-track",
		"margin-fill",
		"warning",
		"s",
		posterior.marginPercent,
	);

	setBadge(
		root.querySelector("[data-cortex='entropy-badge']"),
		posterior.ambiguous ? "ambiguous" : "decisive",
		posterior.ambiguous ? "error" : "success",
	);
	setText(
		root.querySelector("[data-cortex='entropy-label']"),
		`${posterior.entropy}b`,
	);
	setText(
		root.querySelector("[data-cortex='entropy-value']"),
		`thr ${posterior.entropyThreshold}`,
	);
	setMeter(
		root,
		"entropy-track",
		"entropy-fill",
		posterior.ambiguous ? "error" : "success",
		"m",
		posterior.entropyPercent,
	);

	const replays = finite(reading?.remReplays);
	const remFrom = typeof reading?.remFrom === "string" ? reading.remFrom : "";
	const remThrough =
		typeof reading?.remThrough === "string" ? reading.remThrough : "";
	const remWindow =
		remFrom === "" || remThrough === ""
			? "—"
			: `${remFrom.slice(11, 19)}–${remThrough.slice(11, 19)}`;

	setBadge(
		root.querySelector("[data-cortex='rem-badge']"),
		replays === null ? "waiting" : "consolidated",
		replays === null ? "warning" : "success",
	);
	setStatValue(root.querySelector("[data-cortex='rem-window']"), remWindow);
	setStatValue(
		root.querySelector("[data-cortex='rem-replays']"),
		replays === null ? "—" : replays.toString(),
	);
	setStatValue(
		root.querySelector("[data-cortex='rem-cohort']"),
		finite(reading?.regimeCohort)?.toString() ?? "—",
	);
};
