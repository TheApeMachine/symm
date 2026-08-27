import sqlite3
import pandas as pd
import json
import argparse
import os
import re

"""
Parse one raw capture_frames payload BLOB into a plain dict.

Unparseable or non-object JSON payloads become a synthetic {"error": ...} row
so no frame silently disappears from the dump.
"""
def flatten_payload(payload):
    try:
        if isinstance(payload, bytes):
            payload = payload.decode('utf-8')

        data = json.loads(payload)

        return data if isinstance(data, dict) else {"data": data}
    except Exception as exc:
        return {"error": str(exc)}


"""
Resolve the subscription channel a frame belongs to.

Channel data frames carry it at the top level ($.channel); some transport
responses (e.g. subscribe results) only expose it under result.channel. Frames
with neither fall back to the endpoint they arrived on, so every row is still
grouped into a file.
"""
def resolve_channel(payload, endpoint):
    data = flatten_payload(payload)

    if not isinstance(data, dict):
        return endpoint

    for channel in (data.get("channel"), (data.get("result") or {}).get("channel")):
        if channel:
            return str(channel)

    return endpoint


"""
Make a channel name safe to embed in a file name.
"""
def channel_slug(channel):
    slug = re.sub(r"[^A-Za-z0-9._-]+", "_", str(channel)).strip("._")

    return slug or "unknown"


def dump_capture_frames(db_path, output_dir, limit=100000, since=None, until=None):
    print(f"Connecting to {db_path}...")
    conn = sqlite3.connect(db_path)

    query = "SELECT capture_id, seq, received_at, endpoint, payload FROM capture_frames"
    params = []

    if since:
        query += " WHERE received_at >= ?"
        params.append(since)

    if until:
        if since:
            query += " AND received_at <="

        else:
            query += " WHERE received_at <="

        params.append(until)

    if limit:
        query += " LIMIT ?"
        params.append(limit)

    print(f"Executing query: {query} with params {params}")

    df = pd.read_sql_query(query, conn, params=params)
    conn.close()
    print(f"Fetched {len(df)} rows.")

    if df.empty:
        print("No data found for the given criteria.")
        return

    print("Flattening payloads and resolving channels...")
    df["channel"] = [
        resolve_channel(payload, endpoint)
        for payload, endpoint in zip(df["payload"], df["endpoint"])
    ]

    os.makedirs(output_dir, exist_ok=True)

    metadata_cols = ["capture_id", "seq", "received_at", "endpoint", "channel"]

    for channel, group in df.groupby("channel", sort=True):
        flattened_df = pd.json_normalize(group["payload"].map(flatten_payload).tolist())

        if "channel" in flattened_df.columns:
            # The resolved channel already lives in the metadata columns.
            flattened_df = flattened_df.drop(columns=["channel"])

        result_df = pd.concat(
            [group[metadata_cols].reset_index(drop=True), flattened_df],
            axis=1,
        )

        out_path = os.path.join(output_dir, f"{channel_slug(channel)}.csv")
        print(f"Writing {len(result_df)} rows ({result_df.shape[1]} columns) to {out_path}...")
        result_df.to_csv(out_path, index=False)

    print("Done!")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", required=True, help="Path to symm.sqlite")
    parser.add_argument("--out", required=True, help="Output directory; one <channel>.csv is written per channel")
    parser.add_argument("--limit", type=int, default=100000, help="Max rows to dump")
    parser.add_argument("--since", help="Start time (ISO format)")
    parser.add_argument("--until", help="End time (ISO format)")

    args = parser.parse_args()
    dump_capture_frames(args.db, args.out, args.limit, args.since, args.until)