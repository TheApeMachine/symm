@0xfa9c165121df4b42;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface BetaKullbackLeibler {
  write @0 (
    alphaL :Float64,
    betaL :Float64,
    alphaR :Float64,
    betaR :Float64,
  ) -> stream;

  done @1 () -> (
    kl :Float64,
  );
}
