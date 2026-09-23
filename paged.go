package aion2

import "iter"

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

// pages walks a listing from page 1 to upstream's LastPage, one request a page, and stops at the first error.
//
// LastPage is re-read from every page, so a cap NC moves is followed. An empty page ends the walk too.
func pages[T any](fetch func(page int) (*Paged[T], error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for page, last := 1, 1; page <= last; page++ {
			p, err := fetch(page)
			if err != nil {
				var zero T
				yield(zero, err)
				return
			}
			if len(p.Items) == 0 {
				return
			}
			for _, item := range p.Items {
				if !yield(item, nil) {
					return
				}
			}
			last = p.Page.LastPage
		}
	}
}
