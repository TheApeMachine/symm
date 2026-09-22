@0xa0684c4fcd8acd37;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface BetaHellinger {
  write @0 (
    alphaL :Float64,
    betaL :Float64,
    alphaR :Float64,
    betaR :Float64,
  ) -> stream;

  done @1 () -> (
    hellinger :Float64,
  );
}
