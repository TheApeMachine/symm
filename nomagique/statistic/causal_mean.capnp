@0xcb7a1e8a52b9abda;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface CausalMean {
  write @0 (in :Float64) -> stream;
  done @1 () -> (out :Float64);
}
