import type { RefObject } from "react";
import {
	cognitiveBeamsFromReading,
	cognitivePosteriorFromReading,
} from "#/components/terminal/cognitive-viz";
import { Badge, badgeVariants } from "@/components/ui/badge";
import { meterTrackVariants } from "@/components/ui/meter";
import { Panel } from "@/components/ui/panel";
import { Stat } from "@/components/ui/stat";
import type { Variant } from "@/components/ui/types";

const finite = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

const clampPercent = (percent: number): number =>
	Math.max(0, Math.min(100, percent));

const BEAM_POOL = 8;
const CLASS_POOL = 6;

/*
CortexBeamShell renders a static shell for the cognitive beam list. Live beam
rows are painted into a fixed pool of pre-rendered rows by paintCortexBeams so
websocket cadence never re-renders this React tree.
*/
export const CortexBeamShell = ({
	rootRef,
}: {
	rootRef: RefObject<HTMLDivElement | null>;
}) => (
	<div ref={rootRef} className="flex min-h-0 flex-1 flex-col">
		<div
			data-cortex="waiting"
			className="px-3 py-6 text-center font-mono text-[11px] text-(--f4)"
		>
			waiting for cognitive beam reading
		</div>

		<div
			data-cortex="content"
			style={{ display: "none" }}
			className="flex min-h-0 flex-1 flex-col gap-[5px] overflow-auto px-2 py-1.5"
		>
			{Array.from({ length: BEAM_POOL }, (_, index) => (
				<Panel
					// biome-ignore lint/suspicious/noArrayIndexKey: fixed pool
					key={index}
					size="s"
					className="flex items-center gap-2"
					data-cortex-beam={index}
					style={{ display: "none" }}
				>
					<span
						data-cortex="rank"
						className="w-4 shrink-0 font-mono text-[10px] text-(--info)"
					/>
					<span
						data-cortex="sequence"
						className="flex-1 font-mono text-[11px] text-(--f1)"
					/>
					<div
						data-cortex="meter-track"
						className={meterTrackVariants({ variant: "info", size: "xs" })}
						style={{ width: "70px" }}
					>
						<div
							data-cortex="meter-fill"
							className="h-full bg-(--meter-tone)"
							style={{ width: "0%" }}
						/>
					</div>
					<span
						data-cortex="score"
						className="w-11 shrink-0 text-right font-mono text-[9.5px] text-(--f3)"
					/>
				</Panel>
			))}
		</div>
	</div>
);

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

	for (let index = 0; index < BEAM_POOL; index++) {
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
CortexPanelsShell renders the static chrome for the four cortex side panels.
Live values are painted into data-cortex hooks by paintCortexPanels.
*/
export const CortexPanelsShell = ({
	rootRef,
}: {
	rootRef: RefObject<HTMLDivElement | null>;
}) => (
	<div ref={rootRef} className="flex flex-col gap-3.5">
		<Panel>
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					Attractor basin · classify
				</span>
				<Badge label="waiting 0%" variant="warning" data-cortex="basin-badge" />
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				softmax posterior · b/[class]/[sequence]
			</div>
			<div className="flex flex-col gap-2">
				<div
					data-cortex="basin-waiting"
					className="font-mono text-[10px] text-(--f4)"
				>
					waiting for attractor basin
				</div>
				{Array.from({ length: CLASS_POOL }, (_, index) => (
					<div
						// biome-ignore lint/suspicious/noArrayIndexKey: fixed pool
						key={index}
						data-cortex-class={index}
						className="flex items-center gap-2"
						style={{ display: "none" }}
					>
						<span
							data-cortex="class-name"
							className="w-16 font-mono text-[10px] text-(--f3)"
						/>
						<div
							data-cortex="meter-track"
							className={meterTrackVariants({ variant: "info", size: "m" })}
							style={{ flex: "1 1 0%" }}
						>
							<div
								data-cortex="meter-fill"
								className="h-full bg-(--meter-tone)"
								style={{ width: "0%" }}
							/>
						</div>
						<span
							data-cortex="class-percent"
							className="w-8 text-right font-mono text-[10px] text-(--f2)"
						/>
					</div>
				))}
			</div>
		</Panel>

		<Panel>
			<div className="font-semibold text-[12px] text-(--f1)">
				Contrastive evidence
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				routing margin · winner vs runner-up
			</div>
			<div className="grid grid-cols-3 gap-2.5 text-center">
				<Stat
					layout="feature"
					label="winner bits"
					value=""
					variant="success"
					data-cortex="winner-bits"
				/>
				<Stat
					layout="feature"
					label="runner-up bits"
					value=""
					data-cortex="runner-bits"
				/>
				<Stat
					layout="feature"
					label="KL divergence"
					value=""
					variant="warning"
					data-cortex="kl"
				/>
			</div>
			<div className="mt-3">
				<div className="mb-1 flex justify-between font-mono text-[9.5px] text-(--f4)">
					<span>separation margin</span>
					<span data-cortex="margin-value" />
				</div>
				<div
					data-cortex="margin-track"
					className={meterTrackVariants({ variant: "warning", size: "s" })}
				>
					<div
						data-cortex="margin-fill"
						className="h-full bg-(--meter-tone)"
						style={{ width: "0%" }}
					/>
				</div>
			</div>
		</Panel>

		<Panel>
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					Branch entropy gate
				</span>
				<Badge label="decisive" variant="success" data-cortex="entropy-badge" />
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				shannon H vs uniform threshold
			</div>
			<div>
				<div className="mb-1 flex justify-between font-mono text-[10px]">
					<span data-cortex="entropy-label" className="text-(--f3)" />
					<span data-cortex="entropy-value" className="text-(--f1)" />
				</div>
				<div
					data-cortex="entropy-track"
					className={meterTrackVariants({ variant: "success", size: "m" })}
				>
					<div
						data-cortex="entropy-fill"
						className="h-full bg-(--meter-tone)"
						style={{ width: "0%" }}
					/>
				</div>
			</div>
		</Panel>

		<Panel>
			<div className="flex items-center justify-between">
				<span className="font-semibold text-[12px] text-(--f1)">
					REM consolidation
				</span>
				<Badge label="waiting" variant="warning" data-cortex="rem-badge" />
			</div>
			<div className="mt-1 mb-3 font-mono text-[9.5px] text-(--f4)">
				episodic replay · decay · retroactive inhibition
			</div>
			<div className="grid grid-cols-3 gap-2 font-mono">
				<Stat layout="tile" label="window" value="" data-cortex="rem-window" />
				<Stat layout="tile" label="replays" value="" data-cortex="rem-replays" />
				<Stat layout="tile" label="cohort" value="" data-cortex="rem-cohort" />
			</div>
		</Panel>
	</div>
);

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

/*
setStatValue writes into a Stat tile, whose value lives in the div marked
data-stat-value (see the Stat primitive).
*/
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

	for (let index = 0; index < CLASS_POOL; index++) {
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

	// Contrastive evidence
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

	// Branch entropy gate
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

	// REM consolidation
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
