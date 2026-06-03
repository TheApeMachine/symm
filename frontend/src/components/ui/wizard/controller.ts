import { Store } from "@tanstack/store";
import type {
	WizardState,
	WizardStepDefinition,
} from "#/components/ui/wizard/types";

/*
WizardControllerArgs configures a fresh controller. persistStep runs
between steps in linear mode (e.g. autosave a draft on Continue) and
is ignored in sectioned mode. onSubmit fires on Finish / Create and
must throw to surface an error to the user.
*/
export type WizardControllerArgs<TDraft> = {
	initialDraft: TDraft;
	steps: ReadonlyArray<WizardStepDefinition<TDraft>>;
	onSubmit: (draft: TDraft) => Promise<void> | void;
	persistStep?: (draft: TDraft, stepIndex: number) => Promise<void> | void;
};

/*
WizardController owns the wizard's reactive state and exposes typed
methods that step renderers and the wizard chrome call. State lives
in a Tanstack Store so subscribers re-render through useStore selectors
without component-local useState.
*/
export class WizardController<TDraft> {
	readonly store: Store<WizardState<TDraft>>;
	readonly steps: ReadonlyArray<WizardStepDefinition<TDraft>>;
	private readonly onSubmit: (draft: TDraft) => Promise<void> | void;
	private readonly persistStep?: (
		draft: TDraft,
		stepIndex: number,
	) => Promise<void> | void;

	constructor(args: WizardControllerArgs<TDraft>) {
		if (args.steps.length === 0) {
			throw new Error("WizardController requires at least one step");
		}

		this.steps = args.steps;
		this.onSubmit = args.onSubmit;
		this.persistStep = args.persistStep;
		this.store = new Store<WizardState<TDraft>>({
			draft: args.initialDraft,
			stepIndex: 0,
			submitting: false,
			error: null,
		});
	}

	mergeDraft = (patch: Partial<TDraft>): void => {
		this.store.setState((previous) => ({
			...previous,
			draft: { ...previous.draft, ...patch },
		}));
	};

	setDraft = (updater: (draft: TDraft) => TDraft): void => {
		this.store.setState((previous) => ({
			...previous,
			draft: updater(previous.draft),
		}));
	};

	clearError = (): void => {
		this.store.setState((previous) => {
			if (previous.error === null) {
				return previous;
			}

			return { ...previous, error: null };
		});
	};

	jumpTo = (stepId: string): void => {
		const index = this.steps.findIndex((step) => step.id === stepId);

		if (index === -1) {
			throw new Error(`Unknown wizard step "${stepId}"`);
		}

		this.store.setState((previous) => ({
			...previous,
			stepIndex: index,
			error: null,
		}));
	};

	goBack = (): void => {
		this.store.setState((previous) => ({
			...previous,
			stepIndex: Math.max(0, previous.stepIndex - 1),
			error: null,
		}));
	};

	goNext = async (): Promise<void> => {
		const snapshot = this.store.state;

		if (snapshot.submitting) {
			return;
		}

		const isLast = snapshot.stepIndex >= this.steps.length - 1;

		if (isLast) {
			await this.submit();
			return;
		}

		this.store.setState((previous) => ({
			...previous,
			submitting: true,
			error: null,
		}));

		const success = await this.runAsync(async () => {
			if (this.persistStep) {
				await this.persistStep(snapshot.draft, snapshot.stepIndex);
			}
		});

		if (!success) {
			return;
		}

		this.store.setState((previous) => ({
			...previous,
			stepIndex: previous.stepIndex + 1,
			submitting: false,
		}));
	};

	submit = async (): Promise<void> => {
		const snapshot = this.store.state;

		if (!this.canSubmit()) {
			return;
		}

		this.store.setState((previous) => ({
			...previous,
			submitting: true,
			error: null,
		}));

		const success = await this.runAsync(() => this.onSubmit(snapshot.draft));

		if (!success) {
			return;
		}

		this.store.setState((previous) => ({ ...previous, submitting: false }));
	};

	isStepComplete = (stepIndex: number): boolean => {
		const step = this.steps[stepIndex];

		if (!step) {
			return false;
		}

		return step.isComplete(this.store.state.draft);
	};

	canSubmit = (): boolean => {
		const draft = this.store.state.draft;
		return this.steps.every((step) => step.isComplete(draft));
	};

	private runAsync = async (
		action: () => Promise<void> | void,
	): Promise<boolean> => {
		try {
			await action();
			return true;
		} catch (cause) {
			const message = cause instanceof Error ? cause.message : String(cause);

			this.store.setState((previous) => ({
				...previous,
				submitting: false,
				error: message,
			}));

			return false;
		}
	};
}

export const createWizardController = <TDraft>(
	args: WizardControllerArgs<TDraft>,
): WizardController<TDraft> => new WizardController(args);
