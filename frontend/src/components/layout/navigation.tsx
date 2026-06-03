import { Link } from "@tanstack/react-router";
import {
	BlocksIcon,
	BotIcon,
	ChevronRightIcon,
	CpuIcon,
	FlaskConicalIcon,
	GaugeIcon,
	KanbanIcon,
	LightbulbIcon,
	MicroscopeIcon,
	NetworkIcon,
} from "lucide-react";
import {
	Accordion,
	AccordionItem,
	AccordionPanel,
	AccordionTrigger,
} from "#/components/ui/accordion";
import { Button } from "#/components/ui/button";
import { Flex } from "../ui/flex";

export const Navigation = ({
	onNavigate,
}: {
	onNavigate?: () => void;
} = {}) => {
	return (
		<Accordion className="w-full">
			<AccordionItem value="item-1">
				<AccordionTrigger>
					<BlocksIcon /> Projects
				</AccordionTrigger>
				<AccordionPanel className="flex flex-col gap-2">
					<Link to={"/"} onClick={onNavigate}>
						<Button
							className="w-full h-auto! flex flex-row items-center justify-between gap-4 px-4 py-3 text-left"
							variant="outline"
						>
							<KanbanIcon className="shrink-0" />
							<Flex.Column gap={1} className="text-left" fullWidth>
								<h3>Kanban</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									Kanban board for managing projects
								</p>
							</Flex.Column>
							<ChevronRightIcon
								aria-hidden="true"
								className="in-[[data-slot=button]:hover]:translate-x-0.5 transition-transform"
							/>
						</Button>
					</Link>
					<Link to={"/"} onClick={onNavigate}>
						<Button
							className="w-full h-auto! flex flex-row items-center justify-between gap-4 px-4 py-3 text-left"
							variant="outline"
						>
							<LightbulbIcon className="shrink-0" />
							<Flex.Column gap={1} className="text-left" fullWidth>
								<h3>Request Feature</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									Request a new feature for the project
								</p>
							</Flex.Column>
							<ChevronRightIcon
								aria-hidden="true"
								className="in-[[data-slot=button]:hover]:translate-x-0.5 transition-transform"
							/>
						</Button>
					</Link>
				</AccordionPanel>
			</AccordionItem>
			<AccordionItem value="item-2">
				<AccordionTrigger>
					<MicroscopeIcon /> Research
				</AccordionTrigger>
				<AccordionPanel className="flex flex-col gap-2">
					<Link to={"/"} onClick={onNavigate}>
						<Button
							className="w-full h-auto! flex flex-row items-center justify-between gap-4 px-4 py-3 text-left"
							variant="outline"
						>
							<NetworkIcon className="shrink-0" />
							<Flex.Column gap={1} className="text-left" fullWidth>
								<h3>Architecture</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									Architecture for the project
								</p>
							</Flex.Column>
							<ChevronRightIcon
								aria-hidden="true"
								className="in-[[data-slot=button]:hover]:translate-x-0.5 transition-transform"
							/>
						</Button>
					</Link>
					<Link to={"/"} onClick={onNavigate}>
						<Button
							className="w-full h-auto! flex flex-row items-center justify-between gap-4 px-4 py-3 text-left"
							variant="outline"
						>
							<GaugeIcon className="shrink-0" />
							<Flex.Column gap={1} className="text-left" fullWidth>
								<h3>Benchmarks</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									Benchmarks for the project
								</p>
							</Flex.Column>
							<ChevronRightIcon
								aria-hidden="true"
								className="in-[[data-slot=button]:hover]:translate-x-0.5 transition-transform"
							/>
						</Button>
					</Link>
					<Link to={"/"} onClick={onNavigate}>
						<Button
							className="w-full h-auto! flex flex-row items-center justify-between gap-4 px-4 py-3 text-left"
							variant="outline"
						>
							<FlaskConicalIcon className="shrink-0" />
							<Flex.Column gap={1} className="text-left" fullWidth>
								<h3>New Benchmark</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									Create a new benchmark for the project
								</p>
							</Flex.Column>
							<ChevronRightIcon
								aria-hidden="true"
								className="in-[[data-slot=button]:hover]:translate-x-0.5 transition-transform"
							/>
						</Button>
					</Link>
					<Link to={"/"} onClick={onNavigate}>
						<Button
							className="w-full h-auto! flex flex-row items-center justify-between gap-4 px-4 py-3 text-left"
							variant="outline"
						>
							<MicroscopeIcon className="shrink-0" />
							<Flex.Column gap={1} className="text-left" fullWidth>
								<h3>New Research Project</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									Create a new research project for the project
								</p>
							</Flex.Column>
							<ChevronRightIcon
								aria-hidden="true"
								className="in-[[data-slot=button]:hover]:translate-x-0.5 transition-transform"
							/>
						</Button>
					</Link>
				</AccordionPanel>
			</AccordionItem>
			<AccordionItem value="item-3">
				<AccordionTrigger>
					<NetworkIcon /> Models
				</AccordionTrigger>
				<AccordionPanel className="flex flex-col gap-2">
					<Link to={"/"} onClick={onNavigate}>
						<Button
							className="w-full h-auto! flex flex-row items-center justify-between gap-4 px-4 py-3 text-left"
							variant="outline"
						>
							<CpuIcon className="shrink-0" />
							<Flex.Column gap={1} className="text-left" fullWidth>
								<h3>Models</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									Models for the project
								</p>
							</Flex.Column>
							<ChevronRightIcon
								aria-hidden="true"
								className="in-[[data-slot=button]:hover]:translate-x-0.5 transition-transform"
							/>
						</Button>
					</Link>
				</AccordionPanel>
			</AccordionItem>
			<AccordionItem value="item-4">
				<AccordionTrigger>
					<BotIcon /> Agents
				</AccordionTrigger>
				<AccordionPanel className="flex flex-col gap-2">
					<Link to={"/"} onClick={onNavigate}>
						<Button
							className="w-full h-auto! flex flex-row items-center justify-between gap-4 px-4 py-3 text-left"
							variant="outline"
						>
							<BotIcon className="shrink-0" />
							<Flex.Column gap={1} className="text-left" fullWidth>
								<h3>Agentic</h3>
								<p className="whitespace-break-spaces font-normal text-muted-foreground">
									Agents for the project
								</p>
							</Flex.Column>
							<ChevronRightIcon
								aria-hidden="true"
								className="in-[[data-slot=button]:hover]:translate-x-0.5 transition-transform"
							/>
						</Button>
					</Link>
				</AccordionPanel>
			</AccordionItem>
		</Accordion>
	);
};
