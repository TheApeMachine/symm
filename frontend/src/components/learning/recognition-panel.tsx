import { Flex } from "#/components/ui/flex";
import { Section } from "#/components/ui/section";
import { Stat } from "#/components/ui/stat";
import { Typography } from "#/components/ui/typography";

export const RecognitionPanel = () => (
	<Flex.Column className="gap-3 p-3">
		<Section fit="content">
			<Section.Header
				title="Precursor recognition"
				meta={<span data-l="recog-meta">0 frames · 0 learned situations</span>}
			/>
			<Section.Body scroll={false} className="p-3">
				<Flex.Row className="gap-6 flex-wrap">
					<Stat label="Confidence" value={<span data-metric="confidence" data-format="percent">0.0%</span>} />
					<Stat label="Contrast" value={<span data-metric="contrast" data-format="bits">0.00 bits</span>} />
					<Stat label="Spread" value={<span data-metric="ambiguity" data-format="spread">Spread 0.000</span>} />
					<Stat label="Surprisal" value={<span data-metric="surprisal" data-format="surprisal">Surprisal 0.00 nat</span>} />
					<Stat label="Resolved" value={<span data-metric="resolved" data-format="integer">0</span>} />
				</Flex.Row>
			</Section.Body>
			<Typography.Mono size="s" className="p-3 opacity-70" data-l="recog-status">
				Training precursor associations · Execution remains inert until confident
			</Typography.Mono>
		</Section>
	</Flex.Column>
);
