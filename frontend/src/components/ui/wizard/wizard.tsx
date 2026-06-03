"use client";

import type { ReactNode } from "react";
import {
	useWizardController,
	WizardProvider,
} from "#/components/ui/wizard/context";
import type { WizardControllerArgs } from "#/components/ui/wizard/controller";
import { WizardLinear } from "#/components/ui/wizard/linear";
import { WizardSectioned } from "#/components/ui/wizard/sectioned";
import type { WizardMode } from "#/components/ui/wizard/types";

/*
WizardProps is the declarative entry point. The caller passes the
draft shape, step config, and submit handler; the Wizard owns
controller lifecycle, navigation, and chrome.
*/
export type WizardProps<TDraft> = WizardControllerArgs<TDraft> & {
	mode: WizardMode;
	title: ReactNode;
	subtitle?: ReactNode;
	submitLabel?: string;
	submitPendingLabel?: string;
	submitIcon?: ReactNode;
	onCancel?: () => void;
	header?: ReactNode;
	preview?: ReactNode;
};

/*
Wizard is the canonical multi-step flow primitive. Linear mode walks
the user through one step at a time; sectioned mode shows every step
at once as a scroll-with-checklist. State is owned by a single
WizardController instance held in a ref so step renderers, indicator,
and chrome subscribe through the same Tanstack Store.
*/
export const Wizard = <TDraft,>({
	mode,
	title,
	subtitle,
	submitLabel,
	submitPendingLabel,
	submitIcon,
	onCancel,
	header,
	preview,
	...controllerArgs
}: WizardProps<TDraft>) => {
	const controller = useWizardController(controllerArgs);

	return (
		<WizardProvider controller={controller}>
			{mode === "linear" ? (
				<WizardLinear
					title={title}
					subtitle={subtitle}
					submitLabel={submitLabel}
					submitPendingLabel={submitPendingLabel}
					submitIcon={submitIcon}
					onCancel={onCancel}
				/>
			) : (
				<WizardSectioned
					title={title}
					subtitle={subtitle}
					submitLabel={submitLabel}
					submitPendingLabel={submitPendingLabel}
					submitIcon={submitIcon}
					onCancel={onCancel}
					header={header}
					preview={preview}
				/>
			)}
		</WizardProvider>
	);
};
