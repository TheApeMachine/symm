import { DEFAULT_KERNELS } from "#/collections/app";
import { Metric } from "./metric";

/*
One Metric per kernel: each subscribes to its own origin store at focus.
*/
export const KernelMetricStrip = () => (
	<>
		{DEFAULT_KERNELS.map((kernel) => (
			<Metric key={kernel} name={kernel} />
		))}
	</>
);
