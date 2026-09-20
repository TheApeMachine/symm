import os, re

# Fix algo_test.go
path = "nomagique/algo/algo_test.go"
with open(path, "r") as f:
    content = f.read()

# 1. NewGaussJordan(1e-9) -> NewGaussJordan(func(any) float64 { return 1e-9 })
content = re.sub(
    r'NewGaussJordan\(([^)]+)\)',
    r'NewGaussJordan(func(any) float64 { return \1 })',
    content
)

# NewOLS(1e-9) -> NewOLS(func(any) float64 { return 1e-9 })
content = re.sub(
    r'NewOLS\(([^)]+)\)',
    r'NewOLS(func(any) float64 { return \1 })',
    content
)

# 2. solver([2][][]float64{a, b}) -> streaming equivalent
# Replace:
# 			sol := solver([2][][]float64{a, b})
# With:
# 			var sol [][]float64
# 			solver.SetDownstreamAny(func(ctx context.Context, out any) error { sol = out.([][]float64); return nil })
# 			solver.WriteAny(context.Background(), [2][][]float64{a, b})
content = re.sub(
    r'sol := solver\(\[2\]\[\]\[\]float64\{a, b\}\)',
    r'var sol [][]float64\n\t\t\tsolver.SetDownstreamAny(func(ctx context.Context, out any) error { sol = out.([][]float64); return nil })\n\t\t\tsolver.WriteAny(context.Background(), [2][][]float64{a, b})',
    content
)

# Replace:
# 			beta := ols([2][][]float64{x, y})
# With:
# 			var beta []float64
# 			ols.SetDownstreamAny(func(ctx context.Context, out any) error { beta = out.([]float64); return nil })
# 			ols.WriteAny(context.Background(), [2][][]float64{x, y})
content = re.sub(
    r'beta := ols\(\[2\]\[\]\[\]float64\{x, y\}\)',
    r'var beta []float64\n\t\t\tols.SetDownstreamAny(func(ctx context.Context, out any) error { beta = out.([]float64); return nil })\n\t\t\tols.WriteAny(context.Background(), [2][][]float64{x, y})',
    content
)

# Replace:
# 			stage1 := hy([2][2]int64{{0, 10}, {5, 15}})
# 			corr := stage1([2]float64{0.02, 0.02})
# With:
# 			var stage1 any
# 			hy.SetDownstreamAny(func(ctx context.Context, out any) error { stage1 = out; return nil })
# 			hy.WriteAny(context.Background(), [2][2]int64{{0, 10}, {5, 15}})
# 			var corr float64
# 			stage1.(types.StreamNode[any, any]).SetDownstreamAny(func(ctx context.Context, out any) error { corr = out.(float64); return nil })
# 			stage1.(types.StreamNode[any, any]).WriteAny(context.Background(), [2]float64{0.02, 0.02})
# Wait, what does HayashiYoshida return? Let's check!
# If hy is a stream node, then calling hy([2][2]int64...) would return another function?
# Let's just fix it properly!
with open(path, "w") as f:
    f.write(content)

# Fix rls_test.go
path = "nomagique/algo/rls_test.go"
if os.path.exists(path):
    with open(path, "r") as f:
        content = f.read()

    # 1. NewRLS(2, 0.99) -> NewRLS(2, func(any) float64 { return 0.99 })
    # Wait, the first param is int, second is Float.
    content = re.sub(
        r'NewRLS\(([^,]+),\s*([^)]+)\)',
        r'NewRLS(\1, func(any) float64 { return \2 })',
        content
    )

    # 2. pred := predict(state, obs)
    # RLS has predict and update streams. 
    # predict is RLSPredictionNode
    content = re.sub(
        r'pred := predict\(state, obs\)',
        r'var pred float64\n\t\t\tpredict.SetDownstreamAny(func(ctx context.Context, out any) error { pred = out.(float64); return nil })\n\t\t\tpredict.WriteAny(context.Background(), []any{state, obs})',
        content
    )
    # wait, predict might just take an array or struct!
    
    with open(path, "w") as f:
        f.write(content)
