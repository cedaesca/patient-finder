package persons

import (
	"context"
	"fmt"

	"github.com/cedaesca/patient-finder/internal/search"
)

var PersonCollection = search.CollectionConfig{
	Name: "persons",
	Fields: []search.Field{
		{Name: "code", Type: "string"},
		{Name: "first_name", Type: "string", Optional: true},
		{Name: "last_name", Type: "string", Optional: true},
		{Name: "cedula", Type: "string", Optional: true},
		{Name: "sex", Type: "string", Facet: true, Optional: true},
		{Name: "rescue_estado_id", Type: "string", Facet: true, Optional: true},
		{Name: "rescue_municipio_id", Type: "string", Facet: true, Optional: true},
		{Name: "rescue_parroquia_id", Type: "string", Facet: true, Optional: true},
	},
	Search: search.SearchConfig{
		QueryBy:        "first_name,last_name,cedula",
		QueryByWeights: "3,2,4",
		NumTypos:       "1,1,0",
		Prefix:         "true",
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
		Collection:     PersonCollection,
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
					Code: p.ID,
					IndexedFields: buildPersonIndexedFields(p),
					Facets:     facets,
				})
			}

			return docs, nil
		},
	}
}

func PersonToSearchDoc(person *Person) search.SearchDoc {
	facets := map[string]any{
		"sex":                 nil,
		"rescue_estado_id":    person.RescueEstadoID.String(),
		"rescue_municipio_id": person.RescueMunicipioID.String(),
	}
	if person.Sex != nil {
		facets["sex"] = *person.Sex
	}
	if person.RescueParroquiaID != nil {
		facets["rescue_parroquia_id"] = person.RescueParroquiaID.String()
	}
	return search.SearchDoc{
		Code:        person.ID.String(),
		IndexedFields: buildIndexedFields(person.FirstName, person.LastName, person.Cedula),
		Facets:     facets,
	}
}

func buildIndexedFields(firstName, lastName, cedula *string) map[string]string {
	fields := make(map[string]string)
	if firstName != nil {
		fields["first_name"] = *firstName
	}
	if lastName != nil {
		fields["last_name"] = *lastName
	}
	if cedula != nil {
		fields["cedula"] = *cedula
	}
	return fields
}

func buildPersonIndexedFields(p PersonLite) map[string]string {
	return buildIndexedFields(p.FirstName, p.LastName, p.Cedula)
}
