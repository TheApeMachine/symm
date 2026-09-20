import os
import subprocess

operations = [
    ("DirectionalTarget", 
     "learning", 
     "past :Float64, current :Float64", 
     "float64", 
     "\tDeadband float64", 
     "\"math\"", 
     """\tpast := callArgs.Past()
\tcurrent := callArgs.Current()
\tdelta := current - past
\tvar result float64
\tif math.Abs(delta) > s.Deadband {
\t\tresult = math.Copysign(1, delta)
\t}
\tif s.DownstreamDirectionalTarget != nil {
\t\treturn s.DownstreamDirectionalTarget(ctx, result)
\t}
\treturn nil"""),

    ("BinaryTarget", 
     "learning", 
     "past :Float64, current :Float64", 
     "float64", 
     "", 
     "", 
     """\tpast := callArgs.Past()
\tcurrent := callArgs.Current()
\tvar result float64
\tif current > past {
\t\tresult = 1.0
\t}
\tif s.DownstreamBinaryTarget != nil {
\t\treturn s.DownstreamBinaryTarget(ctx, result)
\t}
\treturn nil"""),

    ("IdentityTarget", 
     "learning", 
     "past :Float64, current :Float64", 
     "float64", 
     "", 
     "", 
     """\tcurrent := callArgs.Current()
\tif s.DownstreamIdentityTarget != nil {
\t\treturn s.DownstreamIdentityTarget(ctx, current)
\t}
\treturn nil"""),

    ("DeltaTarget", 
     "learning", 
     "past :Float64, current :Float64", 
     "float64", 
     "", 
     "", 
     """\tpast := callArgs.Past()
\tcurrent := callArgs.Current()
\tresult := current - past
\tif s.DownstreamDeltaTarget != nil {
\t\treturn s.DownstreamDeltaTarget(ctx, result)
\t}
\treturn nil"""),

    ("RatioTarget", 
     "learning", 
     "past :Float64, current :Float64", 
     "float64", 
     "", 
     "\"math\"", 
     """\tpast := callArgs.Past()
\tcurrent := callArgs.Current()
\tvar result float64
\tif past == 0 {
\t\tresult = math.NaN()
\t} else {
\t\tresult = current/past - 1
\t}
\tif s.DownstreamRatioTarget != nil {
\t\treturn s.DownstreamRatioTarget(ctx, result)
\t}
\treturn nil"""),

    ("Forecast", 
     "learning", 
     "in :Float64", 
     "float64, float64, float64, float64", 
     "\tcount float64\n\tmean float64\n\tm2 float64\n\tm3 float64\n\tm4 float64", 
     "\"github.com/theapemachine/symm/nomagique/core\"", 
     """\tin := callArgs.In()
\ts.count++
\tn := s.count
\tdelta := in - s.mean
\tdeltaN := delta / n
\tdeltaN2 := deltaN * deltaN
\tterm1 := delta * deltaN * (n - 1)

\ts.mean += deltaN
\ts.m4 += term1*deltaN2*(n*n-3*n+3) + 6*deltaN2*s.m2 - 4*deltaN*s.m3
\ts.m3 += term1*deltaN*(n-2) - 3*deltaN*s.m2
\ts.m2 += term1

\tvariance := 0.0
\tskewness := 0.0
\tkurtosis := 0.0

\tif n > core.Unit {
\t\tvariance = s.m2 / (n - core.Unit)
\t}
\tif s.m2 > 0 {
\t\tskewness = (core.Unit * n * s.m3) / (s.m2 * s.m2)
\t\tkurtosis = (n * s.m4) / (s.m2 * s.m2)
\t}

\tif s.DownstreamForecast != nil {
\t\treturn s.DownstreamForecast(ctx, s.mean, variance, skewness, kurtosis)
\t}
\treturn nil""")
]

for name, pkg, capnp_args, dst_types, state_fields, imports, write_logic in operations:
    uid = subprocess.check_output(["capnp", "id"]).decode().strip()
    file_prefix = "".join(['_'+c.lower() if c.isupper() else c for c in name]).lstrip('_')
    capnp_filename = f"nomagique/{pkg}/{file_prefix}.capnp"
    
    with open(capnp_filename, "w") as f:
        f.write(f"""{uid};

using Go = import "/go.capnp";
$Go.package("{pkg}");
$Go.import("github.com/theapemachine/symm/nomagique/{pkg}");

interface {name} {{
  write @0 ({capnp_args}) -> stream;
  done @1 ();
}}
""")

    go_filename = f"nomagique/{pkg}/{file_prefix}.go"
    imports_str = f"import (\n\t\"context\"\n"
    if imports:
        imports_str += f"\t{imports}\n"
    imports_str += ")\n"
    
    with open(go_filename, "w") as f:
        f.write(f"""package {pkg}

{imports_str}

type {name}Server struct {{
\tDownstream{name} func(context.Context, {dst_types}) error
{state_fields}
}}

func (s *{name}Server) Write(ctx context.Context, call {name}_write) error {{
\treturn s.WriteParams(ctx, call.Args())
}}

func (s *{name}Server) WriteParams(ctx context.Context, callArgs {name}_write_Params) error {{
{write_logic}
}}

func (s *{name}Server) Done(ctx context.Context, call {name}_done) error {{
\treturn nil
}}
""")
        
    print(f"Generated {pkg}/{file_prefix}")
