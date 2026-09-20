import os
import subprocess

operations = [
    ("LinearPrediction", 
     "learning", 
     "coefficients :List(Float64), rawRow :List(Float64)", 
     "float64", 
     "\tFeatures []int", 
     "\"github.com/theapemachine/symm/nomagique/core\"", 
     """\tcoeffs, _ := callArgs.Coefficients()
\traw, _ := callArgs.RawRow()
\t
\tif coeffs.Len() != len(s.Features)+1 {
\t\treturn nil // Coefficient length mismatch
\t}
\t
\tdesignRow := make([]float64, 1, len(s.Features)+1)
\tdesignRow[0] = core.Unit
\t
\tfor _, feat := range s.Features {
\t\tif feat < 0 || feat >= raw.Len() {
\t\t\treturn nil // Feature out of bounds
\t\t}
\t\tdesignRow = append(designRow, raw.At(feat))
\t}
\t
\tsum := 0.0
\tfor i := 0; i < coeffs.Len(); i++ {
\t\tsum += coeffs.At(i) * designRow[i]
\t}
\t
\tif s.DownstreamLinearPrediction != nil {
\t\treturn s.DownstreamLinearPrediction(ctx, sum)
\t}
\treturn nil"""),

    ("LinearFit", 
     "learning", 
     "rows :List(LinearFitRow)", 
     "[]float64", 
     "\tTolerance float64\n\tTarget int\n\tFeatures []int", 
     "\"github.com/theapemachine/symm/nomagique/core\"\n\t\"math\"", 
     """\trowsList, _ := callArgs.Rows()
\trowCount := rowsList.Len()
\tif rowCount == 0 { return nil }
\t
\tx := make([][]float64, 0, rowCount)
\ty := make([][]float64, 0, rowCount)
\t
\tfor r := 0; r < rowCount; r++ {
\t\trowItem := rowsList.At(r)
\t\trowListVals, _ := rowItem.Values()
\t\tif s.Target < 0 || s.Target >= rowListVals.Len() {
\t\t\treturn nil
\t\t}
\t\t
\t\tdesignRow := make([]float64, 1, len(s.Features)+1)
\t\tdesignRow[0] = core.Unit
\t\t
\t\tfor _, featureIdx := range s.Features {
\t\t\tif featureIdx < 0 || featureIdx >= rowListVals.Len() {
\t\t\t\treturn nil
\t\t\t}
\t\t\tdesignRow = append(designRow, rowListVals.At(featureIdx))
\t\t}
\t\tx = append(x, designRow)
\t\ty = append(y, []float64{rowListVals.At(s.Target)})
\t}
\t
\t// OLS implementation inline to avoid circular deps
\tcols := len(x[0])
\txtx := make([][]float64, cols)
\txty := make([][]float64, cols)
\tfor i := range xtx {
\t\txtx[i] = make([]float64, cols)
\t\txty[i] = make([]float64, 1)
\t}
\tfor r := 0; r < len(x); r++ {
\t\tfor i := 0; i < cols; i++ {
\t\t\txi := x[r][i]
\t\t\tfor j := 0; j < cols; j++ { xtx[i][j] += xi * x[r][j] }
\t\t\txty[i][0] += xi * y[r][0]
\t\t}
\t}
\ttol := s.Tolerance
\tif tol <= 0 { tol = 1e-9 }
\tfor col := 0; col < cols; col++ {
\t\tpivotRow := col
\t\tmaxVal := math.Abs(xtx[col][col])
\t\tfor r := col + 1; r < cols; r++ {
\t\t\tval := math.Abs(xtx[r][col])
\t\t\tif val > maxVal { maxVal = val; pivotRow = r }
\t\t}
\t\tif maxVal <= tol { return nil }
\t\tif pivotRow != col {
\t\t\txtx[col], xtx[pivotRow] = xtx[pivotRow], xtx[col]
\t\t\txty[col], xty[pivotRow] = xty[pivotRow], xty[col]
\t\t}
\t\tpivot := xtx[col][col]
\t\tfor c := col; c < cols; c++ { xtx[col][c] /= pivot }
\t\txty[col][0] /= pivot
\t\tfor r := 0; r < cols; r++ {
\t\t\tif r != col {
\t\t\t\tfactor := xtx[r][col]
\t\t\t\tfor c := col; c < cols; c++ { xtx[r][c] -= factor * xtx[col][c] }
\t\t\t\txty[r][0] -= factor * xty[col][0]
\t\t\t}
\t\t}
\t}
\tcoeffs := make([]float64, cols)
\tfor i := 0; i < cols; i++ { coeffs[i] = xty[i][0] }
\t
\tif s.DownstreamLinearFit != nil {
\t\treturn s.DownstreamLinearFit(ctx, coeffs)
\t}
\treturn nil"""),

    ("Counterfactual", 
     "learning", 
     "history :List(CounterfactualRow), factualRow :List(Float64)", 
     "float64, float64", 
     "\tTolerance float64\n\tTarget int\n\tTreatment int\n\tLevel float64\n\tFeatures []int", 
     "\"math\"\n\t\"github.com/theapemachine/symm/nomagique/core\"", 
     """\thistoryList, _ := callArgs.History()
\tfactualRowList, _ := callArgs.FactualRow()
\t
\t// 1. Fit
\trowCount := historyList.Len()
\tx := make([][]float64, 0, rowCount)
\ty := make([][]float64, 0, rowCount)
\tfor r := 0; r < rowCount; r++ {
\t\trowItem := historyList.At(r)
\t\trowListVals, _ := rowItem.Values()
\t\tdesignRow := make([]float64, 1, len(s.Features)+1)
\t\tdesignRow[0] = core.Unit
\t\tfor _, featureIdx := range s.Features { designRow = append(designRow, rowListVals.At(featureIdx)) }
\t\tx = append(x, designRow)
\t\ty = append(y, []float64{rowListVals.At(s.Target)})
\t}
\tcols := len(x[0])
\txtx := make([][]float64, cols)
\txty := make([][]float64, cols)
\tfor i := range xtx { xtx[i] = make([]float64, cols); xty[i] = make([]float64, 1) }
\tfor r := 0; r < len(x); r++ {
\t\tfor i := 0; i < cols; i++ {
\t\t\txi := x[r][i]
\t\t\tfor j := 0; j < cols; j++ { xtx[i][j] += xi * x[r][j] }
\t\t\txty[i][0] += xi * y[r][0]
\t\t}
\t}
\ttol := s.Tolerance
\tif tol <= 0 { tol = 1e-9 }
\tfor col := 0; col < cols; col++ {
\t\tpivotRow := col
\t\tmaxVal := math.Abs(xtx[col][col])
\t\tfor r := col + 1; r < cols; r++ {
\t\t\tval := math.Abs(xtx[r][col])
\t\t\tif val > maxVal { maxVal = val; pivotRow = r }
\t\t}
\t\tif maxVal <= tol { return nil }
\t\tif pivotRow != col {
\t\t\txtx[col], xtx[pivotRow] = xtx[pivotRow], xtx[col]
\t\t\txty[col], xty[pivotRow] = xty[pivotRow], xty[col]
\t\t}
\t\tpivot := xtx[col][col]
\t\tfor c := col; c < cols; c++ { xtx[col][c] /= pivot }
\t\txty[col][0] /= pivot
\t\tfor r := 0; r < cols; r++ {
\t\t\tif r != col {
\t\t\t\tfactor := xtx[r][col]
\t\t\t\tfor c := col; c < cols; c++ { xtx[r][c] -= factor * xtx[col][c] }
\t\t\t\txty[r][0] -= factor * xty[col][0]
\t\t\t}
\t\t}
\t}
\tcoeffs := make([]float64, cols)
\tfor i := 0; i < cols; i++ { coeffs[i] = xty[i][0] }
\t
\t// 2. Factual residual
\tfactualOutcome := factualRowList.At(s.Target)
\tdesignRow := make([]float64, 1, len(s.Features)+1)
\tdesignRow[0] = core.Unit
\tfor _, feat := range s.Features { designRow = append(designRow, factualRowList.At(feat)) }
\tfactualPrediction := 0.0
\tfor i := 0; i < len(coeffs); i++ { factualPrediction += coeffs[i] * designRow[i] }
\tnoise := factualOutcome - factualPrediction
\t
\t// 3. Intervention
\tintervened := make([]float64, factualRowList.Len())
\tfor i := 0; i < factualRowList.Len(); i++ { intervened[i] = factualRowList.At(i) }
\tif s.Treatment >= 0 && s.Treatment < len(intervened) {
\t\tintervened[s.Treatment] = s.Level
\t} else {
\t\treturn nil
\t}
\t
\t// 4. Counterfactual Prediction
\tcDesignRow := make([]float64, 1, len(s.Features)+1)
\tcDesignRow[0] = core.Unit
\tfor _, feat := range s.Features { cDesignRow = append(cDesignRow, intervened[feat]) }
\tcPrediction := 0.0
\tfor i := 0; i < len(coeffs); i++ { cPrediction += coeffs[i] * cDesignRow[i] }
\tcOutcome := cPrediction + noise
\t
\t// 5. Precision
\tprecision := core.Unit / (1.0 + math.Abs(noise))
\tif s.DownstreamCounterfactual != nil {
\t\treturn s.DownstreamCounterfactual(ctx, cOutcome, precision)
\t}
\treturn nil"""),

    ("Backdoor", 
     "learning", 
     "history :List(BackdoorRow), factualRow :List(Float64)", 
     "float64", 
     "\tTolerance float64\n\tFeatures []int\n\tTarget int\n\tTreatment int\n\tLevel float64\n\tmean float64\n\tcount float64", 
     "\"math\"\n\t\"github.com/theapemachine/symm/nomagique/core\"", 
     """\thistoryList, _ := callArgs.History()
\t
\t// Fit OLS on history
\trowCount := historyList.Len()
\tx := make([][]float64, 0, rowCount)
\ty := make([][]float64, 0, rowCount)
\tfor r := 0; r < rowCount; r++ {
\t\trowItem := historyList.At(r)
\t\trowListVals, _ := rowItem.Values()
\t\tdesignRow := make([]float64, 1, len(s.Features)+1)
\t\tdesignRow[0] = core.Unit
\t\tfor _, featureIdx := range s.Features { designRow = append(designRow, rowListVals.At(featureIdx)) }
\t\tx = append(x, designRow)
\t\ty = append(y, []float64{rowListVals.At(s.Target)})
\t}
\tcols := len(x[0])
\txtx := make([][]float64, cols)
\txty := make([][]float64, cols)
\tfor i := range xtx { xtx[i] = make([]float64, cols); xty[i] = make([]float64, 1) }
\tfor r := 0; r < len(x); r++ {
\t\tfor i := 0; i < cols; i++ {
\t\t\txi := x[r][i]
\t\t\tfor j := 0; j < cols; j++ { xtx[i][j] += xi * x[r][j] }
\t\t\txty[i][0] += xi * y[r][0]
\t\t}
\t}
\ttol := s.Tolerance
\tif tol <= 0 { tol = 1e-9 }
\tfor col := 0; col < cols; col++ {
\t\tpivotRow := col
\t\tmaxVal := math.Abs(xtx[col][col])
\t\tfor r := col + 1; r < cols; r++ {
\t\t\tval := math.Abs(xtx[r][col])
\t\t\tif val > maxVal { maxVal = val; pivotRow = r }
\t\t}
\t\tif maxVal <= tol { return nil }
\t\tif pivotRow != col {
\t\t\txtx[col], xtx[pivotRow] = xtx[pivotRow], xtx[col]
\t\t\txty[col], xty[pivotRow] = xty[pivotRow], xty[col]
\t\t}
\t\tpivot := xtx[col][col]
\t\tfor c := col; c < cols; c++ { xtx[col][c] /= pivot }
\t\txty[col][0] /= pivot
\t\tfor r := 0; r < cols; r++ {
\t\t\tif r != col {
\t\t\t\tfactor := xtx[r][col]
\t\t\t\tfor c := col; c < cols; c++ { xtx[r][c] -= factor * xtx[col][c] }
\t\t\t\txty[r][0] -= factor * xty[col][0]
\t\t\t}
\t\t}
\t}
\tcoeffs := make([]float64, cols)
\tfor i := 0; i < cols; i++ { coeffs[i] = xty[i][0] }
\t
\texpectation := 0.0
\ts.count = 0
\ts.mean = 0
\tfor r := 0; r < rowCount; r++ {
\t\trowItem := historyList.At(r)
\t\trowListVals, _ := rowItem.Values()
\t\tintervened := make([]float64, rowListVals.Len())
\t\tfor i := 0; i < rowListVals.Len(); i++ { intervened[i] = rowListVals.At(i) }
\t\tif s.Treatment < 0 || s.Treatment >= len(intervened) { return nil }
\t\tintervened[s.Treatment] = s.Level
\t\t
\t\tdesignRow := make([]float64, 1, len(s.Features)+1)
\t\tdesignRow[0] = core.Unit
\t\tfor _, feat := range s.Features { designRow = append(designRow, intervened[feat]) }
\t\t
\t\tprediction := 0.0
\t\tfor i := 0; i < len(coeffs); i++ { prediction += coeffs[i] * designRow[i] }
\t\t
\t\ts.count++
\t\tdelta := prediction - s.mean
\t\ts.mean += delta / s.count
\t\texpectation = s.mean
\t}
\t
\tif s.DownstreamBackdoor != nil {
\t\treturn s.DownstreamBackdoor(ctx, expectation)
\t}
\treturn nil"""),

    ("Pace", 
     "learning", 
     "errorMagnitude :Float64", 
     "float64", 
     "\tRest float64\n\tLower float64\n\tUpper float64\n\tGain float64\n\tBand float64\n\tWindow int\n\t// state\n\thistory []float64\n\tema float64\n\tcount int", 
     "\"math\"\n\t\"sort\"", 
     """\terrorMag := callArgs.ErrorMagnitude()
\t
\t// 1. Maintain history and map error magnitude to empirical rank (Calibrator)
\tif s.history == nil {
\t\ts.history = make([]float64, 0, s.Window)
\t}
\t
\tif len(s.history) >= s.Window {
\t\ts.history = s.history[1:] // pop first
\t}
\ts.history = append(s.history, errorMag)
\t
\t// Compute empirical rank
\tsorted := make([]float64, len(s.history))
\tcopy(sorted, s.history)
\tsort.Float64s(sorted)
\t
\trank := 0.0
\tfor i, v := range sorted {
\t\tif errorMag <= v {
\t\t\trank = float64(i) / float64(len(sorted)-1)
\t\t\tbreak
\t\t}
\t\tif i == len(sorted)-1 {
\t\t\trank = 1.0
\t\t}
\t}
\tif len(sorted) <= 1 { rank = 0.5 }
\t
\t// 2. Map rank to target log-alpha based on bands (Threshold)
\trestLog := math.Log(s.Rest)
\tlowerLog := math.Log(s.Lower)
\tupperLog := math.Log(s.Upper)
\t
\ttargetLog := restLog
\tif rank < s.Band {
\t\ttargetLog = lowerLog
\t} else if rank > 1.0-s.Band {
\t\ttargetLog = upperLog
\t}
\t
\t// 3. Smooth target log-alpha with EMA
\tif s.count == 0 {
\t\ts.ema = targetLog
\t} else {
\t\ts.ema = s.Gain*targetLog + (1.0-s.Gain)*s.ema
\t}
\ts.count++
\t
\t// 4. Clamp log space
\tif s.ema < lowerLog { s.ema = lowerLog }
\tif s.ema > upperLog { s.ema = upperLog }
\t
\t// 5. Exp
\talpha := math.Exp(s.ema)
\t
\t// 6. Clamp hard bounds
\tif alpha < s.Lower { alpha = s.Lower }
\tif alpha > s.Upper { alpha = s.Upper }
\t
\tif s.DownstreamPace != nil {
\t\treturn s.DownstreamPace(ctx, alpha)
\t}
\treturn nil""")
]

for name, pkg, capnp_args, dst_types, state_fields, imports, write_logic in operations:
    uid = subprocess.check_output(["capnp", "id"]).decode().strip()
    file_prefix = "".join(['_'+c.lower() if c.isupper() else c for c in name]).lstrip('_')
    capnp_filename = f"nomagique/{pkg}/{file_prefix}.capnp"
    
    struct_def = ""
    if name in ["LinearFit", "Counterfactual", "Backdoor"]:
        struct_def = f"""struct {name}Row {{
  values @0 :List(Float64);
}}
"""

    with open(capnp_filename, "w") as f:
        f.write(f"""{uid};

using Go = import "/go.capnp";
$Go.package("{pkg}");
$Go.import("github.com/theapemachine/symm/nomagique/{pkg}");

{struct_def}
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
