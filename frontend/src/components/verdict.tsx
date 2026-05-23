interface Props {
	allow: boolean;
	inPlay: boolean;
}

export const VerdictBadge = ({ allow, inPlay }: Props) => {
	if (allow) {
		return (
			<span className="rounded bg-(--dash-live-bg) px-1.5 py-0.5 text-[10px] font-semibold uppercase text-(--dash-live-text)">
				allow
			</span>
		);
	}
	if (!inPlay) {
		return (
			<span className="rounded bg-black/10 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-(--dash-muted) dark:bg-white/5">
				below
			</span>
		);
	}
	return (
		<span className="rounded bg-(--dash-off-bg) px-1.5 py-0.5 text-[10px] font-semibold uppercase text-(--dash-off-text)">
			deny
		</span>
	);
};
