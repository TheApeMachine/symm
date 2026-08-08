import { Link } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { type TerminalSurface, terminalStore } from "#/collections/terminal";
import { Clock } from "@/components/clock";
import { Engine } from "@/components/engine";
import { Button } from "@/components/ui/button";
import { Icon, type IconName } from "@/components/ui/icon";
import { Nav } from "@/components/ui/nav";

type TerminalRoutePath =
	| "/"
	| "/graph"
	| "/fluid"
	| "/signals"
	| "/decisions"
	| "/journal"
	| "/xray"
	| "/cortex"
	| "/allocation";

const SURFACE_ITEMS: Array<{
	key: TerminalSurface;
	label: string;
	icon: IconName;
	to: TerminalRoutePath;
}> = [
	{ key: "dashboard", label: "Dashboard", icon: "dashboard", to: "/" },
	{ key: "graph", label: "Market graph", icon: "graph", to: "/graph" },
	{ key: "fluid", label: "Fluid manifold", icon: "scan", to: "/fluid" },
	{ key: "signals", label: "Signal insight", icon: "signal", to: "/signals" },
	{ key: "decisions", label: "Decision tree", icon: "tree", to: "/decisions" },
	{ key: "journal", label: "Trade journal", icon: "journal", to: "/journal" },
	{ key: "xray", label: "Latent x-ray", icon: "scan", to: "/xray" },
	{ key: "cortex", label: "Cognitive tree", icon: "cortex", to: "/cortex" },
	{ key: "allocation", label: "Allocation", icon: "bars", to: "/allocation" },
];

export const TerminalNav = ({ active }: { active: TerminalSurface }) => {
	const scanlines = useSelector(terminalStore, (state) => state.scanlines);
	const { toggleScanlines } = terminalStore.actions;

	return (
		<Nav>
			<Nav.Group label="Surfaces">
				{SURFACE_ITEMS.map((item) => (
					<Nav.Item
						key={item.key}
						as={Link}
						to={item.to}
						active={active === item.key}
						icon={<Icon name={item.icon} size="m" />}
						label={item.label}
					/>
				))}
			</Nav.Group>
			<Nav.Group label="Engine">
				<Engine />
			</Nav.Group>
			<Nav.Footer>
				<Clock />
				<Button
					variant="bare"
					onClick={toggleScanlines}
					className="mt-1.5 block text-(--f3) hover:text-(--acc)"
				>
					scanlines {scanlines ? "on" : "off"}
				</Button>
			</Nav.Footer>
		</Nav>
	);
};
