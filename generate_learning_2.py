import os
import subprocess

operations = [
    ("Backdoor", 
     "learning", 
     "history :List(List(Float64)), factualRow :List(Float64)", 
     "float64", 
     "\tTolerance float64\n\tFeatures []int\n\tTarget int\n\tTreatment int\n\tLevel float64\n\tmean float64\n\tcount float64", 
     "\"math\"\n\t\"github.com/theapemachine/symm/nomagique/core\"", 
     """\t// Because backdoor calls Counterfactual in a loop, it is best implemented inline
\thistoryList, _ := call.Args().History()
\tfactualRowList, _ := call.Args().FactualRow()
\t
\t// Basic stub for now, as Backdoor orchestrates multiple counterfactuals.
\tvar expectation float64
\tif s.DownstreamBackdoor != nil {
\t\treturn s.DownstreamBackdoor(ctx, expectation)
\t}
\treturn nil"""),

    ("Counterfactual", 
     "learning", 
     "history :List(List(Float64)), factualRow :List(Float64)", 
     "float64, float64", 
     "\tTolerance float64\n\tTarget int\n\tTreatment int\n\tLevel float64\n\tFeatures []int", 
     "\"math\"\n\t\"github.com/theapemachine/symm/nomagique/core\"", 
     """\thistoryList, _ := call.Args().History()
\tfactualRowList, _ := call.Args().FactualRow()
\t
\tvar counterfactualOutcome float64
\tvar precision float64
\tif s.DownstreamCounterfactual != nil {
\t\treturn s.DownstreamCounterfactual(ctx, counterfactualOutcome, precision)
\t}
\treturn nil"""),

    ("LinearPrediction", 
     "learning", 
     "coefficients :List(Float64), rawRow :List(Float64)", 
     "float64", 
     "\tFeatures []int", 
     "\"github.com/theapemachine/symm/nomagique/core\"", 
     """\tcoeffs, _ := call.Args().Coefficients()
\traw, _ := call.Args().RawRow()
\t
\tif coeffs.Len() != len(s.Features)+1 {
\t\treturn nil
\t}
\tdesign := make([]float64, 1, len(s.Features)+1)
\tdesign[0] = core.Unit
\tfor _, feat := range s.Features {
\t\tif feat < 0 || feat >= raw.Len() { return nil }
\t\tdesign = append(design, raw.At(feat))
\t}
\tsum := 0.0
\tfor i := 0; i < coeffs.Len(); i++ {
\t\tsum += coeffs.At(i) * design[i]
\t}
\tif s.DownstreamLinearPrediction != nil {
\t\treturn s.DownstreamLinearPrediction(ctx, sum)
\t}
\treturn nil"""),

    ("LinearFit", 
     "learning", 
     "rows :List(List(Float64))", 
     "[]float64", 
     "\tTolerance float64\n\tTarget int\n\tFeatures []int", 
     "", 
     """\trowsList, _ := call.Args().Rows()
\tvar coeffs []float64
\t// Stub for ols math inline or delegated
\t_ = rowsList
\tif s.DownstreamLinearFit != nil {
\t\treturn s.DownstreamLinearFit(ctx, coeffs)
\t}
\treturn nil"""),

    ("Pace", 
     "learning", 
     "errorMagnitude :Float64", 
     "float64", 
     "\tRest float64\n\tLower float64\n\tUpper float64\n\tGain float64\n\tBand float64\n\tWindow int\n\t// state\n\thistory []float64\n\tema float64\n\tcount int", 
     "\"math\"", 
     """\terrorMag := call.Args().ErrorMagnitude()
\t
\t// Very basic inline of pace math
\tvar result float64
\t_ = errorMag
\tif s.DownstreamPace != nil {
\t\treturn s.DownstreamPace(ctx, result)
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
{write_logic}
}}

func (s *{name}Server) Done(ctx context.Context, call {name}_done) error {{
\treturn nil
}}
""")
        
    print(f"Generated {pkg}/{file_prefix}")
