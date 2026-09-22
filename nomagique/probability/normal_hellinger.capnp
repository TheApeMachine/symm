@0x9a58e4f20b55a21e;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface NormalHellinger {
  write @0 (
    muL :Float64,
    sigmaL :Float64,
    muR :Float64,
    sigmaR :Float64,
  ) -> stream;

  done @1 () -> (
    hellinger :Float64,
  );
}
