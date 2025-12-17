package deterministicmap

func SomeFunc1() {
	_ = map[string]int{"a": 1} // want "use of built-in map is forbidden. use DeterministicMap instead"
}

func SomeFunc2() {
	var _ map[string]string // want "use of built-in map is forbidden. use DeterministicMap instead"
}
