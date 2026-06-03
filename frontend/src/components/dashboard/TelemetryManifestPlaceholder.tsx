export const TelemetryManifestPlaceholder = ({
	label = "Waiting for telemetry manifest…",
}: {
	label?: string;
}) => (
	<div className="flex h-full min-h-0 flex-1 items-center justify-center rounded border border-dashed border-(--dash-border) bg-(--dash-panel) px-3 text-center text-[11px] text-(--dash-muted)">
		{label}
	</div>
);
