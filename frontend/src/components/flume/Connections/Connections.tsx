import { CONNECTIONS_ID } from "#/components/flume/constants";
import styles from "./Connections.module.css";

interface ConnectionsProps {
	editorId: string;
}

const Connections = ({ editorId }: ConnectionsProps) => (
	<div className={styles.svgWrapper} id={`${CONNECTIONS_ID}${editorId}`} />
);

export default Connections;
