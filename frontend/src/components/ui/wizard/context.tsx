"use client";

import { useSelector } from "@tanstack/react-store";
import { createContext, type ReactNode, useContext, useRef } from "react";
import {
	createWizardController,
	type WizardController,
	type WizardControllerArgs,
} from "#/components/ui/wizard/controller";

const WizardContext = createContext<WizardController<unknown> | null>(null);

/*
useWizardController lazy-instantiates a controller once per component
lifetime. The ref keeps the instance stable across renders without
useState, and the args are read only on first construction.
*/
export const useWizardController = <TDraft,>(
	args: WizardControllerArgs<TDraft>,
): WizardController<TDraft> => {
	const ref = useRef<WizardController<TDraft> | null>(null);

	if (ref.current === null) {
		ref.current = createWizardController(args);
	}

	return ref.current;
};

export const WizardProvider = <TDraft,>({
	controller,
	children,
}: {
	controller: WizardController<TDraft>;
	children: ReactNode;
}) => (
	<WizardContext.Provider value={controller as WizardController<unknown>}>
		{children}
	</WizardContext.Provider>
);

export const useWizard = <TDraft,>(): WizardController<TDraft> => {
	const controller = useContext(WizardContext);

	if (!controller) {
		throw new Error("useWizard must be used within a <Wizard> tree");
	}

	return controller as WizardController<TDraft>;
};

export const useWizardDraft = <TDraft,>(): TDraft => {
	const controller = useWizard<TDraft>();
	return useSelector(controller.store, (state) => state.draft);
};

export const useWizardStepIndex = (): number => {
	const controller = useWizard<unknown>();
	return useSelector(controller.store, (state) => state.stepIndex);
};

export const useWizardSubmitting = (): boolean => {
	const controller = useWizard<unknown>();
	return useSelector(controller.store, (state) => state.submitting);
};

export const useWizardError = (): string | null => {
	const controller = useWizard<unknown>();
	return useSelector(controller.store, (state) => state.error);
};
