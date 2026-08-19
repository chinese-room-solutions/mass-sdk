package proclife

// parentLost returns nil: see [WatchParent] for why Windows can't answer this,
// and what covers it instead.
func parentLost() func() bool { return nil }
