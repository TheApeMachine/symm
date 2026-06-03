import type { ReactNode } from "react";

/*
WizardMode selects the layout strategy. Linear renders one step at a
time with next/back navigation. Sectioned renders every step at once
as a scrollable column with a completion checklist at the top.
*/
export type WizardMode = "linear" | "sectioned";

/*
WizardStepRenderProps is the contract step renderers receive from the
Wizard. Steps may mutate draft through merge (shallow patch) or set
(full replace via updater).
*/
export type WizardStepRenderProps<TDraft> = {
	draft: TDraft;
	merge: (patch: Partial<TDraft>) => void;
	set: (updater: (draft: TDraft) => TDraft) => void;
};

/*
WizardStepDefinition is the per-step config. isComplete drives the
indicator and the submit gate. render is the step body.
*/
export type WizardStepDefinition<TDraft> = {
	id: string;
	title: string;
	subtitle?: string;
	icon?: ReactNode;
	isComplete: (draft: TDraft) => boolean;
	render: (props: WizardStepRenderProps<TDraft>) => ReactNode;
};

/*
WizardState is the reactive payload held by the controller's store.
*/
export type WizardState<TDraft> = {
	draft: TDraft;
	stepIndex: number;
	submitting: boolean;
	error: string | null;
};
