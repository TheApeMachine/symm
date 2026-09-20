@0xc0fa48ae932ef25d;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

struct WireRLSState {
  beta @0 :List(Float64);
  design @1 :List(Float64);
  root @2 :List(List(Float64));
  noiseShape @3 :Float64;
  noiseScale @4 :Float64;
  observations @5 :Float64;
}

struct WireRLSForecast {
  state @0 :WireRLSState;
  prediction @1 :Float64;
  factor @2 :List(Float64);
  scale @3 :Float64;
  degreesOfFreedom @4 :Float64;
  predictiveVariance @5 :Float64;
  ready @6 :Bool;
}

struct WireRLSObservation {
  forecast @0 :WireRLSForecast;
  lambda @1 :Float64;
  target @2 :Float64;
}

struct WireRLSPosterior {
  forecast @0 :WireRLSForecast;
  alpha @1 :Float64;
  innovation @2 :Float64;
  rootLambda @3 :Float64;
  gammaDenominator @4 :Float64;
  gain @5 :List(Float64);
}

struct WireSample {
  features @0 :List(Float64);
  target @1 :Float64;
  observed @2 :Bool;
}

struct WireQuery {
  rows @0 :List(List(Float64));
  features @1 :List(Int64);
  target @2 :Int64;
  treatment @3 :Int64;
  level @4 :Float64;
  actual @5 :List(Float64);
}

struct WireRLSOutput {
  value @0 :Float64;
  scale @1 :Float64;
  degreesOfFreedom @2 :Float64;
  ready @3 :Bool;
  innovation @4 :Float64;
  reset @5 :Bool;
}

struct WireResonanceLayer {
  state @0 :List(Float64);
  prediction @1 :List(Float64);
  errorNorm @2 :Float64;
  temporal @3 :Bool;
}

struct WireManifoldReading {
  reconstruction @0 :Float64;
  energy @1 :Float64;
  predictionEnergy @2 :Float64;
  reconstructionError @3 :Float64;
  energyDensity @4 :Float64;
  surprise @5 :Float64;
  temporalError @6 :Float64;
  hasTemporalError @7 :Bool;

  readoutDimension @8 :Int64;
  readout @9 :List(Float64);
  latent @10 :List(Float64);
  taskPrediction @11 :List(Float64);
  layers @12 :List(WireResonanceLayer);

  skill @13 :List(Float64);
  skillReady @14 :List(Bool);
  skillAverage @15 :Float64;
  skillReadyAvg @16 :Bool;
  precisionReady @17 :List(Bool);
  precisionAverage @18 :Float64;
  precisionReadyAvg @19 :Bool;
  scaleAverage @20 :Float64;
  scaleReadyAvg @21 :Bool;

  forecast @22 :List(WireRLSOutput);
  retention @23 :List(Float64);
}

struct SettleAction {
  features @0 :List(Float64);
  target @1 :List(Float64);
  advanceTemporal @2 :Bool;
}

struct ForecastAction {
  steps @0 :Int64;
}

struct BatchAction {
  input @0 :List(Float64);
  learn @1 :Bool;
  advanceTemporal @2 :Bool;
}

struct AlphaAction {
  alpha @0 :Float64;
}

struct ReadingAction {}

struct RetentionAction {
  steps @0 :Int64;
}

struct TaskAction {
  horizon @0 :Int64;
  features @1 :List(Float64);
  prediction @2 :Float64;
  target @3 :Float64;
}

struct WireManifoldCommand {
  settle @0 :SettleAction;
  forecast @1 :ForecastAction;
  batch @2 :BatchAction;
  alpha @3 :AlphaAction;
  reading @4 :ReadingAction;
  retention @5 :RetentionAction;
  observeTask @6 :TaskAction;
}

interface ResonanceManifold {
  write @0 (command :WireManifoldCommand) -> stream;
  done @1 ();
}
