import { useSelector } from "@tanstack/react-store";
import { useEffect, useRef } from "react";
import { appStore } from "#/collections/app";
import { Flex } from "./flex";
import { Grid } from "./grid";

export const Dialog = () => {
	const error = useSelector(appStore, (state) => state.error);
	const errorDialogRef = useRef<HTMLDialogElement>(null);
	const dismissRef = useRef<HTMLButtonElement>(null);

	useEffect(() => {
		const dialog = errorDialogRef.current;

		if (dialog === null) {
			return;
		}

		if (error) {
			if (!dialog.open) {
				dialog.showModal();
			}

			dismissRef.current?.focus();

			return;
		}

		if (dialog.open) {
			dialog.close();
		}
	}, [error]);

	return (
		<dialog
			ref={errorDialogRef}
			aria-label="Backend error"
			className="fixed inset-0 z-80 m-0 h-full w-full max-h-none max-w-none border-0 bg-[rgba(8,6,5,0.82)] p-6 text-[#f1d7cf]"
			onCancel={(event) => {
				event.preventDefault();
				appStore.actions.clearError();
			}}
			onClick={(event) => {
				if (event.target === event.currentTarget) {
					appStore.actions.clearError();
				}
			}}
			onKeyDown={(event) => {
				if (
					(event.key === "Enter" || event.key === " ") &&
					event.target === event.currentTarget
				) {
					event.preventDefault();
					appStore.actions.clearError();
				}
			}}
		>
			{error ? (
				<Flex.Column
					className="mx-auto max-h-full w-full max-w-3xl border border-[#5f2d2d] bg-[#1a0f0e] shadow-[0_18px_80px_rgba(0,0,0,0.55)]"
					fullWidth
				>
					<Flex.Row
						justify="between"
						align="center"
						className="border-[#5f2d2d] border-b px-4 py-3"
						fullWidth
					>
						<span className="font-mono text-[11px] tracking-[0.14em] text-[#d56b61] uppercase">
							backend error
						</span>
						<button
							ref={dismissRef}
							type="button"
							className="font-mono text-[11px] text-[#f1d7cf] underline-offset-2 hover:underline"
							onClick={() => appStore.actions.clearError()}
						>
							dismiss
						</button>
					</Flex.Row>
					<Grid
						cols={2}
						className="min-h-0 flex-1 grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-2 overflow-auto p-4 font-mono text-[11px]"
					>
						{Object.entries(error).map(([key, value]) => (
							<Flex
								key={`${key}:${value === null ? "null" : typeof value === "object" ? JSON.stringify(value) : String(value)}`}
								className="contents"
							>
								<span className="text-[#d56b61]">{key}</span>
								<span className="min-w-0 wrap-break-word text-[#f1d7cf]">
									{value === null
										? "null"
										: typeof value === "object"
											? JSON.stringify(value)
											: String(value)}
								</span>
							</Flex>
						))}
					</Grid>
				</Flex.Column>
			) : null}
		</dialog>
	);
};
