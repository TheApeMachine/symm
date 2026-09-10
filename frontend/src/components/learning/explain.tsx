import { Icon } from "#/components/ui/icon";

/*
Explain is the prose that used to sit under a plot, folded into the one place a
reader goes looking for it.

Everything on this surface is a picture of a measurement, and the sentence that
says how to read that picture is worth having exactly once — the first time.
Left in the layout it is read once and then occupies the same column inches
forever, pushing the thing it describes off the screen. Behind this mark it
costs a hover.

It is a title attribute rather than a floating panel on purpose: a tooltip that
this surface has to position, dismiss and keep inside the viewport is a great
deal of machinery for a sentence, and the native one already survives being
rendered inside an SVG-heavy pane that clips its own overflow.
*/
export const Explain = ({ children }: { children: string }) => (
	<span
		className="inline-flex cursor-help align-middle text-(--f4) transition-colors hover:text-(--f2)"
		title={children}
		aria-label={children}
		role="note"
	>
		<Icon name="about" size="s" />
	</span>
);
