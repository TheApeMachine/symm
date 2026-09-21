@0xefa7e2459e748f2a;

using Go = import "/go.capnp";
$Go.package("algo");
$Go.import("github.com/theapemachine/symm/nomagique/algo");

using import "../runtime/status.capnp".Status;

# HayashiYoshida is the asynchronous covariance of two return paths. It reports
# the estimate and the terms it was formed from, so a caller can audit the
# normalization rather than take the ratio on faith.
#
# Correlation is symmetric but provenance is not, so each leg carries its own
# symbol. The estimator keeps one accumulation per pair of symbols, which is
# what lets a single graph measure every pair flowing through it.
interface HayashiYoshida {
  write @0 (
    symbol1      :Text,
    symbol2      :Text,
    boundsStart1 :Float64,
    boundsEnd1   :Float64,
    returns1     :Float64,
    boundsStart2 :Float64,
    boundsEnd2   :Float64,
    returns2     :Float64
  ) -> stream;
  done @1 () -> (
    correlation :Float64,
    covariance  :Float64,
    support     :Float64,
    leftEnergy  :Float64,
    rightEnergy :Float64,
    status      :Status
  );
}
