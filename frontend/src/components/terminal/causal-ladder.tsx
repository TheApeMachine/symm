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
		desc: "P(y | x)",
		path: "association",
		variant: "info",
	},
	{
		rung: 2,
		name: "Intervention",
		desc: "P(y | do(x))",
		path: "doExpectation",
		variant: "info",
	},
	{
		rung: 3,
		name: "Counterfactual",
		desc: "P(y' | x', x)",
		path: "counterfactual",
		variant: "warning",
	},
	{
		rung: 4,
		name: "Condition",
		desc: "P(y | x, parents)",
		path: "condition",
		variant: "success",
	},
	{
		rung: 5,
		name: "Intervention uplift",
		desc: "P(y | do(x)) - P(y)",
		path: "uplift",
		variant: "success",
	},
	{
		rung: 6,
		name: "Noise floor",
		desc: "P(y | ¬x)",
		path: "noise",
		variant: "disabled",
	},
];

const FOOTER_FIELDS = [
	{ label: "uplift", path: "upliftScore" },
	{ label: "residual", path: "residual" },
	{ label: "baseline", path: "entry_baseline" },
	{ label: "panic", path: "contagion" },
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

const rungFill = (variant: Variant): string => {
	if (variant === "success") {
		return "bg-(--success)";
	}

	if (variant === "warning") {
		return "bg-(--warning)";
	}

	if (variant === "disabled") {
		return "bg-(--line2)";
	}

	return "bg-(--info)";
};

/*
CausalLadder reads the Pearl rungs for the selected candidate.

The bar width is set from the reading itself rather than a precomputed
percentage: the value lands in a custom property and CSS clamps it, so a rung
that reports more than unit strength saturates instead of running off the
panel. Every number shown is a wire field — the browser scales the picture, not
the evidence.
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
						Pearl estimates · evidence class{" "}
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
										data-paint-format=".4f"
										className={`font-mono text-[11px] ${rungTone(rung.variant)}`}
									/>
								</div>
								<div className="my-1.5 font-mono text-[9px] text-(--f4)">
									{rung.desc}
								</div>
								<div className="h-1.25 overflow-hidden rounded-[3px] bg-(--line)">
									<div
										data-set={rung.path}
										data-target="style.--rung"
										className={`h-full transition-[width] duration-500 ease-out ${rungFill(rung.variant)}`}
										style={{
											width: "clamp(0%, calc(var(--rung, 0) * 100%), 100%)",
										}}
									/>
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
