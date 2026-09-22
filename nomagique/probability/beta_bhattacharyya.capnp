@0xd67cb1cc40ae47a9;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface BetaBhattacharyya {
  write @0 (
    alphaL :Float64,
    betaL :Float64,
    alphaR :Float64,
    betaR :Float64,
  ) -> stream;

  done @1 () -> (
    bhattacharyya :Float64,
  );
}
