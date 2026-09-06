"""Exact raw JSON and supported compression are part of the replay contract."""
import json
import pathlib
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from compression import zstd

class ExportTest(unittest.TestCase):
    def test_exact_public_payload(self):
        with tempfile.TemporaryDirectory() as directory:
            database = pathlib.Path(directory) / "events.sqlite"
            output = pathlib.Path(directory) / "capture.jsonl"
            connection = sqlite3.connect(database)
            connection.execute("CREATE TABLE events(run_id,capture_seq,stream,stream_epoch,stream_seq,kind,endpoint,at,data,encoding)")
            payload = b'{"price":12345.67000000000000000001,"qty":0.0000000010000}'
            for sequence, kind, encoding in [(1,"level3","identity"),(2,"ticker","zstd"),(3,"balances","identity")]:
                connection.execute("INSERT INTO events VALUES(?,?,?,?,?,?,?,?,?,?)",("run",sequence,"public",1,sequence,kind,"ws","2026-09-06T00:00:00Z",zstd.compress(payload) if encoding=="zstd" else payload,encoding))
            connection.commit()
            connection.close()
            subprocess.run([sys.executable,str(pathlib.Path(__file__).with_name("export.py")),"--database",str(database),"--run","run","--through-sequence","3","--output",str(output)],check=True,capture_output=True)
            lines=output.read_bytes().splitlines()
            self.assertEqual(len(lines),2)
            for index,line in enumerate(lines):
                self.assertIn(payload,line)
                self.assertEqual(json.loads(line)["capture"]["identity"]["sequence"],index+1)

if __name__ == "__main__":
    unittest.main()
