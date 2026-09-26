import type { ComponentProps } from "react";
import { TerminalPredictionChart } from "#/components/terminal/charts";
import { LiveResonanceTitle } from "#/components/terminal/live-resonance-title";
import { cn } from "#/lib/utils";
import { Canvas } from "./canvas";

export type PredictiveCodingCanvasProps = Omit<
	ComponentProps<typeof Canvas>,
	"title" | "children"
> & {
	title?: string;
};

export const PredictiveCodingCanvas = ({
	className,
	...props
}: PredictiveCodingCanvasProps = {}) => (
	<Canvas
		title={
			<>
				Predictive coding · <LiveResonanceTitle />
			</>
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
		<TerminalPredictionChart />
	</Canvas>
);
