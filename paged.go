package aion2

// Paged is one page of a listing
type Paged[T any] struct {
	Page  PageInfo
	Items []T
}

// PageInfo is upstream's own pagination, and Total is not a count to rely on.
//
// The item dictionary reports every match but serves only the first 10,000, so total can exceed LastPage x Size.
//
// Character search stops counting at 10,000 altogether. Loop on LastPage.
type PageInfo struct {
	Page     int
	Size     int
	Total    int
	LastPage int
}
