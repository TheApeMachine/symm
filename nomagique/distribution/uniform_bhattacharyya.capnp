@0x8a61d10308643f53;

using Go = import "/go.capnp";
$Go.package("distribution");
$Go.import("github.com/theapemachine/symm/nomagique/distribution");

interface UniformBhattacharyya {
  write @0 (
    minL :List(Float64),
    maxL :List(Float64),
    minR :List(Float64),
    maxR :List(Float64),
    dim :Int32,
  ) -> stream;

  done @1 () -> (
    bhattacharyya :Float64,
  );
}
