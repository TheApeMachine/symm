using Go = import "/go.capnp";
@0xd28f099c0a6f44d9;
$Go.package("data");
$Go.import("nomagique/data");

interface Quality {
  write @0 (
    support :Float64,
    divergence :Float64,
    noiseVariance :Float64,
    mahalanobisSNR :Float64,
    maturity :Float64
  ) -> stream;
  done @1 () -> (
    snr :Float64,
    snrDefined :Bool,
    estimated :Bool,
    maturity :Float64
  );
}
