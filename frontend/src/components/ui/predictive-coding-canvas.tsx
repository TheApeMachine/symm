import type { ComponentProps } from "react";
import { cn } from "#/lib/utils";
import { Canvas } from "./canvas";
import { PredictionChart, type PredictionChartProps } from "./prediction-chart";

export type PredictiveCodingCanvasProps = Omit<
	ComponentProps<typeof Canvas>,
	"title" | "children"
> &
	PredictionChartProps & {
		title?: string;
	};

export const PredictiveCodingCanvas = ({
	className,
	artifact,
	latent,
	forwardCurve,
	layers,
	skill,
	relativePrecision,
	issued,
	realized,
	error,
	horizon,
	reach,
	samples,
	surprise,
	energy,
	confidence,
	status,
	skillStatus,
	forecast,
	title,
	...props
}: PredictiveCodingCanvasProps = {}) => {
	const h = horizon !== undefined ? String(horizon) : "—";
	const r =
		reach !== undefined
			? String(reach)
			: forwardCurve?.length
				? String(forwardCurve.length)
				: "—";
	const prec =
		relativePrecision !== undefined
			? typeof relativePrecision === "number"
				? relativePrecision.toFixed(3)
				: String(relativePrecision)
			: "—";

	return (
		<Canvas
			title={
				title ?? (
					<>
						Predictive coding ·{" "}
						<span className="text-(--f3)">
							h <span className="text-(--f1)">{h}</span> · r{" "}
							<span className="text-(--f1)">{r}</span> · relative precision{" "}
							<span className="text-(--f1)">{prec}</span>
						</span>
					</>
				)
			}
			meta="settled latent state · adaptive horizon · strict-prior direction head"
			topRight={
				<div className="flex gap-3 text-left">
					<span className="inline-flex items-center gap-1.5">
						<span className="inline-block h-px w-3 bg-(--acc)" />
						forward curve
					</span>
					<span className="inline-flex items-center gap-1.5">
						<span className="inline-block h-px w-3 bg-info" />
						latent state
					</span>
					<span className="inline-flex items-center gap-1.5">
						<span className="inline-block h-px w-3 bg-(--line2)" />
						zero
					</span>
				</div>
			}
			className={cn("flex-1", className)}
			{...props}
		>
			<PredictionChart
				artifact={artifact}
				latent={latent}
				forwardCurve={forwardCurve}
				layers={layers}
				skill={skill}
				relativePrecision={relativePrecision}
				issued={issued}
				realized={realized}
				error={error}
				horizon={horizon}
				reach={reach}
				samples={samples}
				surprise={surprise}
				energy={energy}
				confidence={confidence}
				status={status}
				skillStatus={skillStatus}
				forecast={forecast}
			/>
		</Canvas>
	);
};
