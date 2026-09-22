@0xc75b3404e312baa4;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

interface MomentAbout {
  write @0 (
    value :Float64,
    order :Float64,
    mean :Float64,
  ) -> stream;

  done @1 () -> (
    moment :Float64,
  );
}
