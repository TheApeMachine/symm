import { useSelector } from "@tanstack/react-store";
import { appStore } from "#/collections/app";
import { decisionStore } from "#/collections/decisions";
import { tickStore } from "#/collections/tick";
import { whyLabel } from "#/components/terminal/decision-format";

export const Pulse = () => {
	const app = useSelector(appStore, (state) => state);
	const tick = useSelector(tickStore, (state) => state);
	const denied = useSelector(decisionStore, (state) => state.denied);
	const latestDenied = denied.at(-1);
	const rejectText =
		latestDenied === undefined
			? ""
			: `reject ${String(latestDenied.source ?? "trader")} ${whyLabel(
					String(latestDenied.why ?? latestDenied.reason ?? ""),
				)} x${denied.length}`;

	return (
		<div className="flex h-8 shrink-0 items-center gap-4 border-(--line) border-b bg-(--sunken) px-3.5 font-mono text-[11px] text-(--f3)">
			<span className="font-semibold text-(--f1)">
				#{String(tick?.frame?.count ?? 0)}
			</span>
			<span>
				phase{" "}
				<span className="text-(--acc)">
					{app.online ? String(tick?.frame?.phase ?? "stream") : "offline"}
				</span>
			</span>
			<span>meas {String(tick?.frame?.measurements ?? "—")}</span>
			<span>cand {String(tick?.frame?.candidates ?? "—")}</span>
			<span>open {String(tick?.frame?.open ?? "—")}</span>
			<span>
				quotes{" "}
				{tick?.frame?.quotes_ready !== undefined &&
				tick?.frame?.quotes_total !== undefined
					? `${String(tick?.frame?.quotes_ready)}/${String(tick?.frame?.quotes_total)}`
					: "—"}
			</span>
			{rejectText === "" ? null : (
				<span className="ml-auto text-(--down)">{rejectText}</span>
			)}
		</div>
	);
};
