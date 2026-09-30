import os
import re

def refactor_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    original = content

    content = re.sub(
        r'([a-zA-Z0-9_]+),\s*([a-zA-Z0-9_]+)\s*:=\s*([a-zA-Z0-9_\.\*]+)\.GetMetric\(([^)]+)\)',
        r'\1, \2 := \3.LookupMetric(\4)',
        content
    )
    
    content = re.sub(
        r'([a-zA-Z0-9_]+),\s*([a-zA-Z0-9_]+)\s*=\s*([a-zA-Z0-9_\.\*]+)\.GetMetric\(([^)]+)\)',
        r'\1, \2 = \3.LookupMetric(\4)',
        content
    )

    if content != original:
        with open(filepath, 'w') as f:
            f.write(content)
        print(f"Refactored {filepath}")

for root, _, files in os.walk('.'):
    if 'scratch' in root or '.gemini' in root or '.git' in root or 'tools' in root:
        continue
    for file in files:
        if file.endswith('.go'):
            refactor_file(os.path.join(root, file))
