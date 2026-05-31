import { createLazyFileRoute } from "@tanstack/react-router";

import { DashboardHeader } from "#/components/header";
import { DashboardLayout } from "#/components/symm/DashboardLayout";
import {
	useSymmConnected,
	useSymmEnginePulse,
} from "#/lib/symm/use-dashboard-data";

const TradingDashboard = () => {
	const connected = useSymmConnected();
	const pulse = useSymmEnginePulse();

	return (
		<div className="dashboard-shell">
			<DashboardHeader />
			<main className="dashboard-main">
				<DashboardLayout />
			</main>
			<footer className="dashboard-footer">
				<span>{connected ? "live" : "offline"}</span>
				<span className="mx-2">·</span>
				<span>measurements {pulse?.measurements ?? 0}</span>
				<span className="mx-2">·</span>
				<span>open {pulse?.open ?? 0}</span>
			</footer>
		</div>
	);
};

export const Route = createLazyFileRoute("/")({
	component: TradingDashboard,
});
