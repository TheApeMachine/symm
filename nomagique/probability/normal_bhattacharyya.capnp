@0xe5c81eeeea32139b;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface NormalBhattacharyya {
  write @0 (
    muL :Float64,
    sigmaL :Float64,
    muR :Float64,
    sigmaR :Float64,
  ) -> stream;

  done @1 () -> (
    bhattacharyya :Float64,
  );
}
