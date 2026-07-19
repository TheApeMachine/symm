import { useSelector } from "@tanstack/react-store";
import { memo, useRef } from "react";
import { appStore } from "#/collections/app";
import type {
	CausalFrame,
	ManifoldFrame,
	ResonanceFrame,
} from "#/collections/types";
import type { StrategyDecision } from "#/types/thesis";
import {
	verdictBadgeClassName,
	verdictToVariant,
} from "#/components/terminal/badge-tone";
import {
	type CandidateModel,
	buildCandidate,
} from "#/components/terminal/decision-candidate";
import { fixed } from "#/components/terminal/decision-format";
import { useDirectStorePaint } from "#/hooks/use-direct-store-paint";
import { cn } from "#/lib/utils";
import { getWorker } from "#/providers/websocket";
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

const paintCandidateRow = (
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

const InlineMeterShell = ({ src }: { src: string }) => (
	<div
		data-candidate-bar={src}
		className="flex min-w-0 items-center gap-2 font-mono text-[9px]"
		style={{ display: "none" }}
	>
		<span className="w-16 text-(--f4)">{src}</span>
		<div
			data-meter="track"
			data-meter-layout="inline"
			className={cn(meterTrackVariants({ variant: "info", size: "s" }), "flex-1")}
		>
			<div
				data-meter="fill"
				className="h-full bg-(--meter-tone)"
				style={{ width: "0%" }}
			/>
		</div>
		<span data-meter="value" className="w-[30px] text-(--f3)" />
	</div>
);

/*
CandidateRow paints one symbol's ladder attribution from store buffers without
re-rendering the decisions list on every websocket tick.
*/
export const CandidateRow = memo(
	({
		symbol,
		selected,
		onSelect,
	}: {
		symbol: string;
		selected: boolean;
		onSelect: (symbol: string) => void;
	}) => {
		const rootRef = useRef<HTMLButtonElement>(null);
		const online = useSelector(appStore, (state) => state.online);

		useDirectStorePaint(
			getWorker(),
			[
				{ store: "decisions", key: symbol },
				{ store: "causal", key: symbol },
				{ store: "manifold", key: symbol },
				{ store: "resonance", key: symbol },
			],
			(buffers) =>
				paintCandidateRow(
					rootRef.current,
					buildCandidate(
						symbol,
						(buffers[`decisions:${symbol}`] ?? []).at(-1) as
							| StrategyDecision
							| undefined,
						(buffers[`causal:${symbol}`] ?? []).at(-1) as
							| CausalFrame
							| undefined,
						(buffers[`resonance:${symbol}`] ?? []).at(-1) as
							| ResonanceFrame
							| undefined,
						(buffers[`manifold:${symbol}`] ?? []).at(-1) as
							| ManifoldFrame
							| undefined,
					),
				),
			[online, symbol],
		);

		return (
			<button
				ref={rootRef}
				type="button"
				data-candidate={symbol}
				data-symbol={symbol}
				onClick={() => onSelect(symbol)}
				className={cn(
					"cursor-pointer overflow-hidden rounded border bg-(--surface) text-left font-[inherit]",
					selected
						? "border-[color-mix(in_srgb,var(--up)_30%,transparent)]"
						: "border-(--line)",
				)}
			>
				<div className="grid grid-cols-[78px_1fr_132px_92px] items-center gap-3 px-3 py-2.5">
					<div>
						<div className="font-mono font-semibold text-[13px] text-(--f1)">
							{symbol}
						</div>
						<div
							data-candidate="support"
							className="font-mono text-[9px] text-(--f4)"
						/>
					</div>
					<div className="flex min-w-0 flex-col gap-1 font-mono text-[9px]">
						<div data-candidate="bars-waiting" className="text-(--f4)">
							waiting for ladder frames
						</div>
						<InlineMeterShell src="causal" />
						<InlineMeterShell src="predict" />
						<InlineMeterShell src="manifold" />
					</div>
					<div>
						<div data-candidate="score">
							<div className="mb-1 flex justify-between font-mono text-[9.5px] text-(--f4)">
								<span data-meter="label">combined</span>
								<span data-meter="value" className="text-(--f1)" />
							</div>
							<div
								data-meter="track"
								data-meter-size="m"
								className={cn(
									meterTrackVariants({ variant: "info", size: "m" }),
								)}
							>
								<div
									data-meter="fill"
									className="h-full bg-(--meter-tone)"
									style={{ width: "0%" }}
								/>
							</div>
						</div>
						<div data-candidate="edge" className="mt-1 font-mono text-[9px]" />
					</div>
					<div className="text-right">
						<span data-candidate="verdict" />
						<div
							data-candidate="why"
							className="mt-1 font-mono text-[9px] text-(--f4)"
						/>
					</div>
				</div>
				<div
					className="grid grid-cols-2 gap-5 border-(--line) border-t bg-(--sunken) px-3.5 py-3 font-mono text-[9.5px]"
					style={{ display: selected ? "" : "none" }}
				>
					<div>
						<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-widest">
							Score attribution
						</div>
						<div className="flex flex-col gap-1.5">
							{(["causal", "predict", "field"] as const).map((src) => (
								<div
									key={src}
									data-waterfall={src}
									className="flex items-center gap-2"
								>
									<span className="w-[60px] text-(--f4)">{src}</span>
									<div className="relative h-3 flex-1 rounded-sm bg-(--line)">
										<div className="absolute top-0 bottom-0 left-1/2 w-px bg-(--f4)" />
										<div
											data-waterfall="bar"
											className="absolute top-px bottom-px rounded-[1px]"
										/>
									</div>
									<span
										data-waterfall="delta"
										className="w-[50px] text-right"
									/>
								</div>
							))}
						</div>
						<div
							data-candidate="branch"
							className="mt-2 text-[9px] text-(--f4)"
						/>
					</div>
					<div>
						<div className="mb-2 font-semibold text-[10px] text-(--f3) uppercase tracking-widest">
							Counterfactual probes · do(·)
						</div>
						<div className="flex flex-col gap-1.5">
							{["beta", "panic", "residual", "intervention"].map((label) => (
								<div
									key={label}
									className="flex items-center justify-between gap-2 rounded-sm border border-(--line) bg-(--surface) px-2 py-1.5"
								>
									<span className="text-(--f2)">{label}</span>
									<span data-probe={label} className="text-(--f1)" />
								</div>
							))}
						</div>
					</div>
				</div>
			</button>
		);
	},
);
