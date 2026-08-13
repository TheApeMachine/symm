import { useDecisionsScopeSymbol } from "#/components/terminal/decision-side";
import { Component } from "#/components/ui/component";
import { Typography } from "@/components/ui/typography";
import { Panel } from "@/components/ui/panel";
import type { Variant } from "@/components/ui/types";

type Rung = {
	rung: number;
	name: string;
	desc: string;
	/*
		The wire key on the causal row. Rows arrive flat — one object per symbol
		with every reading at the top level — so a rung is a path, not a lookup
		through a reading wrapper the engine does not send.
	*/
	path: string;
	variant: Variant;
};

const RUNGS: Rung[] = [
	{
		rung: 1,
		name: "Association",
		desc: "ρ(treatment, target)",
		path: "association",
		variant: "info",
	},
	{
		rung: 2,
		name: "Intervention expectation",
		desc: "E[target | do(treatment)]",
		path: "doExpectation",
		variant: "info",
	},
	{
		rung: 3,
		name: "Counterfactual target",
		desc: "structural target under alternate treatment + abducted residual",
		path: "counterfactual",
		variant: "warning",
	},
	{
		rung: 4,
		name: "Condition context",
		desc: "resonance energy supplied as causal context",
		path: "condition",
		variant: "success",
	},
	{
		rung: 5,
		name: "Intervention uplift",
		desc: "counterfactual target − observed target",
		path: "uplift",
		variant: "success",
	},
	{
		rung: 6,
		name: "Abducted residual",
		desc: "observed target − fitted structural target",
		path: "noise",
		variant: "disabled",
	},
];

const FOOTER_FIELDS = [
	{ label: "standardized uplift", path: "upliftScore" },
	{ label: "standardized residual", path: "noiseScore" },
	{ label: "runner-up share", path: "entry_baseline" },
	{ label: "context surprise", path: "contagion" },
] as const;

const rungTone = (variant: Variant): string => {
	if (variant === "success") {
		return "text-(--up)";
	}

	if (variant === "warning") {
		return "text-(--acc)";
	}

	if (variant === "disabled") {
		return "text-(--f3)";
	}

	return "text-(--info)";
};

/*
CausalLadder reads the dimensionally distinct Pearl estimates for the selected
candidate. Values are shown numerically without a shared progress scale because
correlation, target expectations, context, uplift, and residual are not
interchangeable probabilities.
*/
export const CausalLadder = () => {
	const scope = useDecisionsScopeSymbol();

	if (scope === undefined) {
		return (
			<Panel>
				<Typography.Label size="lg" tone="f1" className="block">
					Causal ladder
				</Typography.Label>
				<div className="mt-2 font-mono text-[9.5px] text-(--f4)">
					select a candidate to scope its causal reading
				</div>
			</Panel>
		);
	}

	return (
		<Component registerKey="causal">
			{({ ref }) => (
				<Panel ref={ref} data-scope="symbol" data-filter={scope}>
					<Typography.Label size="lg" tone="f1" className="block">
						Causal ladder
					</Typography.Label>
					<div className="mt-0.5 mb-3 font-mono text-[9.5px] text-(--f4)">
						Pearl causal estimates · evidence class{" "}
						<span data-paint="category" data-paint-empty="—" />
					</div>

					<div className="flex flex-col gap-2.5">
						{RUNGS.map((rung) => (
							<div
								key={rung.name}
								className="rounded-sm border border-(--line) bg-(--surface) px-2.5 py-2"
							>
								<div className="flex items-center justify-between">
									<Typography.Label size="xs" tone="f1">
										{rung.rung}. {rung.name}
									</Typography.Label>
									<span
										data-paint={rung.path}
										data-paint-format=".6f"
										className={`font-mono text-[11px] ${rungTone(rung.variant)}`}
									/>
								</div>
								<div className="my-1.5 font-mono text-[9px] text-(--f4)">
									{rung.desc}
								</div>
							</div>
						))}
					</div>

					<div className="mt-3 grid grid-cols-2 gap-1.5 border-(--line) border-t pt-2 font-mono text-[10px]">
						{FOOTER_FIELDS.map((field) => (
							<div key={field.path} className="flex justify-between">
								<Typography.Label size="xxs" tone="f4" weight="normal">
									{field.label}
								</Typography.Label>
								<span
									data-paint={field.path}
									data-paint-format=".4f"
									className="text-(--f1)"
								/>
							</div>
						))}
					</div>
				</Panel>
			)}
		</Component>
	);
};
