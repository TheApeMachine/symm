import sqlite3
import pandas as pd
import json
import argparse
import os

def dump_capture_frames(db_path, output_path, limit=100000, since=None, until=None):
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
    print(f"Fetched {len(df)} rows.")

    if df.empty:
        print("No data found for the given criteria.")
        return

    print("Flattening payloads...")
    
    def flatten_payload(row):
        try:
            p = row['payload']
            if isinstance(p, bytes):
                p = p.decode('utf-8')
            
            data = json.loads(p)
            return data
        except Exception as e:
            return {"error": str(e)}

    payload_data = df.apply(flatten_payload, axis=1).tolist()
    flattened_df = pd.json_normalize(payload_data)
    
    metadata_cols = ['capture_id', 'seq', 'received_at', 'endpoint']
    result_df = pd.concat([df[metadata_cols], flattened_df], axis=1)

    print(f"Final DataFrame shape: {result_df.shape}")
    print(f"Writing to {output_path}...")
    result_df.to_csv(output_path, index=False)
    print("Done!")

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--db", required=True, help="Path to symm.sqlite")
    parser.add_argument("--out", required=True, help="Output CSV path")
    parser.add_argument("--limit", type=int, default=100000, help="Max rows to dump")
    parser.add_argument("--since", help="Start time (ISO format)")
    parser.add_argument("--until", help="End time (ISO format)")
    
    args = parser.parse_args()
    dump_capture_frames(args.db, args.out, args.limit, args.since, args.until)
