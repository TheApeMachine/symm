import { Section } from "#/components/ui/section";
import { Typography } from "#/components/ui/typography";
import { basis } from "./format";
import type { LearningView } from "./state";

export const CapitalPanel = ({view}: {view: LearningView | null}) => {
 const member = view?.agents[0];
 return <Section fit="content">
  <Section.Header title="Consolidated model account" meta="Independent simulated wallet · agent 1" />
  <Section.Body className="p-3 space-y-3">
   <Typography.Mono>Cash {member ? String(member.cash) : "unmeasured"} · Equity {member ? String(member.equity) : "unmeasured"} · P&L {member ? String(member.profit) : "unmeasured"}</Typography.Mono>
   <Typography.Mono>Fees {member ? String(member.fees) : "unmeasured"} · Return {member ? basis(member.wealth) : "unmeasured"}</Typography.Mono>
   <Typography.Mono>Positive explorer experience is consolidated into this model. Its own completed decisions measure its performance.</Typography.Mono>
   {member?.positions.map(position => <Typography.Mono key={String(position.holding?.symbol)}>{String(position.holding?.symbol)} · {position.status} · quantity {String(position.holding?.qty)} · mark {String(position.holding?.mark ?? "unmeasured")} · P&L {String(position.holding?.pnl ?? "unmeasured")}</Typography.Mono>)}
   {member?.positions.length === 0 && <Typography.Mono>No positions.</Typography.Mono>}
  </Section.Body>
 </Section>;
};
