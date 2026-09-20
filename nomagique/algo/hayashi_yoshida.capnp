@0xefa7e2459e748f2a;

using Go = import "/go.capnp";
$Go.package("algo");
$Go.import("github.com/theapemachine/symm/nomagique/algo");

interface HayashiYoshida {
  write @0 (boundsStart1 :Int64, boundsEnd1 :Int64, boundsStart2 :Int64, boundsEnd2 :Int64, returns1 :Float64, returns2 :Float64) -> stream;
  done @1 () -> (out :Float64);
}
