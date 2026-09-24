using Go = import "/go.capnp";
@0xc048f341be4b5490;
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# Authority measures how much weight each element of a list of readings has
# earned, with the definitions data.Quality uses for one measurement, applied
# to each element's own history.
#
# value and defined are this reading; prior and known are each element's
# retained state, three numbers per element: how many readings it has had,
# their summed square, and its summed SNR fraction. state and index are the
# updated records of the elements read now, to be written back.
#
# A reading's SNR is divergence²/noise variance, the reading's square over the
# element's mean square before it: the element's own earlier movement is its
# noise. standard is the reading divided by that root mean square, so it is
# dimensionless and a reading of exactly zero stays zero. It is only defined
# for an element that already has a scale; a first reading has none.
#
# maturity is 1 - 1/support over the element's readings, and snr is its mean
# SNR fraction snr/(1+snr) across them. authority is maturity × snr: an
# element earns weight by being read often and by standing above its own
# noise when it moves. An element that never moved has none.
#
# energy is standard² × authority for the elements defined now: how strongly
# each one lit up, weighted by how much it is believed.
interface Authority {
  write @0 (
    value   :List(Float64),
    defined :List(Bool),
    prior   :List(Float64),
    known   :List(Bool)
  ) -> stream;
  done @1 () -> (
    standard  :List(Float64),
    defined   :List(Bool),
    authority :List(Float64),
    maturity  :List(Float64),
    snr       :List(Float64),
    energy    :List(Float64),
    index     :List(Int64),
    state     :List(Float64)
  );
}
