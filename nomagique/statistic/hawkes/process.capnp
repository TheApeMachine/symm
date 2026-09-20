using Go = import "/go.capnp";
@0xb6ad448353121c47;
$Go.package("hawkes");
$Go.import("nomagique/statistic/hawkes");

interface Process {
  write @0 (time :Float64, mark :Float64) -> stream;
  done @1 () -> (
    eventCount :Float64,
    buyCount :Float64,
    sellCount :Float64,
    buyFraction :Float64,
    sellFraction :Float64,
    arrivalRate :Float64,
    buyRate :Float64,
    sellRate :Float64,
    lambda :Float64,
    lambdaBuy :Float64,
    lambdaSell :Float64,
    spectralRadius :Float64
  );
}
