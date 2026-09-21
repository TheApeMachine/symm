@0xd3478951e734bd48;

using Go = import "/go.capnp";
$Go.package("ui");
$Go.import("github.com/theapemachine/symm/nomagique/ui");

struct Section {
	spans @0 :List(Int64);
}

interface UISection {
	write @0 (
		top    :Int64,
		right  :Int64,
		bottom :Int64,
		left   :Int64
	) -> stream;
	done @1 ();
}
