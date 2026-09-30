import os
import re

files_to_fix = [
    "signal/pumpdump/level3.go",
    "signal/correlation/ticker.go",
    "signal/leadlag/ticker.go",
    "signal/liquidity/ticker.go",
    "signal/sentiment/ticker.go",
    "signal/pumpdump/ticker.go",
    "signal/hawkes/trade.go",
    "signal/toxicity/trade.go",
    "signal/pumpdump/trade.go",
    "signal/depthflow/level3.go",
    "signal/morphology/level3.go",
    "signal/toxicity/level3.go",
    "signal/derivatives/ticker.go",
    "signal/derivatives/trade.go",
]

for filename in files_to_fix:
    with open(filename, 'r') as f:
        content = f.read()

    if 'pipelines sync.Map' in content:
        print(f"Skipping {filename}")
        continue

    # Add import sync if missing
    if '"sync"' not in content:
        content = re.sub(r'import \(', 'import (\n\t"sync"', content, count=1)

    # 1. Replace struct definition
    content = re.sub(r'pipeline\s+core\.Primitive', 'pipelines sync.Map', content)

    # 2. Find the pipeline definition in New*
    # It looks like:
    #   pipeline: nomagique.NewNumber(
    #      ...
    #   ),
    match = re.search(r'(pipeline:\s+nomagique\.NewNumber\([\s\S]*?\n\t\t\),)', content)
    if not match:
        print(f"Could not find pipeline in {filename}")
        continue
    
    pipeline_def_full = match.group(1)
    
    # We remove it from the New function
    content = content.replace(pipeline_def_full, '')
    
    # Clean up the pipeline definition
    pipeline_def = pipeline_def_full.replace('pipeline:', 'pipeline :=')
    # Unindent once
    lines = pipeline_def.split('\n')
    lines = [line[1:] if line.startswith('\t') else line for line in lines]
    pipeline_def = '\n'.join(lines)
    pipeline_def = pipeline_def.rstrip(',')

    # 3. Create pipelineFor method
    struct_name_match = re.search(r'func New([a-zA-Z0-9_]+)\(', content)
    if not struct_name_match:
        print(f"Could not find struct name in {filename}")
        continue
    
    struct_name = struct_name_match.group(1)
    receiver = struct_name.lower()
    
    pipelineFor_method = f"""
func ({receiver} *{struct_name}) pipelineFor(symbol string) core.Primitive {{
	if existing, ok := {receiver}.pipelines.Load(symbol); ok {{
		return existing.(core.Primitive)
	}}

	{pipeline_def}

	actual, _ := {receiver}.pipelines.LoadOrStore(symbol, pipeline)
	return actual.(core.Primitive)
}}
"""
    
    # Insert pipelineFor before Step
    content = re.sub(rf'(func \({receiver} \*{struct_name}\) Step\()', pipelineFor_method + r'\n\1', content)

    # 4. Update Step method to use pipelineFor
    # Identify the param name for measurement (either `m` or `measurement`)
    step_param_match = re.search(rf'func \({receiver} \*{struct_name}\) Step\(([a-zA-Z0-9_]+) \*data\.Measurement\[float64\]\)', content)
    param_name = step_param_match.group(1) if step_param_match else "m"

    # Now replace `.pipeline.Next(` with `.pipelineFor(m.Label).Next(`
    content = re.sub(rf'{receiver}\.pipeline\.Next', rf'{receiver}.pipelineFor({param_name}.Label).Next', content)
    
    with open(filename, 'w') as f:
        f.write(content)

    print(f"Processed {filename}")
