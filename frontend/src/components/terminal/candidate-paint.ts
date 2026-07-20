import {
	verdictBadgeClassName,
	verdictToVariant,
} from "#/components/terminal/badge-tone";
import type { CandidateModel } from "#/components/terminal/decision-candidate";
import { fixed } from "#/components/terminal/decision-format";
import { cn } from "#/lib/utils";
import { badgeVariants } from "@/components/ui/badge";
import { meterTrackVariants } from "@/components/ui/meter";

const ratio = (value: number): number => Math.min(1, Math.max(0, value));

const setText = (node: Element | null | undefined, value: string): void => {
	if (node instanceof HTMLElement) {
		node.textContent = value;
	}
};

const meterVariant = (
	value: number,
): "disabled" | "warning" | "info" => {
	if (value === 0) {
		return "disabled";
	}

	if (ratio(value) > 0.6) {
		return "warning";
	}

	return "info";
};

const scoreVariant = (
	verdict: string,
): "success" | "error" | "info" => {
	if (verdict === "allow") {
		return "success";
	}

	if (verdict === "blocked") {
		return "error";
	}

	return "info";
};

const paintMeter = (
	root: Element | null | undefined,
	value: number,
	variant: "disabled" | "warning" | "info" | "success" | "error",
): void => {
	if (!(root instanceof HTMLElement)) {
		return;
	}

	setText(root.querySelector("[data-meter='value']"), fixed(value));

	const track = root.querySelector("[data-meter='track']");
	const fill = root.querySelector("[data-meter='fill']");

	if (track instanceof HTMLElement) {
		const size = track.dataset.meterSize === "m" ? "m" : "s";

		track.className = cn(
			meterTrackVariants({ variant, size }),
			track.dataset.meterLayout === "inline" ? "flex-1" : "",
		);
	}

	if (fill instanceof HTMLElement) {
		fill.style.width = `${Math.round(ratio(value) * 100)}%`;
	}
};

/*
writeCandidateRow paints one CandidateModel into a mounted CandidateRow shell.
*/
export const writeCandidateRow = (
	root: HTMLElement | null,
	model: CandidateModel,
): void => {
	if (root === null) {
		return;
	}

	setText(
		root.querySelector("[data-candidate='support']"),
		`×${model.support} src`,
	);

	const waiting = root.querySelector("[data-candidate='bars-waiting']");

	if (waiting instanceof HTMLElement) {
		waiting.style.display = model.bars.length === 0 ? "" : "none";
	}

	for (const src of ["causal", "predict", "manifold"] as const) {
		const bar = root.querySelector(`[data-candidate-bar='${src}']`);

		if (!(bar instanceof HTMLElement)) {
			continue;
		}

		const live = model.bars.find((row) => row.src === src);

		bar.style.display = live === undefined ? "none" : "";

		if (live === undefined) {
			continue;
		}

		paintMeter(bar, live.value, meterVariant(live.value));
	}

	const scoreMeter = root.querySelector("[data-candidate='score']");

	paintMeter(scoreMeter, model.score, scoreVariant(model.verdict));
	setText(
		scoreMeter?.querySelector("[data-meter='label']"),
		model.hasDecision ? "utility" : "combined",
	);

	const edge = root.querySelector("[data-candidate='edge']");

	if (edge instanceof HTMLElement) {
		edge.textContent = `pearl Δ ${fixed(model.edge)}`;
		edge.style.color = model.edge >= 0 ? "var(--up)" : "var(--down)";
	}

	const badge = root.querySelector("[data-candidate='verdict']");

	if (badge instanceof HTMLElement) {
		badge.textContent = model.verdict;
		badge.className = cn(
			badgeVariants({ variant: verdictToVariant(model.verdict) }),
			verdictBadgeClassName(model.verdict),
		);
	}

	setText(root.querySelector("[data-candidate='why']"), model.why);
	setText(
		root.querySelector("[data-candidate='branch']"),
		`branch · ${model.branch}`,
	);

	for (const row of model.waterfall) {
		const node = root.querySelector(`[data-waterfall='${row.src}']`);

		if (!(node instanceof HTMLElement)) {
			continue;
		}

		const width = Math.min(46, Math.abs(row.delta) * 100);
		const positive = row.delta >= 0;
		const bar = node.querySelector("[data-waterfall='bar']");
		const label = node.querySelector("[data-waterfall='delta']");

		if (bar instanceof HTMLElement) {
			bar.style.left = `${positive ? 50 : 50 - width}%`;
			bar.style.width = `${width}%`;
			bar.style.background = positive ? "var(--up)" : "var(--down)";
		}

		if (label instanceof HTMLElement) {
			label.textContent = `${positive ? "+" : "−"}${Math.abs(row.delta).toFixed(3)}`;
			label.style.color = positive ? "var(--up)" : "var(--down)";
		}
	}

	for (const probe of model.probes) {
		setText(
			root.querySelector(`[data-probe='${probe.label}']`),
			fixed(probe.value),
		);
	}
};
