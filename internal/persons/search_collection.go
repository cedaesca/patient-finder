package persons

import (
	"context"
	"fmt"
	"strings"

	"github.com/cedaesca/patient-finder/internal/search"
)

var PersonCollection = search.CollectionConfig{
	Name: "persons",
	Fields: []search.Field{
		{Name: "code", Type: "string"},
		{Name: "search_text", Type: "string"},
		{Name: "sex", Type: "string", Facet: true, Optional: true},
		{Name: "rescue_estado_id", Type: "string", Facet: true, Optional: true},
		{Name: "rescue_municipio_id", Type: "string", Facet: true, Optional: true},
		{Name: "rescue_parroquia_id", Type: "string", Facet: true, Optional: true},
	},
}

type PersonsLister interface {
	ListAll(ctx context.Context) ([]PersonLite, error)
}

type PersonLite struct {
	ID                string  `json:"id"`
	FirstName         *string `json:"first_name"`
	LastName          *string `json:"last_name"`
	Cedula            *string `json:"cedula"`
	Sex               *string `json:"sex"`
	RescueEstadoID    string  `json:"rescue_estado_id"`
	RescueMunicipioID string  `json:"rescue_municipio_id"`
	RescueParroquiaID *string `json:"rescue_parroquia_id"`
}

func NewPersonReindexer(store PersonsLister) search.CollectionReindexer {
	return search.CollectionReindexer{
		CollectionName: PersonCollection.Name,
		BuildDocs: func(ctx context.Context) ([]search.SearchDoc, error) {
			persons, err := store.ListAll(ctx)
			if err != nil {
				return nil, fmt.Errorf("list all persons: %w", err)
			}

			docs := make([]search.SearchDoc, 0, len(persons))
			for _, p := range persons {
				facets := map[string]any{
					"sex":                 nil,
					"rescue_estado_id":    p.RescueEstadoID,
					"rescue_municipio_id": p.RescueMunicipioID,
				}
				if p.Sex != nil {
					facets["sex"] = *p.Sex
				}
				if p.RescueParroquiaID != nil {
					facets["rescue_parroquia_id"] = *p.RescueParroquiaID
				}

				docs = append(docs, search.SearchDoc{
					Code:       p.ID,
					SearchText: buildPersonSearchText(p),
					Facets:     facets,
				})
			}

			return docs, nil
		},
	}
}

func buildPersonSearchText(p PersonLite) string {
	var parts []string
	if p.FirstName != nil {
		parts = append(parts, *p.FirstName)
	}
	if p.LastName != nil {
		parts = append(parts, *p.LastName)
	}
	if p.Cedula != nil {
		parts = append(parts, *p.Cedula)
	}
	return strings.Join(parts, " ")
}
