@0xc24dee0f894b6bc7;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface CircularMean {
  write @0 (
    angle :Float64,
  ) -> stream;

  done @1 () -> (
    meanAngle :Float64,
  );
}
