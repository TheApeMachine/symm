import re

path = "nomagique/algo/rls_test.go"
with open(path, "r") as f:
    content = f.read()

# Make sure context is imported
if "context.Background()" not in content and "\"context\"" not in content:
    content = re.sub(
        r'import\s*\(',
        r'import (\n\t"context"',
        content,
        count=1
    )

content = re.sub(
    r'forecast := predict\(state\)',
    r'var forecast algo.RLSForecast\n\t\t\tpredict.SetDownstreamAny(func(ctx context.Context, out any) error { forecast = out.(algo.RLSForecast); return nil })\n\t\t\tpredict.WriteAny(context.Background(), state)',
    content
)

content = re.sub(
    r'posterior := update\(obs\)',
    r'var posterior algo.RLSPosterior\n\t\t\t\tupdate.SetDownstreamAny(func(ctx context.Context, out any) error { posterior = out.(algo.RLSPosterior); return nil })\n\t\t\t\tupdate.WriteAny(context.Background(), obs)',
    content
)

content = re.sub(
    r'nextForecast := predict\(nextState\)',
    r'var nextForecast algo.RLSForecast\n\t\t\t\t\tpredict.SetDownstreamAny(func(ctx context.Context, out any) error { nextForecast = out.(algo.RLSForecast); return nil })\n\t\t\t\t\tpredict.WriteAny(context.Background(), nextState)',
    content
)

with open(path, "w") as f:
    f.write(content)
