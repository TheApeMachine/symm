@0xd4e5b2ed5b75bf08;

using Go = import "/go.capnp";
$Go.package("ui");
$Go.import("github.com/theapemachine/symm/nomagique/ui");

enum ComponentType {
	alert       @0;
	badge       @1;
	button      @2;
	callout     @3;
	canvas      @4;
	card        @5;
	checkbox    @6;
	chip        @7;
	collapsible @8;
	command     @9;
}

enum Variant {
	brand    @0;
	info     @1;
	success  @2;
	warning  @3;
	error    @4;
}

struct Component {
	type       @0 :ComponentType;
	variant    @1 :Variant;
	components @2 :List(Component);
}

interface UIComponent {
	write @0 (
		type       :ComponentType,
		variant    :Variant,
		components :List(Component)
	) -> stream;
	done @1 ();
}
