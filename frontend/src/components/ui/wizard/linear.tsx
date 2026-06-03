"use client";

import { useSelector } from "@tanstack/react-store";
import { ArrowLeftIcon, ArrowRightIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/ui/alert";
import { Button } from "#/components/ui/button";
import {
	Card,
	CardFrame,
	CardFrameDescription,
	CardFrameHeader,
	CardFrameTitle,
	CardPanel,
} from "#/components/ui/card";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { useWizard } from "#/components/ui/wizard/context";
import { WizardIndicatorLinear } from "#/components/ui/wizard/indicator";

type WizardLinearProps = {
	title: ReactNode;
	subtitle?: ReactNode;
	submitLabel?: string;
	submitPendingLabel?: string;
	submitIcon?: ReactNode;
	onCancel?: () => void;
};

/*
WizardLinear renders the single-active-step layout used for guided
setup flows. The active step body is wrapped in a CardFrame and
flanked by Back / Continue buttons. The final step's primary button
triggers submit.
*/
export const WizardLinear = ({
	title,
	subtitle,
	submitLabel = "Finish",
	submitPendingLabel = "Finishing…",
	submitIcon,
	onCancel,
}: WizardLinearProps) => {
	const controller = useWizard<unknown>();
	const stepIndex = useSelector(controller.store, (state) => state.stepIndex);
	const draft = useSelector(controller.store, (state) => state.draft);
	const submitting = useSelector(controller.store, (state) => state.submitting);
	const error = useSelector(controller.store, (state) => state.error);

	const step = controller.steps[stepIndex];

	if (!step) {
		throw new Error(`WizardLinear: step index ${stepIndex} out of bounds`);
	}

	const isFirst = stepIndex === 0;
	const isLast = stepIndex === controller.steps.length - 1;

	const body = step.render({
		draft,
		merge: controller.mergeDraft,
		set: controller.setDraft,
	});

	return (
		<Flex.Column gap={6} className="mx-auto w-full max-w-2xl p-8">
			<Flex.Row align="center" justify="between" wrap="wrap" gap={3}>
				<Flex.Column gap={1}>
					<Typography.Span
						variant="muted"
						className="text-xs font-medium uppercase tracking-wider"
					>
						{subtitle ?? "Step"}
					</Typography.Span>
					<Typography.PageTitle>{title}</Typography.PageTitle>
				</Flex.Column>
				<WizardIndicatorLinear />
			</Flex.Row>

			<CardFrame>
				<CardFrameHeader>
					<CardFrameTitle>{step.title}</CardFrameTitle>
					{step.subtitle ? (
						<CardFrameDescription>{step.subtitle}</CardFrameDescription>
					) : null}
				</CardFrameHeader>
				<Card>
					<CardPanel>{body}</CardPanel>
				</Card>
			</CardFrame>

			{error ? (
				<Alert variant="error">
					<AlertTitle>Something went wrong</AlertTitle>
					<AlertDescription>{error}</AlertDescription>
				</Alert>
			) : null}

			<Flex.Row align="center" justify="between">
				<Flex.Row align="center" gap={2}>
					{onCancel ? (
						<Button
							disabled={submitting}
							onClick={onCancel}
							type="button"
							variant="ghost"
						>
							Cancel
						</Button>
					) : null}
					<Button
						disabled={isFirst || submitting}
						onClick={controller.goBack}
						type="button"
						variant="ghost"
					>
						<ArrowLeftIcon />
						Back
					</Button>
				</Flex.Row>
				<Button
					disabled={submitting}
					loading={submitting && isLast}
					onClick={() => {
						void controller.goNext();
					}}
					type="button"
				>
					{isLast ? (submitting ? submitPendingLabel : submitLabel) : "Continue"}
					{isLast ? submitIcon : <ArrowRightIcon />}
				</Button>
			</Flex.Row>
		</Flex.Column>
	);
};
