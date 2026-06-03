"use client";

import { useSelector } from "@tanstack/react-store";
import { CheckIcon, PlayIcon, XIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/ui/alert";
import { Badge } from "#/components/ui/badge";
import { Button } from "#/components/ui/button";
import { Flex } from "#/components/ui/flex";
import { Typography } from "#/components/ui/typography";
import { useWizard } from "#/components/ui/wizard/context";
import { WizardIndicatorSectioned } from "#/components/ui/wizard/indicator";
import type { WizardStepDefinition } from "#/components/ui/wizard/types";
import { cn } from "#/lib/utils";

type WizardSectionedProps = {
	title: ReactNode;
	subtitle?: ReactNode;
	submitLabel?: string;
	submitPendingLabel?: string;
	submitIcon?: ReactNode;
	header?: ReactNode;
	preview?: ReactNode;
	onCancel?: () => void;
};

const sectionAnchorId = (stepId: string): string => `wizard-section-${stepId}`;

type SectionShellProps<TDraft> = {
	step: WizardStepDefinition<TDraft>;
	complete: boolean;
	children: ReactNode;
};

const SectionShell = <TDraft,>({
	step,
	complete,
	children,
}: SectionShellProps<TDraft>) => (
	<Flex.Column
		gap={3}
		id={sectionAnchorId(step.id)}
		className="scroll-mt-4 rounded-2xl border bg-card/40 p-4"
	>
		<Flex.Row align="start" gap={2}>
			<Flex.Center
				className={cn(
					"size-7 shrink-0 rounded-full border transition",
					complete
						? "border-primary bg-primary text-primary-foreground"
						: "border-muted-foreground/30 bg-background text-muted-foreground",
				)}
			>
				{complete ? (
					<CheckIcon className="size-4" />
				) : (
					(step.icon ?? null)
				)}
			</Flex.Center>
			<Flex.Column gap={1}>
				<Typography.H3 className="text-sm font-semibold">
					{step.title}
				</Typography.H3>
				{step.subtitle ? (
					<Typography.Paragraph variant="muted">
						{step.subtitle}
					</Typography.Paragraph>
				) : null}
			</Flex.Column>
		</Flex.Row>
		{children}
	</Flex.Column>
);

/*
WizardSectioned renders every step at once in a scrollable column. A
sticky preview pane on the right is optional. The action bar lives in
the top-right with a completion badge and a single submit button that
unlocks once every step's isComplete returns true.
*/
export const WizardSectioned = ({
	title,
	subtitle,
	submitLabel = "Create",
	submitPendingLabel = "Creating…",
	submitIcon = <PlayIcon />,
	header,
	preview,
	onCancel,
}: WizardSectionedProps) => {
	const controller = useWizard<unknown>();
	const draft = useSelector(controller.store, (state) => state.draft);
	const submitting = useSelector(controller.store, (state) => state.submitting);
	const error = useSelector(controller.store, (state) => state.error);

	const completedCount = controller.steps.filter((step) =>
		step.isComplete(draft),
	).length;
	const canSubmit = completedCount === controller.steps.length;

	return (
		<Flex.Column gap={4} fullHeight className="min-h-0 flex-1">
			<Flex.Row align="center" justify="between" wrap="wrap" gap={3}>
				<Flex.Column gap={1}>
					<Typography.PageTitle>{title}</Typography.PageTitle>
					{subtitle ? (
						<Typography.Paragraph variant="muted">
							{subtitle}
						</Typography.Paragraph>
					) : null}
				</Flex.Column>
				<Flex.Row align="center" gap={2}>
					<Badge variant="outline" size="lg">
						{completedCount}/{controller.steps.length} steps complete
					</Badge>
					{onCancel ? (
						<Button
							type="button"
							variant="outline"
							onClick={onCancel}
							disabled={submitting}
						>
							<XIcon /> Cancel
						</Button>
					) : null}
					<Button
						type="button"
						onClick={() => {
							void controller.submit();
						}}
						disabled={!canSubmit || submitting}
						loading={submitting}
					>
						{submitIcon} {submitting ? submitPendingLabel : submitLabel}
					</Button>
				</Flex.Row>
			</Flex.Row>

			{error ? (
				<Alert variant="error">
					<AlertTitle>Something went wrong</AlertTitle>
					<AlertDescription>{error}</AlertDescription>
				</Alert>
			) : null}

			{header}

			<WizardIndicatorSectioned />

			<div
				className={cn(
					"grid min-h-0 flex-1 grid-cols-1 gap-4",
					preview
						? "lg:grid-cols-[minmax(0,1fr)_minmax(320px,400px)]"
						: "lg:grid-cols-1",
				)}
			>
				<Flex.Column gap={4} className="min-h-0 overflow-y-auto pr-1">
					{controller.steps.map((step) => (
						<SectionShell
							key={step.id}
							step={step}
							complete={step.isComplete(draft)}
						>
							{step.render({
								draft,
								merge: controller.mergeDraft,
								set: controller.setDraft,
							})}
						</SectionShell>
					))}
				</Flex.Column>

				{preview ? (
					<aside className="min-h-[280px] lg:sticky lg:top-4 lg:self-start">
						{preview}
					</aside>
				) : null}
			</div>
		</Flex.Column>
	);
};
