import { describe, expect, it, vi } from "vitest";
import { createWizardController } from "#/components/ui/wizard/controller";
import type { WizardStepDefinition } from "#/components/ui/wizard/types";

type SampleDraft = {
	name: string;
	count: number;
};

const buildSteps = (
	overrides: Partial<{
		basicsRequired: boolean;
		countRequired: boolean;
	}> = {},
): ReadonlyArray<WizardStepDefinition<SampleDraft>> => {
	const opts = { basicsRequired: true, countRequired: true, ...overrides };

	return [
		{
			id: "basics",
			title: "Basics",
			isComplete: (draft) =>
				opts.basicsRequired ? draft.name.trim().length > 0 : true,
			render: () => null,
		},
		{
			id: "count",
			title: "Count",
			isComplete: (draft) => (opts.countRequired ? draft.count > 0 : true),
			render: () => null,
		},
		{
			id: "review",
			title: "Review",
			isComplete: () => true,
			render: () => null,
		},
	];
};

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("WizardController", () => {
	it("initializes with the supplied draft and step zero", () => {
		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "", count: 0 },
			steps: buildSteps(),
			onSubmit: vi.fn(),
		});

		expect(controller.store.state.draft).toEqual({ name: "", count: 0 });
		expect(controller.store.state.stepIndex).toBe(0);
		expect(controller.store.state.submitting).toBe(false);
		expect(controller.store.state.error).toBe(null);
	});

	it("throws when constructed with zero steps", () => {
		expect(() =>
			createWizardController<SampleDraft>({
				initialDraft: { name: "", count: 0 },
				steps: [],
				onSubmit: vi.fn(),
			}),
		).toThrow(/at least one step/);
	});

	it("merges and replaces draft state immutably", () => {
		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "", count: 0 },
			steps: buildSteps(),
			onSubmit: vi.fn(),
		});

		controller.mergeDraft({ name: "Alpha" });
		expect(controller.store.state.draft).toEqual({ name: "Alpha", count: 0 });

		controller.setDraft((draft) => ({ ...draft, count: 7 }));
		expect(controller.store.state.draft).toEqual({ name: "Alpha", count: 7 });
	});

	it("reports per-step completion against the live draft", () => {
		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "", count: 0 },
			steps: buildSteps(),
			onSubmit: vi.fn(),
		});

		expect(controller.isStepComplete(0)).toBe(false);
		expect(controller.canSubmit()).toBe(false);

		controller.mergeDraft({ name: "Alpha", count: 3 });
		expect(controller.isStepComplete(0)).toBe(true);
		expect(controller.isStepComplete(1)).toBe(true);
		expect(controller.canSubmit()).toBe(true);
	});

	it("advances through steps and persists between transitions in linear mode", async () => {
		const persistStep = vi.fn().mockResolvedValue(undefined);
		const onSubmit = vi.fn().mockResolvedValue(undefined);

		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "Alpha", count: 3 },
			steps: buildSteps(),
			persistStep,
			onSubmit,
		});

		await controller.goNext();
		expect(controller.store.state.stepIndex).toBe(1);
		expect(persistStep).toHaveBeenCalledTimes(1);
		expect(persistStep).toHaveBeenLastCalledWith(
			{ name: "Alpha", count: 3 },
			0,
		);

		await controller.goNext();
		expect(controller.store.state.stepIndex).toBe(2);
		expect(persistStep).toHaveBeenCalledTimes(2);

		await controller.goNext();
		expect(onSubmit).toHaveBeenCalledTimes(1);
		expect(persistStep).toHaveBeenCalledTimes(2);
		expect(controller.store.state.submitting).toBe(false);
	});

	it("captures persistStep failures into error state", async () => {
		const persistStep = vi.fn().mockRejectedValue(new Error("boom"));

		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "Alpha", count: 3 },
			steps: buildSteps(),
			persistStep,
			onSubmit: vi.fn(),
		});

		await controller.goNext();

		expect(controller.store.state.stepIndex).toBe(0);
		expect(controller.store.state.error).toBe("boom");
		expect(controller.store.state.submitting).toBe(false);
	});

	it("captures onSubmit failures into error state and leaves draft intact", async () => {
		const onSubmit = vi.fn().mockRejectedValue(new Error("nope"));

		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "Alpha", count: 3 },
			steps: buildSteps(),
			onSubmit,
		});

		await controller.submit();

		expect(onSubmit).toHaveBeenCalledTimes(1);
		expect(controller.store.state.error).toBe("nope");
		expect(controller.store.state.submitting).toBe(false);
		expect(controller.store.state.draft).toEqual({ name: "Alpha", count: 3 });
	});

	it("ignores submit when the draft is incomplete", async () => {
		const onSubmit = vi.fn();

		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "", count: 0 },
			steps: buildSteps(),
			onSubmit,
		});

		await controller.submit();
		await flush();

		expect(onSubmit).not.toHaveBeenCalled();
		expect(controller.store.state.submitting).toBe(false);
	});

	it("ignores concurrent goNext while submitting", async () => {
		const deferred = ((): {
			promise: Promise<void>;
			resolve: () => void;
		} => {
			let resolve: () => void = () => {};

			const promise = new Promise<void>((settle) => {
				resolve = settle;
			});

			return { promise, resolve };
		})();

		const persistStep = vi.fn(() => deferred.promise);

		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "Alpha", count: 3 },
			steps: buildSteps(),
			persistStep,
			onSubmit: vi.fn(),
		});

		const first = controller.goNext();
		await flush();
		expect(controller.store.state.submitting).toBe(true);

		await controller.goNext();
		expect(persistStep).toHaveBeenCalledTimes(1);

		deferred.resolve();
		await first;
		expect(controller.store.state.stepIndex).toBe(1);
	});

	it("goBack decrements step index but never below zero", () => {
		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "Alpha", count: 3 },
			steps: buildSteps(),
			onSubmit: vi.fn(),
		});

		controller.goBack();
		expect(controller.store.state.stepIndex).toBe(0);

		controller.store.setState((previous) => ({ ...previous, stepIndex: 2 }));
		controller.goBack();
		expect(controller.store.state.stepIndex).toBe(1);
	});

	it("jumpTo resolves a step id to its index and clears errors", () => {
		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "", count: 0 },
			steps: buildSteps(),
			onSubmit: vi.fn(),
		});

		controller.store.setState((previous) => ({
			...previous,
			error: "previous failure",
		}));

		controller.jumpTo("review");
		expect(controller.store.state.stepIndex).toBe(2);
		expect(controller.store.state.error).toBe(null);
	});

	it("jumpTo rejects unknown step ids", () => {
		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "", count: 0 },
			steps: buildSteps(),
			onSubmit: vi.fn(),
		});

		expect(() => controller.jumpTo("nope")).toThrow(/Unknown wizard step/);
	});

	it("clearError flips error back to null", () => {
		const controller = createWizardController<SampleDraft>({
			initialDraft: { name: "", count: 0 },
			steps: buildSteps(),
			onSubmit: vi.fn(),
		});

		controller.store.setState((previous) => ({ ...previous, error: "boom" }));
		controller.clearError();
		expect(controller.store.state.error).toBe(null);
	});
});
