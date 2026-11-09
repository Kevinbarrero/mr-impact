package svc

// table holds hidden as a value outside any call.
var table = map[string]func(){"a": hidden}

func hidden() {}
