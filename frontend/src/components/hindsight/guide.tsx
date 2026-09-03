import { useState } from "react";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";

/*
The orientation guide.

Hindsight is a microscope, and a microscope is useless to someone who has not
been told what the slide is. This panel is that telling: what each band of the
surface is, in what order to read them, and — the part a newcomer most needs —
what a reading here does and does not entitle anyone to conclude.

It teaches procedure, never verdicts. The domain forbids this surface from
saying "missed profit", "should have entered", or that a larger episode means
SYMM did worse (episode-palette.ts), and that boundary is exactly what a
newcomer is least equipped to respect on their own. So the investigation routes
below end at a question the reader takes to the evidence, never at an answer
this panel supplies.
*/

type Route = {
	goal: string;
	question: string;
	steps: string[];
	caution: string;
};

/*
The routes are phrased as investigations, not recipes. Each one ends where the
evidence begins, because the answer lives in the captured state and not in this
panel's opinion of it.
*/
const ROUTES: Route[] = [
	{
		goal: "A move happened and I want to know what SYMM knew during it",
		question: "What did the running system actually hold while this moved?",
		steps: [
			"Pick the instrument on the left, then the episode under the chart. The list is ranked by how far the price travelled — market geometry only, nothing about what SYMM did.",
			"Click the episode's anchor to park the playhead at the start of the move.",
			"Read Signal measurements on the right: those are the numbers the binary actually held at that exact frame.",
			"Walk forward with ] and watch which readings change as the move develops.",
		],
		caution:
			"An episode being large says the market moved. It does not say a trade was available there, nor that missing it was an error.",
	},
	{
		goal: "A reading looks wrong and I want to know where it came from",
		question: "Is this number the system's mistake, or the market's reality?",
		steps: [
			"Park on the frame and open the signal in Signal measurements.",
			"Check the support badge first. An estimator reporting thin support or a value at its own noise level is not evidence of anything yet.",
			"Open the metric row for what it is declared to mean, and what must never be concluded from it.",
			"Use the provenance pane on the left to walk back to the raw captured frame it was computed from.",
		],
		caution:
			"This surface shows what the code produced. If the value is wrong, the value shown is the evidence of that — it is never corrected on the way to the screen.",
	},
	{
		goal: "I want to know why a category or decision was argued for",
		question: "Which measurements did the system itself count as evidence?",
		steps: [
			"Park on the frame where the decision was witnessed.",
			"In Signal measurements, look for metric rows carrying an evidence marker — those were named by a category hypothesis at this boundary.",
			"Open the row to see which reading of the market it was argued for or against.",
			"Compare with a second frame: mark both (m) and the panel reads them side by side.",
		],
		caution:
			"Evidence stance is what the system claimed at that boundary. It is a record of its reasoning, not proof the reasoning was right.",
	},
	{
		goal: "I am comparing two moments to see what changed",
		question: "What was different between these two frames?",
		steps: [
			"Park on the first frame and press m to mark it.",
			"Park on the second and press m again — the comparison opens automatically.",
			"Values are read separately at each exact identity. A blank side means that frame witnessed nothing, never that it carried the other's value.",
		],
		caution:
			"Two frames differing does not establish that the first caused the second. Nothing on this surface establishes causation.",
	},
];

/*
The bands, in reading order. This answers "what am I even looking at?" for the
surface as a whole, which is the question that precedes every route above.
*/
const BANDS: Array<{ name: string; plain: string }> = [
	{
		name: "Run bar",
		plain:
			"Which recorded run you are inside, and whether its capture was complete. A run with an integrity defect cannot be trusted across the gap.",
	},
	{
		name: "Targets",
		plain:
			"Instruments, ranked by how far the price travelled during the run. Ranking is market movement only — no SYMM decision took part in it.",
	},
	{
		name: "The chart",
		plain:
			"The market record as SYMM received it. The bars beneath are arrival activity and quoted spread — what was happening around the price, not what SYMM thought about it.",
	},
	{
		name: "Episodes",
		plain:
			"Moves the selector found in that record, after the fact. They mark where something happened, not where anything should have been done.",
	},
	{
		name: "Signal measurements",
		plain:
			"The numbers the running binary actually held at the exact frame you parked on. This is the evidence — everything else is how you got here.",
	},
];

export const Guide = ({ onClose }: { onClose: () => void }) => {
	const [route, setRoute] = useState<number | null>(null);
	const selected = route === null ? null : ROUTES[route];

	return (
		<Section fit="pane" surface="surface" className="min-h-0 flex-1">
			<Section.Header
				title="How to read this"
				size="m"
				rule
				meta={
					<Button
						variant="bare"
						className="font-mono text-[9px] text-(--f4) hover:text-(--f1)"
						onClick={onClose}
					>
						close (h)
					</Button>
				}
			/>
			<Section.Body className="overflow-auto">
				<div className="px-3 py-2.5">
					<p className="max-w-3xl font-mono text-[10px] text-(--f2) leading-relaxed">
						Hindsight replays a recorded run and shows what the system actually
						held at any exact moment inside it. Nothing here is recalculated:
						if the code produced a wrong number, the wrong number is what you
						see, and that is the point.
					</p>

					<div className="pt-3">
						<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
							the surface, in reading order
						</span>
						<Flex.Column gap={1} className="pt-1.5">
							{BANDS.map((band, index) => (
								<Flex.Row key={band.name} gap={2} className="max-w-3xl">
									<span className="w-4 shrink-0 font-mono text-[9px] text-(--f4) tabular-nums">
										{index + 1}
									</span>
									<span className="w-36 shrink-0 font-mono text-[9px] text-(--f1)">
										{band.name}
									</span>
									<span className="font-mono text-[9px] text-(--f3) leading-relaxed">
										{band.plain}
									</span>
								</Flex.Row>
							))}
						</Flex.Column>
					</div>

					<div className="pt-4">
						<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
							what are you trying to find out?
						</span>
						<Flex.Column gap={1} className="pt-1.5">
							{ROUTES.map((entry, index) => (
								<Button
									key={entry.goal}
									variant="bare"
									className={`max-w-3xl rounded-[3px] border px-2 py-1.5 text-left font-mono text-[9px] leading-relaxed ${
										route === index
											? "border-(--acc) bg-(--raised) text-(--f1)"
											: "border-(--line) text-(--f3) hover:border-(--line2) hover:text-(--f1)"
									}`}
									onClick={() =>
										setRoute((current) => (current === index ? null : index))
									}
								>
									<span className="text-(--f4)">
										{route === index ? "▾" : "▸"}{" "}
									</span>
									{entry.goal}
								</Button>
							))}
						</Flex.Column>
					</div>

					{selected === null ? null : (
						<div className="mt-3 max-w-3xl border-(--line) border-l-2 pl-3">
							<p className="font-mono text-[9px] text-(--acc) leading-relaxed">
								{selected.question}
							</p>
							<Flex.Column gap={1} className="pt-1.5">
								{selected.steps.map((step, index) => (
									<Flex.Row key={step} gap={2}>
										<span className="w-3 shrink-0 font-mono text-[9px] text-(--f4) tabular-nums">
											{index + 1}
										</span>
										<span className="font-mono text-[9px] text-(--f2) leading-relaxed">
											{step}
										</span>
									</Flex.Row>
								))}
							</Flex.Column>
							<p className="pt-2 font-mono text-[9px] text-(--warn) leading-relaxed">
								<span className="text-[8px] uppercase tracking-widest">
									hold on to this
								</span>{" "}
								{selected.caution}
							</p>
						</div>
					)}

					<div className="max-w-3xl pt-4">
						<span className="font-mono text-[8px] text-(--f4) uppercase tracking-widest">
							colour, and what it means
						</span>
						<Flex.Column gap={1} className="pt-1.5 font-mono text-[9px]">
							<span className="text-(--f3)">
								<span className="text-(--up)">●</span> the estimator reports
								full support for this reading
							</span>
							<span className="text-(--f3)">
								<span className="text-(--warn)">●</span> partial support — treat
								the reading as provisional
							</span>
							<span className="text-(--f3)">
								<span className="text-(--down)">●</span> thin support, or a
								value no larger than its own noise
							</span>
							<span className="text-(--f3)">
								<span className="text-(--warn)">undef</span> not estimable here.
								Undefined, which is not the same as zero
							</span>
						</Flex.Column>
						<p className="pt-2 font-mono text-[9px] text-(--f4) leading-relaxed">
							Colour describes how well the system could know a number. It never
							says whether the number is good news, and this surface has no
							opinion on whether a trade was there.
						</p>
					</div>
				</div>
			</Section.Body>
		</Section>
	);
};
