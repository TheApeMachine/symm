@0xf867ea78c0863927;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Calibrator {
  write @0 (value :Float64) -> stream;
  done @1 () -> (out :Float64);
}
