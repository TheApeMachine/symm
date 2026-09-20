@0x8465e5b07e7ec2e8;

using Go = import "/go.capnp";
$Go.package("probability");
$Go.import("github.com/theapemachine/symm/nomagique/probability");

interface Geomean {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
