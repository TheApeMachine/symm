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
					<Stat
						label="Learned"
						value={
							<span data-metric="decisions" data-format="integer">
								0
							</span>
						}
					/>
					<Stat
						label="Evaluated"
						value={
							<span data-metric="evaluated" data-format="integer">
								0
							</span>
						}
					/>
					<Stat
						label="Accuracy"
						value={
							<span data-metric="accuracy" data-format="percent">
								—
							</span>
						}
					/>
					<Stat
						label="Unsupported"
						value={
							<span data-metric="unsupported" data-format="integer">
								0
							</span>
						}
					/>
					<Stat
						label="Resolved"
						value={
							<span data-metric="resolved" data-format="integer">
								0
							</span>
						}
					/>
				</Flex.Row>
			</Section.Body>
			<Typography.Mono
				size="s"
				className="p-3 opacity-70"
				data-l="recog-status"
			>
				Training precursor associations · Quoted returns, no orders
			</Typography.Mono>
		</Section>
	</Flex.Column>
);
