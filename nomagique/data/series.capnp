using Go = import "/go.capnp";
@0xc626270c60f00576;
$Go.package("data");
$Go.import("nomagique/data");

interface Series {
  write @0 (
    key :Text,
    sec :Float64,
    nsec :Float64,
    value :Float64,
    query :Bool
  ) -> stream;
  done @1 () -> (
    key :Text,
    sec :Float64,
    nsec :Float64,
    value :Float64,
    found :Bool
  );
}
