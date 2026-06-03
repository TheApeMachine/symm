"use client";

import { useSelector } from "@tanstack/react-store";
import { CheckIcon } from "lucide-react";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { useWizard } from "#/components/ui/wizard/context";
import { cn } from "#/lib/utils";

/*
WizardIndicatorLinear renders the team-setup-style step indicator: a
horizontal row of numbered circles separated by short connector lines,
with a check mark on completed steps and primary color on the active
one. Read-only — linear navigation goes through Back/Continue buttons.
*/
export const WizardIndicatorLinear = () => {
	const controller = useWizard<unknown>();
	const stepIndex = useSelector(controller.store, (state) => state.stepIndex);

	return (
		<Flex.Row align="center" gap={2}>
			{controller.steps.map((step, index) => {
				const isActive = index === stepIndex;
				const isComplete = index < stepIndex;
				const isLast = index === controller.steps.length - 1;

				return (
					<Flex.Row align="center" gap={2} key={step.id}>
						<Flex.Center
							className={cn(
								"size-7 shrink-0 rounded-full border text-xs font-semibold transition-colors",
								isActive && "border-primary bg-primary text-primary-foreground",
								isComplete && "border-primary/40 bg-primary/10 text-primary",
								!isActive &&
									!isComplete &&
									"border-border text-muted-foreground",
							)}
						>
							{isComplete ? (
								<CheckIcon aria-hidden className="size-3.5" />
							) : (
								index + 1
							)}
						</Flex.Center>
						{isLast ? null : (
							<div
								className={cn(
									"h-px w-8",
									isComplete ? "bg-primary/40" : "bg-border",
								)}
							/>
						)}
					</Flex.Row>
				);
			})}
		</Flex.Row>
	);
};

/*
WizardIndicatorSectioned renders the research/benchmarks-style pill
chips. Each chip is clickable and scrolls / jumps to its section.
Completion is derived live from the controller so the indicator
updates as the user fills the form.
*/
export const WizardIndicatorSectioned = () => {
	const controller = useWizard<unknown>();
	const draft = useSelector(controller.store, (state) => state.draft);

	return (
		<Flex.Row align="center" wrap="wrap" gap={1} className="list-none">
			{controller.steps.map((step, index) => {
				const complete = step.isComplete(draft);
				const isLast = index === controller.steps.length - 1;

				return (
					<Flex.Row align="center" gap={1} key={step.id}>
						<button
							type="button"
							onClick={() => controller.jumpTo(step.id)}
							className={cn(
								"flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs transition",
								complete
									? "border-primary/40 bg-primary/5 text-foreground"
									: "border-transparent text-muted-foreground hover:bg-muted/40",
							)}
						>
							<Flex.Center
								className={cn(
									"size-5 rounded-full font-medium text-[10px]",
									complete
										? "bg-primary text-primary-foreground"
										: "bg-muted text-muted-foreground",
								)}
							>
								{complete ? (
									<CheckIcon aria-hidden className="size-3" />
								) : (
									index + 1
								)}
							</Flex.Center>
							<Typography.Span className="text-xs">
								{step.title}
							</Typography.Span>
						</button>
						{isLast ? null : (
							<span
								aria-hidden
								className="h-px w-3 bg-muted-foreground/30 sm:w-4"
							/>
						)}
					</Flex.Row>
				);
			})}
		</Flex.Row>
	);
};
