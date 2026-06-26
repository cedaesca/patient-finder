package typesense

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/cedaesca/patient-finder/internal/search"
	"github.com/typesense/typesense-go/v4/typesense"
	"github.com/typesense/typesense-go/v4/typesense/api"
)

type Engine struct {
	client *typesense.Client
}

func NewEngine(host, apiKey string) (*Engine, error) {
	if host == "" {
		host = "http://localhost:8108"
	}
	client := typesense.NewClient(
		typesense.WithServer(host),
		typesense.WithAPIKey(apiKey),
	)
	return &Engine{client: client}, nil
}

func NewEngineFromEnv() (*Engine, error) {
	host := os.Getenv("TYPESENSE_HOST")
	port := os.Getenv("TYPESENSE_PORT")
	apiKey := os.Getenv("TYPESENSE_API_KEY")
	if host == "" || apiKey == "" {
		return nil, fmt.Errorf("TYPESENSE_HOST and TYPESENSE_API_KEY must be set")
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		if port == "" {
			port = "8108"
		}
		host = fmt.Sprintf("http://%s:%s", host, port)
	}
	return NewEngine(host, apiKey)
}

func (e *Engine) CreateCollection(ctx context.Context, config search.CollectionConfig) error {
	fields := make([]api.Field, 0, len(config.Fields)+2)
	fields = append(fields, api.Field{Name: "id", Type: "string"})
	fields = append(fields, api.Field{Name: "collection", Type: "string"})
	for _, f := range config.Fields {
		t := f.Type
		if t == "" {
			t = "string"
		}
		field := api.Field{
			Name:     f.Name,
			Type:     t,
			Facet:    &f.Facet,
			Optional: &f.Optional,
		}
		fields = append(fields, field)
	}

	schema := &api.CollectionSchema{
		Name:   config.Name,
		Fields: fields,
	}

	_, err := e.client.Collections().Create(ctx, schema)
	if err != nil && strings.Contains(err.Error(), "already exists") {
		return nil
	}
	return err
}

func (e *Engine) Index(ctx context.Context, collection string, doc search.SearchDoc) error {
	tsDoc := typesenseDocFromSearchDoc(doc, collection)
	action := api.Create
	params := &api.ImportDocumentsParams{Action: &action}
	_, err := e.client.Collection(collection).Documents().Import(ctx, []interface{}{tsDoc}, params)
	if err != nil {
		return fmt.Errorf("index document: %w", err)
	}
	return nil
}

func (e *Engine) Delete(ctx context.Context, collection, code string) error {
	docID := fmt.Sprintf("%s_%s", collection, code)
	_, err := e.client.Collection(collection).Document(docID).Delete(ctx)
	if err != nil && strings.Contains(err.Error(), "Could not find") {
		return nil
	}
	return err
}

func (e *Engine) Search(ctx context.Context, collection, query string, page, pageSize int, filters map[string]string) ([]search.SearchHit, int, error) {
	q := query
	queryBy := "search_text"
	pagePtr := page
	perPagePtr := pageSize
	includeFields := "code"

	params := &api.SearchCollectionParams{
		Q:             &q,
		QueryBy:       &queryBy,
		Page:          &pagePtr,
		PerPage:       &perPagePtr,
		IncludeFields: &includeFields,
	}

	if filterBy := buildFilterBy(filters); filterBy != "" {
		params.FilterBy = &filterBy
	}

	res, err := e.client.Collection(collection).Documents().Search(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("typesense search: %w", err)
	}

	hits := make([]search.SearchHit, 0)
	if res.Hits != nil {
		for _, h := range *res.Hits {
			score := 0.0
			if h.TextMatchInfo != nil && h.TextMatchInfo.Score != nil {
				if s, err := parseScore(*h.TextMatchInfo.Score); err == nil {
					score = s
				}
			}
			doc := make(map[string]interface{})
			if h.Document != nil {
				doc = *h.Document
			}
			hits = append(hits, search.SearchHit{
				Document: doc,
				Score:    score,
			})
		}
	}

	found := 0
	if res.Found != nil {
		found = *res.Found
	}

	return hits, found, nil
}

func (e *Engine) ReindexAll(ctx context.Context, collection string, docs []search.SearchDoc) error {
	dropErr := e.dropCollection(ctx, collection)
	if dropErr != nil {
		return fmt.Errorf("drop collection: %w", dropErr)
	}

	fields := make([]api.Field, 0, 4)
	fields = append(fields, api.Field{Name: "id", Type: "string"})
	fields = append(fields, api.Field{Name: "collection", Type: "string"})
	fields = append(fields, api.Field{Name: "code", Type: "string"})
	fields = append(fields, api.Field{Name: "search_text", Type: "string"})
	if len(docs) > 0 {
		for k := range docs[0].Facets {
			opt := true
			facet := true
			fields = append(fields, api.Field{
				Name: k, Type: "string", Facet: &facet, Optional: &opt,
			})
		}
	}

	schema := &api.CollectionSchema{Name: collection, Fields: fields}
	_, err := e.client.Collections().Create(ctx, schema)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	if len(docs) == 0 {
		return nil
	}

	tsDocs := make([]interface{}, 0, len(docs))
	for _, d := range docs {
		tsDocs = append(tsDocs, typesenseDocFromSearchDoc(d, collection))
	}

	action := api.Create
	params := &api.ImportDocumentsParams{Action: &action}
	_, err = e.client.Collection(collection).Documents().Import(ctx, tsDocs, params)
	if err != nil {
		return fmt.Errorf("import documents: %w", err)
	}

	return nil
}

func (e *Engine) Health(ctx context.Context) error {
	_, err := e.client.Health(ctx, 0)
	if err != nil {
		return fmt.Errorf("typesense health check failed: %w", err)
	}
	return nil
}

func (e *Engine) dropCollection(ctx context.Context, name string) error {
	_, err := e.client.Collection(name).Delete(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "Could not find") {
			return nil
		}
		return err
	}
	return nil
}

func typesenseDocFromSearchDoc(doc search.SearchDoc, collection string) map[string]interface{} {
	out := make(map[string]interface{}, 4+len(doc.Facets))
	for k, v := range doc.Facets {
		out[k] = v
	}
	out["id"] = fmt.Sprintf("%s_%s", collection, doc.Code)
	out["collection"] = collection
	out["code"] = doc.Code
	out["search_text"] = doc.SearchText
	return out
}

func buildFilterBy(filters map[string]string) string {
	if len(filters) == 0 {
		return ""
	}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		v := filters[k]
		if v == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:=`%s`", k, strings.ReplaceAll(v, "`", "\\`")))
	}
	return strings.Join(parts, " && ")
}

func parseScore(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
