package main

import (
	"context"
	"database/sql"
	"flag"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/cedaesca/patient-finder/internal/persons"
	"github.com/cedaesca/patient-finder/internal/search"
	typesense "github.com/cedaesca/patient-finder/internal/search/typesense"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

var centerAliases = map[string]string{
	"Hospital Dr. José Gregorio Hernández":    "Hospital General del Oeste",
	"Hospital Universitario de Carac":         "Hospital Universitario de Caracas",
	"Hospital Ana Francisca Pérez de":         "Hospital Ana Francisca Pérez de León II",
	"Hospital Ana Francisca Pérez de León 2":  "Hospital Ana Francisca Pérez de León II",
	"Hospital José María Vargas - La Guaira":  "Hospital José María Vargas",
}

func main() {
	excelPath := flag.String("file", "pacientes.xlsx", "path to the Excel file")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	dbSvc, err := database.New()
	if err != nil {
		slog.Error("database connection", "err", err)
		os.Exit(1)
	}
	db := dbSvc.GetDbInstance()
	defer db.Close()

	ctx := context.Background()

	engine := initSearchEngine(ctx)

	store := persons.NewPostgresPersonsStore(db)
	svc := persons.NewPersonsService(store, engine)

	centerMap := loadCenters(db)
	if len(centerMap) == 0 {
		slog.Error("no centers found — run the center seeder migration first")
		os.Exit(1)
	}

	rescueEstadoID, rescueMunicipioID, rescueParroquiaID := loadRescueGeography(db)

	f, err := excelize.OpenFile(*excelPath)
	if err != nil {
		slog.Error("open excel", "err", err)
		os.Exit(1)
	}
	defer f.Close()

	rows, err := f.GetRows("\U0001f50d BUSCAR PACIENTES")
	if err != nil {
		slog.Error("read sheet", "err", err)
		os.Exit(1)
	}
	if len(rows) < 4 {
		slog.Error("no data rows in master sheet")
		os.Exit(1)
	}

	source := "pacientes.xlsx"
	imported := 0
	skipped := 0

	for i, row := range rows {
		if i < 3 {
			continue
		}
		if len(row) < 3 {
			skipped++
			continue
		}

		hospital := strings.TrimSpace(row[1])
		fullName := strings.TrimSpace(row[2])
		if fullName == "" {
			skipped++
			continue
		}

		centerID, ok := centerMap[hospital]
		if !ok {
			slog.Warn("unknown center", "name", hospital, "row", i+1)
			skipped++
			continue
		}

		firstName, lastName := parseName(fullName)

		var age *int
		if len(row) > 3 {
			a, _ := strconv.Atoi(strings.TrimSpace(row[3]))
			if a > 0 {
				age = &a
			}
		}

		var cedula *string
		if len(row) > 4 {
			c := cleanCedula(row[4])
			if c != "" {
				cedula = &c
			}
		}

		// Skip records with no identifier (need cedula or both names)
		if cedula == nil && (firstName == nil || lastName == nil) {
			slog.Warn("skip: no identifier", "name", fullName, "row", i+1)
			skipped++
			continue
		}

		var contacts *string
		if len(row) > 5 {
			phone := strings.TrimSpace(row[5])
			if phone != "" {
				contacts = &phone
			}
		}

		var notes string
		if len(row) > 7 {
			obs := strings.TrimSpace(row[7])
			if obs != "" {
				notes = "Observaciones: " + obs
			}
		}

		sourceID := ""
		if len(row) > 0 {
			sourceID = strings.TrimSpace(row[0])
		}

		var exists bool
		err := db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM persons WHERE source=$1 AND source_id=$2 AND deleted_at IS NULL)",
			source, sourceID).Scan(&exists)
		if err != nil {
			slog.Warn("check duplicate", "source_id", sourceID, "err", err)
		}
		if exists {
			skipped++
			continue
		}

		input := persons.CreatePersonInput{
			FirstName:         firstName,
			LastName:          lastName,
			Cedula:            cedula,
			AgeApprox:         age,
			Sex:               nil,
			Status:            "hospitalized",
			AdmittedAt:        time.Now(),
			RescueEstadoID:    rescueEstadoID,
			RescueMunicipioID: rescueMunicipioID,
			RescueParroquiaID: &rescueParroquiaID,
			CenterID:          centerID,
			Contacts:          contacts,
			Notes:             notes,
			Source:            &source,
			SourceID:          &sourceID,
		}

		if _, err := svc.Create(ctx, input, nil); err != nil {
			slog.Error("create person", "name", fullName, "cedula", cedula, "err", err)
			skipped++
			continue
		}
		imported++
	}

	slog.Info("import complete", "imported", imported, "skipped", skipped)
}

func initSearchEngine(ctx context.Context) search.Engine {
	engine, err := typesense.NewEngineFromEnv()
	if err != nil {
		slog.Warn("typesense not configured, search indexing will be skipped during import")
		return nil
	}
	cfg := persons.PersonCollection
	if err := engine.CreateCollection(ctx, cfg); err != nil {
		slog.Warn("create typesense collection", "err", err)
	}
	return engine
}

func loadCenters(db *sql.DB) map[string]uuid.UUID {
	rows, err := db.Query(`SELECT id, name FROM centers WHERE is_active = true`)
	if err != nil {
		slog.Error("query centers", "err", err)
		return nil
	}
	defer rows.Close()

	m := make(map[string]uuid.UUID)
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			slog.Error("scan center", "err", err)
			continue
		}
		m[name] = id
	}

	// Add aliases
	for alias, canonical := range centerAliases {
		if id, ok := m[canonical]; ok {
			m[alias] = id
		}
	}

	return m
}

func loadRescueGeography(db *sql.DB) (uuid.UUID, uuid.UUID, uuid.UUID) {
	var estadoID, municipioID, parroquiaID uuid.UUID
	err := db.QueryRow(`
		SELECT e.id, m.id, p.id
		FROM estados e
		JOIN municipios m ON m.estado_id = e.id
		JOIN parroquias p ON p.municipio_id = m.id
		WHERE e.name = 'La Guaira' AND m.name = 'Vargas' AND p.name = 'La Guaira'
	`).Scan(&estadoID, &municipioID, &parroquiaID)
	if err != nil {
		slog.Error("lookup rescue geography (La Guaira / Vargas / La Guaira)", "err", err)
		os.Exit(1)
	}
	return estadoID, municipioID, parroquiaID
}

func parseName(full string) (firstName, lastName *string) {
	parts := strings.Fields(full)
	if len(parts) == 0 {
		return nil, nil
	}
	if len(parts) == 1 {
		return &parts[0], &parts[0]
	}
	// Venezuelan format: APELLIDO[S] NOMBRE[S]
	fn := parts[len(parts)-1]
	ln := strings.Join(parts[:len(parts)-1], " ")
	return &fn, &ln
}

var nonDigits = regexp.MustCompile(`\D`)

func cleanCedula(raw string) string {
	s := nonDigits.ReplaceAllString(strings.TrimSpace(raw), "")
	if len(s) > 8 {
		s = s[len(s)-8:]
	}
	return s
}
