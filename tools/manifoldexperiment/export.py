"""Export an exact public-market capture prefix, preserving Hindsight identity.
The retained database is opened read-only; private account/order frames are not
selected. --through-sequence is an explicit experiment input boundary.
"""
import argparse
from compression import zstd
import json
import sqlite3

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--database", required=True)
parser.add_argument("--run", required=True)
parser.add_argument("--through-sequence", type=int, required=True)
parser.add_argument("--output", required=True)
args = parser.parse_args()
connection = sqlite3.connect(f"file:{args.database}?mode=ro", uri=True)
query = """SELECT capture_seq,stream,stream_epoch,stream_seq,kind,endpoint,at,data,encoding
FROM events WHERE run_id=? AND capture_seq>0 AND capture_seq<=?
AND kind IN ('ticker','level3','trade','instrument') ORDER BY capture_seq"""
count = 0
with open(args.output, "w") as output:
    for sequence, stream, epoch, stream_sequence, kind, endpoint, at, payload, encoding in connection.execute(query, (args.run, args.through_sequence)):
        if encoding == "zstd":
            payload = zstd.decompress(payload)
        elif encoding not in ("identity", ""):
            raise ValueError(f"unsupported capture encoding {encoding!r} at {sequence}")
        row = {"capture": {"identity": {"run": args.run, "sequence": sequence, "stream": stream, "streamEpoch": epoch, "streamSequence": stream_sequence}, "kind": kind, "endpoint": endpoint, "receivedAt": at}, "payload": None}
        # Validate JSON without passing its numeric lexemes through binary floats.
        json.loads(payload)
        payload_text = payload.decode("utf-8") if isinstance(payload, bytes) else payload
        output.write('{"capture":' + json.dumps(row["capture"], separators=(",", ":")) + ',"payload":' + payload_text.strip() + '}\n')
        count += 1
connection.close()
print(f"Exported {count} public-market frames through capture sequence {args.through_sequence}")
