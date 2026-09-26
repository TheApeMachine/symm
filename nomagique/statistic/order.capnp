@0xa7c4e91b56d3f208;

using Go = import "/go.capnp";
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# Order summarises a set by where its observations fall rather than by what
# they average to. A median and an interquartile range describe a cross
# section that a mean and a standard deviation would misreport whenever a few
# members move far more than the rest — which, in a market, is most of the
# time.
#
# Several producers land on value, so a cross section grows by wiring one more
# member in.
interface Order {
  write @0 (value :List(Float64)) -> stream;
  done @1 () -> (
    median            :Float64,
    lowerQuartile     :Float64,
    upperQuartile     :Float64,
    interquartile     :Float64,
    medianAbsolute    :Float64,
    extremeMagnitude  :Float64,
    extremeSigned     :Float64,
    extremeIndex      :Float64,
    extremeProminence :Float64,
    extremeCurvature  :Float64,
    count             :Float64,
    positive          :Float64,
    negative          :Float64,
    zero              :Float64,
    sumAbsolute       :Float64,
    meanAbsolute      :Float64,
    rms               :Float64,
    medianDeviation   :Float64,
    magnitudeDeviation :Float64
  );
}
