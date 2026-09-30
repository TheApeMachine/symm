import os
import re

def process_file(filepath):
    with open(filepath, 'r') as f:
        content = f.read()

    lines = content.split('\n')
    changed = False
    for i, line in enumerate(lines):
        if "data.NewMetric" in line:
            match = re.search(r'(data\.NewMetric\[[^\]]+\]\("([^"]+)",\s*[^,]+,\s*[^,]+,\s*)([-\d\.]+)(\s*,\s*)([-\d\.]+)(\))', line)
            if match:
                prefix = match.group(1)
                name = match.group(2)
                old_center = match.group(3)
                comma = match.group(4)
                old_scale = match.group(5)
                suffix = match.group(6)

                new_center, new_scale = "0", "0"
                if name.endswith("_zscore"):
                    new_center, new_scale = "0", "1"
                elif name.endswith("_imbalance") or name.endswith("_correlation") or name == "leadlag" or name == "sentiment_score":
                    new_center, new_scale = "0", "1"
                elif name.endswith("_fraction") or name.endswith("_ks"):
                    new_center, new_scale = "0.5", "0.5"

                new_line = line[:match.start()] + f'{prefix}{new_center}{comma}{new_scale}{suffix}' + line[match.end():]
                if new_line != line:
                    lines[i] = new_line
                    changed = True

    if changed:
        with open(filepath, 'w') as f:
            f.write('\n'.join(lines))
        print(f"Updated {filepath}")

for root, _, files in os.walk('signal'):
    for f in files:
        if f.endswith('.go'):
            process_file(os.path.join(root, f))
