package search

import "context"

type CollectionReindexer struct {
	CollectionName string
	Collection     CollectionConfig
	BuildDocs      func(ctx context.Context) ([]SearchDoc, error)
}
