package launch

// fakeSignalMode plays no mode on Windows: the modes that need Unix signals
// belong to the tests in run_unix_test.go.
func fakeSignalMode(string, func(name, s string)) (code int, ok bool) {
	return 0, false
}
