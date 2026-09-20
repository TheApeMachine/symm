@0x82b3128748d034d9;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface GeometricMean {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
