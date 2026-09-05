"""Run the backend and dashboard as one foreground, interruptible session."""

import os
from pathlib import Path
import signal
import subprocess
import sys
import time


def main():
    root = Path(__file__).resolve().parents[1]
    children = []

    def stop(signum, _frame):
        raise SystemExit(128 + signum)

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)

    try:
        if not (root / "frontend/node_modules").is_dir():
            subprocess.run(["pnpm", "--dir", "frontend", "install", "--frozen-lockfile"],
                           cwd=root, check=True)
        for command in (
            ["go", "run", "main.go", *sys.argv[1:]],
            ["pnpm", "--dir", "frontend", "dev", "--host", "127.0.0.1", "--strictPort"],
        ):
            children.append(subprocess.Popen(command, cwd=root, start_new_session=True))
        print("symm dashboard: http://127.0.0.1:3000 · learning: http://127.0.0.1:3000/learning · Ctrl+C stops both services", flush=True)
        while True:
            for child in children:
                result = child.poll()
                if result is not None:
                    return result
            # Process supervision cadence only; this never clocks market learning.
            time.sleep(0.1)
    finally:
        for child in children:
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass  # The complete process group has already exited.
        for child in children:
            child.wait()


if __name__ == "__main__":
    sys.exit(main())
