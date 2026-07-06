import { useSelector } from "@tanstack/react-store";
import { useState } from "react";
import { actionStore } from "#/collections/actions";
import { cn } from "#/lib/utils";
import { fixed } from "./decision-format";
import { DecisionSideRail } from "./decision-side";

export const DecisionsSurface = () => {
	const [selectedID, setSelectedID] = useState<string | null>(null);
	const decisions = useSelector(actionStore, (state) =>
		Object.values(state.actions).flatMap((history) => history.values()),
	);
	const allowed = decisions.filter((decision) => decision.verdict === "allow");
	const denied = decisions.filter((decision) => decision.verdict !== "allow");
	const current =
		decisions.find((decision) => String(decision.id) === selectedID) ??
		decisions.at(-1);

	return (
		<div className="grid h-full min-h-0 min-w-[1040px] grid-cols-[minmax(640px,1fr)_332px]">
			<div className="min-h-0 overflow-auto px-5 py-[18px]">
				<div className="mb-[18px] grid grid-cols-3 gap-2.5">
					<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
						<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
							Candidates
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--acc)">
							{decisions.length}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							backend actions
						</div>
					</div>

					<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
						<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
							Allowed
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--up)">
							{allowed.length}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							trader admitted
						</div>
					</div>

					<div className="rounded border border-(--line) bg-(--surface) px-3 py-2.5">
						<div className="text-[9.5px] text-(--f4) uppercase tracking-widest">
							Blocked
						</div>
						<div className="mt-0.5 font-mono font-semibold text-[26px] leading-[1.1] text-(--down)">
							{denied.length}
						</div>
						<div className="mt-px font-mono text-[9.5px] text-(--f4)">
							not executable
						</div>
					</div>
				</div>

				{current ? (
					<div className="mb-3.5 flex items-center gap-3.5 rounded border border-(--line) bg-(--sunken) px-3 py-2 font-mono text-[11.5px]">
						<span className="text-(--f3)">entry line</span>
						<span className="font-semibold text-(--acc)">
							{fixed(Number(current.entryLine))}
						</span>
						<span className="text-(--f4)">·</span>
						<span className="text-(--f3)">
							entry score {fixed(Number(current.entryScore))}
						</span>
						<span className="text-(--f4)">·</span>
						<span className="text-(--f3)">
							confidence {fixed(Number(current.entryConfidence))}
						</span>
						<span className="ml-auto text-(--f4)">backend actions only</span>
					</div>
				) : null}

        <div className="mb-2 flex items-baseline justify-between">
          <span className="font-semibold text-[10px] text-(--f3) uppercase tracking-[0.13em]">
            Candidate evaluation
          </span>
          <span className="font-mono text-[9.5px] text-(--f4)">
            click a row to inspect attribution + counterfactuals
          </span>
        </div>

        <div className="flex flex-col gap-[7px]">
          {decisions.length === 0 ? (
            <div className="rounded border border-(--line) bg-(--surface) px-3 py-8 text-center font-mono text-[11px] text-(--f4)">
              waiting for backend decision frames
            </div>
          ) : null}

          {decisions.map((decision) => {
            const id = String(decision.id);
            const selected = id === String(current?.id ?? "");

            return (
              <button
                type="button"
                key={id}
                data-symbol={String(decision.symbol ?? "")}
                onClick={() => setSelectedID(selected ? null : id)}
                className={cn(
                  "cursor-pointer rounded border bg-(--surface) px-3 py-2.5 text-left font-[inherit]",
                  selected ? "border-(--acc)" : "border-(--line)",
                )}
              >
                <div className="grid grid-cols-[88px_1fr_150px_96px] items-center gap-3">
                <div>
                  <div className="font-mono font-semibold text-[13px] text-(--f1)">
                    {String(decision.symbol)}
                  </div>
                  <div className="font-mono text-[9px] text-(--f4)">trader</div>
                </div>

                <div className="min-w-0 font-mono text-[10px]">
                  <div className="truncate text-(--f2)">
                    {String(decision.reason)}
                  </div>
                  <div className="mt-1 truncate text-(--f4)">
                    {String(decision.id)}
                  </div>
                </div>

                <div>
                  <div className="mb-0.5 flex items-center justify-between font-mono text-[9.5px] text-(--f4)">
                    <span>score</span>
                    <span className="text-(--f1)">
                      {fixed(Number(decision.score))}
                    </span>
                  </div>
                  <progress
                    className={cn(
                      "h-1.5 w-full overflow-hidden rounded-sm bg-(--line) [&::-webkit-progress-bar]:bg-(--line)",
                      decision.verdict === "allow" &&
                        "[&::-moz-progress-bar]:bg-(--up) [&::-webkit-progress-value]:bg-(--up)",
                      decision.verdict === "blocked" &&
                        "[&::-moz-progress-bar]:bg-(--down) [&::-webkit-progress-value]:bg-(--down)",
                      decision.verdict !== "allow" &&
                        decision.verdict !== "blocked" &&
                        "[&::-moz-progress-bar]:bg-(--acc) [&::-webkit-progress-value]:bg-(--acc)",
                    )}
                    max={1}
                    value={Number(decision.score)}
                  />
                </div>

                <div className="text-right">
                  <span
                    className={cn(
                      "inline-block rounded-sm px-2.5 py-1 font-semibold text-[10px] uppercase",
                      decision.verdict === "allow" &&
                        "bg-[color-mix(in_srgb,var(--up)_16%,transparent)] text-(--up)",
                      decision.verdict === "blocked" &&
                        "bg-[color-mix(in_srgb,var(--down)_14%,transparent)] text-(--down)",
                      decision.verdict !== "allow" &&
                        decision.verdict !== "blocked" &&
                        "bg-(--line) text-(--f3)",
                    )}
                  >
                    {String(decision.verdict).toUpperCase()}
                  </span>
                </div>
              </div>

                {selected ? (
                  <div className="mt-3 grid grid-cols-3 gap-2 border-(--line) border-t pt-2 font-mono text-[9.5px]">
                    <div>
                      <div className="text-(--f4)">branch</div>
                      <div className="truncate text-(--f2)">
                        {String(decision.branchKey ?? "—")}
                      </div>
                    </div>
                    <div>
                      <div className="text-(--f4)">source</div>
                      <div className="truncate text-(--f2)">
                        {String(decision.reasonSource ?? "—")}
                      </div>
                    </div>
                    <div>
                      <div className="text-(--f4)">category</div>
                      <div className="truncate text-(--f2)">
                        {String(decision.reasonCategory ?? "—")}
                      </div>
                    </div>
										<div>
											<div className="text-(--f4)">fraction</div>
											<div className="text-(--f2)">
												{fixed(Number(decision.fraction))}
											</div>
										</div>
										<div>
											<div className="text-(--f4)">price</div>
											<div className="text-(--f2)">
												{fixed(Number(decision.price))}
											</div>
										</div>
										<div>
											<div className="text-(--f4)">side</div>
											<div className="truncate text-(--f2)">
												{String(decision.side)}
											</div>
										</div>
										<div className="col-span-3 min-w-0">
											<div className="text-(--f4)">action</div>
											<div className="truncate text-(--f2)">{id}</div>
										</div>
                  </div>
                ) : null}
              </button>
            );
          })}
        </div>
      </div>

      <div className="min-h-0 overflow-auto border-(--line) border-l bg-(--surface) p-3.5">
        <DecisionSideRail symbol={current?.symbol as string | undefined} />
      </div>
    </div>
  );
};
