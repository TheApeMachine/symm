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
	| "/learning"
	| "/influence"
	| "/lineage"
	| "/fluid"
	| "/signals"
	| "/journal"
	| "/xray"
	| "/cortex"
	| "/allocation"
	| "/diagnostics"
	| "/hindsight";

export const SURFACE_ITEMS: Array<{
	key: TerminalSurface;
	label: string;
	icon: IconName;
	to: TerminalRoutePath;
}> = [
	{ key: "dashboard", label: "Dashboard", icon: "dashboard", to: "/" },
	{ key: "learning", label: "Forward learning", icon: "grid", to: "/learning" },
	{
		key: "diagnostics",
		label: "System diagnostics",
		icon: "lanes",
		to: "/diagnostics",
	},
	{
		key: "influence",
		label: "Influence field",
		icon: "spark",
		to: "/influence",
	},
	{
		key: "lineage",
		label: "Metric lineage",
		icon: "target",
		to: "/lineage",
	},
	{ key: "fluid", label: "Fluid manifold", icon: "scan", to: "/fluid" },
	{ key: "signals", label: "Signal insight", icon: "signal", to: "/signals" },
	{ key: "journal", label: "Trade journal", icon: "journal", to: "/journal" },
	{ key: "xray", label: "Latent x-ray", icon: "scan", to: "/xray" },
	{ key: "cortex", label: "Cognitive tree", icon: "cortex", to: "/cortex" },
	{ key: "allocation", label: "Allocation", icon: "bars", to: "/allocation" },
	{ key: "hindsight", label: "Hindsight", icon: "lanes", to: "/hindsight" },
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
