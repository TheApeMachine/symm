import type { ComponentProps } from "react";
import { cn } from "#/lib/utils";
import { Canvas } from "./canvas";
import {
	PredictionChart,
	type PredictionChartProps,
} from "./prediction-chart";

export type PredictiveCodingCanvasProps = Omit<
	ComponentProps<typeof Canvas>,
	"title" | "children"
> &
	PredictionChartProps & {
		title?: string;
		points?: any;
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
	points,
	title,
	...props
}: PredictiveCodingCanvasProps = {}) => {
	const resolvedLatent =
		latent ??
		points?.x ??
		(Array.isArray(points) ? points.map((p: any) => p?.x ?? p?.[0]) : undefined);
	const resolvedForwardCurve =
		forwardCurve ??
		points?.y ??
		(Array.isArray(points) ? points.map((p: any) => p?.y ?? p?.[1]) : undefined);
	const resolvedEnergy =
		energy ??
		(Array.isArray(points?.energy)
			? points.energy[points.energy.length - 1]
			: points?.energy);
	const resolvedSkill =
		skill ??
		(Array.isArray(points?.authority)
			? points.authority[points.authority.length - 1]
			: points?.authority);
	const resolvedPrecision =
		relativePrecision ??
		(Array.isArray(points?.snr)
			? points.snr[points.snr.length - 1]
			: points?.snr);
	const resolvedConfidence =
		confidence ??
		(Array.isArray(points?.activation)
			? points.activation[points.activation.length - 1]
			: points?.activation);

	const h = horizon !== undefined ? String(horizon) : "—";
	const r =
		reach !== undefined
			? String(reach)
			: resolvedForwardCurve?.length
				? String(resolvedForwardCurve.length)
				: "—";
	const prec =
		resolvedPrecision !== undefined
			? typeof resolvedPrecision === "number"
				? resolvedPrecision.toFixed(3)
				: String(resolvedPrecision)
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
				latent={resolvedLatent}
				forwardCurve={resolvedForwardCurve}
				layers={layers}
				skill={resolvedSkill}
				relativePrecision={resolvedPrecision}
				issued={issued}
				realized={realized}
				error={error}
				horizon={horizon}
				reach={reach}
				samples={samples}
				surprise={surprise}
				energy={resolvedEnergy}
				confidence={resolvedConfidence}
				status={status}
				skillStatus={skillStatus}
				forecast={forecast}
			/>
		</Canvas>
	);
};
