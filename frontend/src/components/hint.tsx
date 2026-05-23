interface Props {
	connected: boolean;
	message?: string;
}

export const EmptyHint = ({ connected, message }: Props) => {
	return (
		<p className="px-3 pb-3 text-xs text-(--dash-muted)">
			{message ??
				(connected
					? "Waiting for engine events…"
					: "Connect telemetry WebSocket")}
		</p>
	);
};
