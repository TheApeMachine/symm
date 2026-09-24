using Go = import "/go.capnp";
@0xc048f341be4b5490;
$Go.package("statistic");
$Go.import("github.com/theapemachine/symm/nomagique/statistic");

# Authority measures how much weight each element of a list of readings has
# earned, and scales this reading against each element's own history.
#
# value and defined are this reading; prior and known are each element's
# retained state, three numbers per element: how many readings it has had,
# their summed square, and its summed signal power. state and index are the
# updated records of the elements read now, to be written back.
#
# standard is a reading divided by the element's root mean square before it,
# so it is dimensionless and a reading of exactly zero stays zero. It is only
# defined for an element that already has a scale; a first reading has none.
#
# A reading's signal power is standard²/(1+standard²): the share of it that
# stands above the element's own typical movement. authority is an element's
# summed power over the readings of the most-read element, so it grows with
# the element's SNR and with how mature its evidence is next to its peers. An
# element that never moved has none.
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
    energy    :List(Float64),
    index     :List(Int64),
    state     :List(Float64)
  );
}
