package tab

// loadState is a data source's fetch lifecycle. Modeling it as a single value
// makes the two facts it encodes — "is there data to show?" and "is a fetch in
// flight?" — explicit and their combinations enumerable, rather than a pair of
// correlated booleans. Crucially, srcRefreshing is a first-class state: showing
// cached data while a re-fetch runs is legitimate, not an illegal loaded+loading
// combination (L-3). Shared by the Feed, Channels, and Playlists tabs.
type loadState int

const (
	srcUnloaded   loadState = iota // no data, nothing in flight
	srcLoading                     // first fetch in flight, no data yet
	srcLoaded                      // data present, idle
	srcRefreshing                  // data present, a re-fetch in flight
)

// hasData reports whether there is data to show (loaded or refreshing).
func (s loadState) hasData() bool { return s == srcLoaded || s == srcRefreshing }

// inFlight reports whether a fetch is running (initial load or a refresh).
func (s loadState) inFlight() bool { return s == srcLoading || s == srcRefreshing }

// fetching returns the state after starting a fetch: srcRefreshing when data is
// already present (the old data keeps showing), else srcLoading.
func (s loadState) fetching() loadState {
	if s.hasData() {
		return srcRefreshing
	}
	return srcLoading
}

// settled clears the in-flight bit after a fetch finishes or fails, keeping
// whether data is present: srcLoaded when data was already shown, else srcUnloaded.
func (s loadState) settled() loadState {
	if s.hasData() {
		return srcLoaded
	}
	return srcUnloaded
}

// initLoad is srcLoading when a source must fetch at construction, else srcUnloaded.
func initLoad(need bool) loadState {
	if need {
		return srcLoading
	}
	return srcUnloaded
}
