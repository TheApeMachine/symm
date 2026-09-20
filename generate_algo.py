import os
import subprocess

operations = [
    ("RLS", 
     "algo", 
     "in :List(Float64)", 
     "float64, float64", 
     "\tDimensions int\n\tLambda float64\n\tbeta []float64\n\troot [][]float64\n\tnoiseShape float64\n\tnoiseScale float64\n\tobservations float64", 
     "\"math\"\n\t\"github.com/theapemachine/symm/nomagique/core\"", 
     """\tinList, _ := call.Args().In()
\tdim := s.Dimensions
\tif dim <= 0 { dim = 1 }
\tlam := s.Lambda
\tif lam <= 0 { lam = 0.99 }

\tif s.beta == nil {
\t\ts.beta = make([]float64, dim)
\t\ts.root = make([][]float64, dim)
\t\tfor i := range s.root {
\t\t\ts.root[i] = make([]float64, dim)
\t\t\ts.root[i][i] = core.Unit
\t\t}
\t\ts.noiseShape = core.Unit
\t\ts.noiseScale = core.Unit
\t}

\tif inList.Len() != dim+1 {
\t\treturn nil // Invalid input shape
\t}

\tprediction := 0.0
\tfactor := make([]float64, dim)
\tfor row := 0; row < dim; row++ {
\t\tfeature := inList.At(row)
\t\tprediction += s.beta[row] * feature
\t\tfor col, coeff := range s.root[row] {
\t\t\tfactor[col] += coeff * feature
\t\t}
\t}

\tenergy := 0.0
\tfor _, val := range factor { energy += val * val }
\tvariance := (s.noiseScale / s.noiseShape) * (s.observations + energy)

\ty := inList.At(dim)
\tinnovation := y - prediction
\talpha := lam + energy

\tif alpha > 0 {
\t\trootLambda := math.Sqrt(lam)
\t\tdenominator := alpha + rootLambda*math.Sqrt(alpha)
\t\tgain := make([]float64, dim)
\t\tfor row := range s.root {
\t\t\tfor col, coeff := range s.root[row] {
\t\t\t\tgain[row] += coeff * factor[col]
\t\t\t}
\t\t\tgain[row] /= alpha
\t\t\ts.beta[row] += gain[row] * innovation
\t\t\tfor col, coeff := range s.root[row] {
\t\t\t\ts.root[row][col] = (coeff - gain[row]*(alpha/denominator)*factor[col]) / rootLambda
\t\t\t}
\t\t}
\t\ts.observations++
\t\ts.noiseScale = lam*s.noiseScale + 0.5*innovation*innovation/alpha
\t\ts.noiseShape = lam*s.noiseShape + 0.5
\t}

\tif s.DownstreamRLS != nil {
\t\treturn s.DownstreamRLS(ctx, prediction, variance)
\t}
\treturn nil"""),

    ("GaussJordan", 
     "algo", 
     "left :List(GaussJordanRow), right :List(GaussJordanRow)", 
     "[][]float64", 
     "\tTolerance float64", 
     "\"math\"", 
     """\tleft, _ := call.Args().Left()
\tright, _ := call.Args().Right()
\trows := left.Len()
\tif rows == 0 { return nil }

\ttol := s.Tolerance
\tif tol <= 0 { tol = 1e-9 }

\trightCols := 0
\tif rows > 0 && right.Len() > 0 {
\t\tright0 := right.At(0)
\t\tvals, _ := right0.Values()
\t\trightCols = vals.Len()
\t}

\ta := make([][]float64, rows)
\tb := make([][]float64, rows)
\tfor i := 0; i < rows; i++ {
\t\tleftRow := left.At(i)
\t\tleftVals, _ := leftRow.Values()
\t\ta[i] = make([]float64, rows)
\t\tfor j := 0; j < rows; j++ { a[i][j] = leftVals.At(j) }
\t\t
\t\trightRow := right.At(i)
\t\trightVals, _ := rightRow.Values()
\t\tb[i] = make([]float64, rightCols)
\t\tfor j := 0; j < rightCols; j++ { b[i][j] = rightVals.At(j) }
\t}

\tfor col := 0; col < rows; col++ {
\t\tpivotRow := col
\t\tmaxVal := math.Abs(a[col][col])
\t\tfor r := col + 1; r < rows; r++ {
\t\t\tval := math.Abs(a[r][col])
\t\t\tif val > maxVal {
\t\t\t\tmaxVal = val
\t\t\t\tpivotRow = r
\t\t\t}
\t\t}
\t\tif maxVal <= tol { return nil }
\t\tif pivotRow != col {
\t\t\ta[col], a[pivotRow] = a[pivotRow], a[col]
\t\t\tb[col], b[pivotRow] = b[pivotRow], b[col]
\t\t}
\t\tpivot := a[col][col]
\t\tfor c := col; c < rows; c++ { a[col][c] /= pivot }
\t\tfor c := 0; c < rightCols; c++ { b[col][c] /= pivot }
\t\tfor r := 0; r < rows; r++ {
\t\t\tif r != col {
\t\t\t\tfactor := a[r][col]
\t\t\t\tfor c := col; c < rows; c++ { a[r][c] -= factor * a[col][c] }
\t\t\t\tfor c := 0; c < rightCols; c++ { b[r][c] -= factor * b[col][c] }
\t\t\t}
\t\t}
\t}
\tif s.DownstreamGaussJordan != nil {
\t\treturn s.DownstreamGaussJordan(ctx, b)
\t}
\treturn nil"""),

    ("OLS", 
     "algo", 
     "x :List(OLSRow), y :List(OLSRow)", 
     "[]float64", 
     "\tTolerance float64", 
     "", 
     """\txIn, _ := call.Args().X()
\tyIn, _ := call.Args().Y()
\trows := xIn.Len()
\tif rows == 0 { return nil }
\txIn0 := xIn.At(0)
\txIn0Vals, _ := xIn0.Values()
\tcols := xIn0Vals.Len()
\t
\tx := make([][]float64, rows)
\ty := make([][]float64, rows)
\tfor i := 0; i < rows; i++ {
\t\txInI := xIn.At(i)
\t\txInIVals, _ := xInI.Values()
\t\tx[i] = make([]float64, cols)
\t\tfor j := 0; j < cols; j++ { x[i][j] = xInIVals.At(j) }
\t\t
\t\tyInI := yIn.At(i)
\t\tyInIVals, _ := yInI.Values()
\t\ty[i] = make([]float64, 1)
\t\ty[i][0] = yInIVals.At(0)
\t}
\txtx := make([][]float64, cols)
\txty := make([][]float64, cols)
\tfor i := range xtx {
\t\txtx[i] = make([]float64, cols)
\t\txty[i] = make([]float64, 1)
\t}
\tfor r := 0; r < rows; r++ {
\t\tfor i := 0; i < cols; i++ {
\t\t\txi := x[r][i]
\t\t\tfor j := 0; j < cols; j++ {
\t\t\t\txtx[i][j] += xi * x[r][j]
\t\t\t}
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
\tif s.DownstreamOLS != nil {
\t\treturn s.DownstreamOLS(ctx, coeffs)
\t}
\treturn nil"""),

    ("HayashiYoshida", 
     "algo", 
     "boundsStart1 :Int64, boundsEnd1 :Int64, boundsStart2 :Int64, boundsEnd2 :Int64, returns1 :Float64, returns2 :Float64", 
     "float64", 
     "\tcovSum float64\n\tleftEnergySum float64\n\trightEnergySum float64", 
     "\"math\"", 
     """\tboundsStart1 := call.Args().BoundsStart1()
\tboundsEnd1 := call.Args().BoundsEnd1()
\tboundsStart2 := call.Args().BoundsStart2()
\tboundsEnd2 := call.Args().BoundsEnd2()
\treturns1 := call.Args().Returns1()
\treturns2 := call.Args().Returns2()

\tisOverlapping := boundsStart1 < boundsEnd2 && boundsStart2 < boundsEnd1
\tif isOverlapping {
\t\ts.covSum += returns1 * returns2
\t}
\ts.leftEnergySum += returns1 * returns1
\ts.rightEnergySum += returns2 * returns2

\tscale := math.Sqrt(s.leftEnergySum * s.rightEnergySum)
\tvar result float64
\tif scale > 0 {
\t\tresult = s.covSum / scale
\t}
\tif s.DownstreamHayashiYoshida != nil {
\t\treturn s.DownstreamHayashiYoshida(ctx, result)
\t}
\treturn nil""")
]

for name, pkg, capnp_args, dst_types, state_fields, imports, write_logic in operations:
    if name == "OLS":
        imports = "\"math\""

    uid = subprocess.check_output(["capnp", "id"]).decode().strip()
    file_prefix = "".join(['_'+c.lower() if c.isupper() else c for c in name]).lstrip('_')
    if name == "RLS": file_prefix = "rls"
    if name == "OLS": file_prefix = "ols"
    
    capnp_filename = f"nomagique/{pkg}/{file_prefix}.capnp"
    
    struct_def = ""
    if name in ["GaussJordan", "OLS"]:
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
{write_logic}
}}

func (s *{name}Server) Done(ctx context.Context, call {name}_done) error {{
\treturn nil
}}
""")
        
    print(f"Generated {pkg}/{file_prefix}")
