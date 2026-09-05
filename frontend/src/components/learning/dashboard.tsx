import { useState } from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { ImpulseMap } from "./map";
import { useLearning } from "./state";

const amount = (value: number) => value.toLocaleString(undefined, { maximumFractionDigits: 4 });
const clock = (value: string) => value.startsWith("0001-") ? "not valued" : new Date(value).toLocaleTimeString();

export const LearningDashboard = () => {
	const [symbol, setSymbol] = useState("");
	const { view, events, error } = useLearning(symbol);
	return (
		<Flex.Column className="h-full min-h-0 w-full">
			<Section.Header title="Forward learning" meta={view ? `${view.steps.toLocaleString()} observations · ${view.decisions.toLocaleString()} decisions · ${view.resolved.toLocaleString()} outcomes` : "Connecting to the workspace"} />
			{error && <Typography.Mono role="alert" className="p-3 text-error">{error} · Last successful state remains visible.</Typography.Mono>}
			<Flex className="min-h-0 flex-1 max-lg:flex-col">
				<Section className="w-52 shrink-0 border-r border-(--line) max-lg:h-36 max-lg:w-full">
					<Section.Header title="Universe" meta={`${view?.universe?.length ?? 0} keys`} />
					<Section.Body className="p-2">
						{view?.universe?.map((entry) => <Button key={entry.symbol} shape="block" variant={view.symbol === entry.symbol ? "solid" : "quiet"}
							tone="accent" onClick={() => setSymbol(entry.symbol)} aria-pressed={view.symbol === entry.symbol} className="mb-1 justify-between">
							{entry.symbol}<Typography.Mono>{entry.decisions}</Typography.Mono>
						</Button>)}
					</Section.Body>
				</Section>
				<Flex.Column className="min-h-0 min-w-0 flex-1 overflow-auto">
					<Section.Header title={view?.symbol || "Waiting for market observations"} meta={view?.status} />
					<ImpulseMap points={view?.points ?? []} regions={view?.regions ?? []} />
					<Section fit="content">
						<Section.Header title="Independent wallets" meta={view ? `${view.initialCapital} starting cash in each lane` : "Awaiting account economics"} />
						<Section.Body className="overflow-x-auto">
							<table className="w-full text-left font-mono text-xs"><thead className="text-(--f4)"><tr>
								{["Lane", "Action", "Cash", "Inventory", "Fees", "Net P&L", "Fills", "Learned / pending", "Valued at"].map((label) => <th key={label} className="p-3 font-normal">{label}</th>)}
							</tr></thead><tbody>{view?.lanes?.map((lane) => <tr key={lane.lane} className="border-t border-(--line)">
								<td className="p-3 text-(--acc)">{lane.mode} {lane.lane + 1}</td>
								<td className="p-3">{lane.action.kind || "WAIT"}{lane.pending ? " · pending" : ""}</td>
								<td className="p-3">{amount(Number(lane.cash))}</td><td className="p-3">{amount(Number(lane.quantity))}</td>
								<td className="p-3">{amount(Number(lane.fees))}</td>
								<td className={`p-3 ${lane.profit < 0 ? "text-error" : "text-success"}`}>{lane.complete ? amount(lane.profit) : "unvalued"}</td>
								<td className="p-3">{lane.fills}</td><td className="p-3">{lane.resolved} / {lane.unresolved}</td><td className="p-3">{clock(lane.at)}</td>
							</tr>)}</tbody></table>
						</Section.Body>
						<Typography.Mono className="px-3 pb-3 text-(--f4)">P&L includes entry fees and liquidation at displayed bids, including exit fees. Each lane owns its capital. Paper uses completed virtual evidence.</Typography.Mono>
					</Section>
				</Flex.Column>
				<Section className="w-72 shrink-0 border-l border-(--line) max-lg:w-full">
					<Section.Header title="Hot regions" meta="strongest first" />
					<Flex.Column className="gap-2 p-3">
						{view?.regions?.map((region) => <Typography.Mono key={region.id}>#{region.id} · {region.members} cells · {amount(region.strength)} energy · {amount(100 * region.authority)}% authority</Typography.Mono>)}
						{!view?.regions?.length && <Typography.Mono>No evidenced activity yet.</Typography.Mono>}
					</Flex.Column>
					<Section.Header title="Decision journal" meta="persisted" />
					<Section.Body>{events.map((event, index) => <Flex.Column key={`${event.id}-${event.kind}-${index}`} className="gap-1 border-b border-(--line) p-3">
						<Typography.Mono>{clock(event.at)} · {event.mode} {event.lane + 1}</Typography.Mono>
						<Typography.Mono className="text-(--f2)">#{event.id} {event.action} · {event.kind}</Typography.Mono>
						{event.quantity && <Typography.Mono>Quantity {event.quantity}</Typography.Mono>}
						{event.kind === "resolved" && <Typography.Mono>Return target {amount(event.target ?? 0)}</Typography.Mono>}
					</Flex.Column>)}</Section.Body>
				</Section>
			</Flex>
		</Flex.Column>
	);
};
