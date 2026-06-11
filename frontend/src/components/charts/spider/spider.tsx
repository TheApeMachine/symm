import { memo } from "react";
import { SciChartReact } from "scichart-react";
import { appStore } from "#/collections/app";
import { drawSignalSpider } from "#/components/charts/spider/init";

export const SpiderChart = memo(function SpiderChart({
	sources,
	labels,
}: {
	sources: string[];
	labels: Record<string, string>;
}) {
	return (
		<SciChartReact
			key={sources.join(",")}
			initChart={(rootElement) =>
				drawSignalSpider(
					rootElement,
					sources.map((source) => labels[source] ?? source),
					sources,
				)
			}
			onInit={(result) => {
				appStore.actions.updateRegimeUpdater(result.addData);

				return () => appStore.actions.updateRegimeUpdater(null);
			}}
			className="h-full w-full"
			style={{ width: "100%", height: "100%" }}
		/>
	);
});
