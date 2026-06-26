package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/cedaesca/patient-finder/internal/database"
	_ "github.com/joho/godotenv/autoload"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	dbSvc, err := database.New()
	if err != nil {
		slog.Error("database connection", "err", err)
		os.Exit(1)
	}
	db := dbSvc.GetDbInstance()
	defer db.Close()

	ctx := context.Background()

	var adminRoleID string
	err = db.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = 'admin'`).Scan(&adminRoleID)
	if err != nil {
		slog.Error("lookup admin role — run migrations first", "err", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("=== Crear cuenta de administrador ===")
	fmt.Println("(Deja el email vacio para salir)")
	fmt.Println()

	for {
		email := prompt(reader, "Email: ")
		if email == "" {
			fmt.Println("Saliendo...")
			return
		}

		name := prompt(reader, "Nombre: ")
		if name == "" {
			fmt.Println("Error: el nombre es obligatorio")
			continue
		}

		lastName := prompt(reader, "Apellido: ")
		if lastName == "" {
			fmt.Println("Error: el apellido es obligatorio")
			continue
		}

		password, err := readPassword("Contrasena: ")
		if err != nil {
			fmt.Printf("Error leyendo contrasena: %v\n", err)
			continue
		}
		if password == "" {
			fmt.Println("Error: la contrasena es obligatoria")
			continue
		}

		confirm, err := readPassword("Confirmar contrasena: ")
		if err != nil {
			fmt.Printf("Error leyendo confirmacion: %v\n", err)
			continue
		}
		if password != confirm {
			fmt.Println("Error: las contrasenas no coinciden")
			continue
		}

		userID, err := createAdmin(ctx, db, name, lastName, email, password, adminRoleID)
		if err != nil {
			fmt.Printf("Error creando admin: %v\n\n", err)
			continue
		}

		fmt.Printf("\nAdmin creado: %s (id: %s)\n\n", email, userID)
	}
}

func prompt(reader *bufio.Reader, label string) string {
	fmt.Print(label)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func readPassword(label string) (string, error) {
	fmt.Print(label)
	bytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func createAdmin(ctx context.Context, db *sql.DB, name, lastName, email, plaintextPassword, adminRoleID string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), 12)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var userID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (name, last_name, email, password_hash) VALUES ($1, $2, $3, $4) RETURNING id`,
		name, lastName, email, string(hash),
	).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO user_roles (user_id, role_id, center_id) VALUES ($1, $2, NULL) ON CONFLICT DO NOTHING`,
		userID, adminRoleID,
	)
	if err != nil {
		return "", fmt.Errorf("assign admin role: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	return userID, nil
}
