@0xa4cc327ccd989883;

using Go = import "/go.capnp";
$Go.package("learning");
$Go.import("github.com/theapemachine/symm/nomagique/learning");

interface Forecast {
  write @0 (value :Float64) -> stream;
  done @1 () -> (
    out :Float64,
    mean :Float64,
    variance :Float64,
    skewness :Float64,
    kurtosis :Float64
  );
}
