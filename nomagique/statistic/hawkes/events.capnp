@0xa5775e7f61fff1b5;

using Go = import "/go.capnp";
$Go.package("hawkes");
$Go.import("github.com/theapemachine/symm/nomagique/statistic/hawkes");

# Events retains one observed realisation of a multivariate point process:
# the time of every arrival and the component it landed on. It is the only
# node here that remembers anything about the process being observed; every
# other node is handed the window and is free of state.
#
# Arrivals at or before origin are prehistory: they still excite later
# arrivals but are not themselves counted observations, which is what keeps
# the likelihood well defined at the start of a window.
interface Events {
  write @0 (
    time      :Float64,
    component :Float64,
    dimension :Int32,
    capacity  :Int32,
  ) -> stream;

  done @1 () -> (
    times      :List(Float64),
    components :List(Float64),
    origin     :Float64,
    horizon    :Float64,
    span       :Float64,
    count      :Float64,
    counts     :List(Float64),
  );
}
