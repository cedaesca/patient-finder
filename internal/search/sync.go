package search

import "context"

type CollectionReindexer struct {
	CollectionName string
	BuildDocs      func(ctx context.Context) ([]SearchDoc, error)
}
