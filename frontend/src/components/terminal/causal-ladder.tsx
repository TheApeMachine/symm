import { useRef } from "react";
import { causalStore } from "#/collections/app";
import { useDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Panel } from "#/components/ui/panel";
import type { Variant } from "#/components/ui/types";
import { Typography } from "#/components/ui/typography";
import { Causal } from "#/providers/telemetry/telemetry/causal";

type Rung = { rung: number; name: string; desc: string; getter: (c: Causal) => number; variant: Variant; path: string };

const RUNGS: Rung[] = [
	{ rung: 1, name: "Association", desc: "ρ(treatment, target)", path: "association", getter: (c) => c.association(), variant: "info" },
	{ rung: 2, name: "Intervention expectation", desc: "E[target | do(treatment)]", path: "doExpectation", getter: (c) => c.doExpectation(), variant: "info" },
	{ rung: 3, name: "Counterfactual target", desc: "structural target under alternate treatment + abducted residual", path: "counterfactual", getter: (c) => c.counterfactual(), variant: "warning" },
	{ rung: 4, name: "Condition context", desc: "resonance energy supplied as causal context", path: "condition", getter: (c) => c.condition(), variant: "success" },
	{ rung: 5, name: "Intervention uplift", desc: "counterfactual target − observed target", path: "uplift", getter: (c) => c.uplift(), variant: "success" },
	{ rung: 6, name: "Abducted residual", desc: "observed target − fitted structural target", path: "noise", getter: (c) => c.noise(), variant: "disabled" },
];

const FOOTER_FIELDS = [
	{ label: "standardized uplift", path: "upliftScore", getter: (c: Causal) => c.upliftScore() },
	{ label: "standardized residual", path: "noiseScore", getter: (c: Causal) => c.noiseScore() },
	{ label: "runner-up share", path: "entry_baseline", getter: (c: Causal) => c.entryBaseline() },
	{ label: "context surprise", path: "contagion", getter: (c: Causal) => c.contagion() },
] as const;

const rungTone = (variant: Variant): string => {
	if (variant === "success") return "text-(--up)";
	if (variant === "warning") return "text-(--acc)";
	if (variant === "disabled") return "text-(--f3)";
	return "text-(--info)";
};

const causalObj = new Causal();

export const CausalLadder = () => {
	const scope = useDecisionsScopeSymbol();
	const root = useRef<HTMLDivElement>(null);

	causalStore.subscribe((frames) => {
		if (!root.current) return;
		const lastFrame = frames.getLast();
		if (!lastFrame) return;

		let targetRow: Causal | null = null;
		for (let i = 0; i < lastFrame.rowsLength(); i++) {
			const row = lastFrame.rows(i, causalObj);
			if (row && row.symbol() === scope) {
				targetRow = row;
				break;
			}
		}

		const set = (q: string, value: string) => {
			const el = root.current?.querySelector<HTMLElement>(`[data-f=${q}]`);
			if (el) el.textContent = value;
		};

		if (!targetRow) return;

		set("category", String(targetRow.category()));

		for (const rung of RUNGS) {
			const value = rung.getter(targetRow);
			set(rung.path, Number.isFinite(value) ? value.toFixed(6) : "—");
		}

		for (const field of FOOTER_FIELDS) {
			const value = field.getter(targetRow);
			set(field.path, Number.isFinite(value) ? value.toFixed(4) : "—");
		}
	});

	if (scope === undefined) {
		return (
			<Panel>
				<Typography.Label size="lg" tone="f1" className="block">Causal ladder</Typography.Label>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">select a candidate to scope its causal reading</div>
			</Panel>
		);
	}

	return (
		<Panel ref={root}>
			<Typography.Label size="lg" tone="f1" className="block">Causal ladder</Typography.Label>
			<div className="mt-0.5 mb-3 font-mono text-[9.5px] text-(--f4)">
				Pearl causal estimates · evidence class <span data-f="category" />
			</div>
			<div className="flex flex-col gap-2.5">
				{RUNGS.map((rung) => (
					<div key={rung.name} className="rounded-sm border border-(--line) bg-(--surface) px-2.5 py-2">
						<div className="flex items-center justify-between">
							<Typography.Label size="xs" tone="f1">{rung.rung}. {rung.name}</Typography.Label>
							<span data-f={rung.path} className={`font-mono text-[11px] ${rungTone(rung.variant)}`}>—</span>
						</div>
						<div className="my-1.5 font-mono text-[9px] text-(--f4)">{rung.desc}</div>
					</div>
				))}
			</div>
			<div className="mt-3 grid grid-cols-2 gap-1.5 border-(--line) border-t pt-2 font-mono text-[10px]">
				{FOOTER_FIELDS.map((field) => (
					<div key={field.path} className="flex justify-between">
						<Typography.Label size="xxs" tone="f4" weight="normal">{field.label}</Typography.Label>
						<span data-f={field.path} className="text-(--f1)">—</span>
					</div>
				))}
			</div>
		</Panel>
	);
};

