import os
import glob

files = glob.glob("nomagique/arithmetic/*.go") + glob.glob("nomagique/calculus/*.go")

for file in files:
    # Skip the generated capnp files
    if file.endswith(".capnp.go"):
        continue
        
    with open(file, "r") as f:
        content = f.read()
    
    # We are looking to replace:
    # 	if s.Downstream != nil {
    # 		return s.Downstream(ctx, result)
    # 	}
    # 	return nil
    
    # with:
    # 	return s.Downstream(ctx, result)
    
    # Since formatting might vary slightly with tabs, we can do a targeted string replace
    
    old_block_1 = """\tif s.Downstream != nil {
\t\treturn s.Downstream(ctx, result)
\t}
\treturn nil"""
    
    new_block_1 = """\treturn s.Downstream(ctx, result)"""
    
    if old_block_1 in content:
        content = content.replace(old_block_1, new_block_1)
        with open(file, "w") as f:
            f.write(content)
            
    # For relative_change and second_difference, the logic might differ slightly, let's verify if they used result.
    # Ah, all files used 'result := ...' and then the exact same downstream block.
    
    print(f"Updated {file}")
