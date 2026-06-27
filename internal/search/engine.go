package search

import "context"

type Engine interface {
	CreateCollection(ctx context.Context, config CollectionConfig) error
	Index(ctx context.Context, collection string, doc SearchDoc) error
	Delete(ctx context.Context, collection, code string) error
	Search(ctx context.Context, collection, query string, page, pageSize int, filters map[string]string) ([]SearchHit, int, error)
	ReindexAll(ctx context.Context, collection string, docs []SearchDoc) error
	Health(ctx context.Context) error
}

type SearchDoc struct {
	Code       string         `json:"code"`
	SearchText string         `json:"search_text"`
	Facets     map[string]any `json:"-"`
}

type SearchHit struct {
	Document map[string]interface{} `json:"document"`
	Score    float64                `json:"score"`
}

type CollectionConfig struct {
	Name   string
	Fields []Field
}

type Field struct {
	Name     string
	Type     string
	Facet    bool
	Optional bool
}
