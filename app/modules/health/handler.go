// Package health is responsible fro returning the health of the app's services.
package health

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"kumacore/core/db/dialect"
	"kumacore/core/db/driver"
	"kumacore/core/db/migrate"
)

// Handler returns live and ready health check responses.
type Handler struct {
	databaseConnection driver.DBTX
	databaseDialect    dialect.Dialect
	migrationSource    migrate.Source
}

// NewHandler returns a health handler that reports DB connectivity and migration state.
func NewHandler(
	databaseConnection driver.DBTX,
	databaseDialect dialect.Dialect,
	migrationSource migrate.Source,
) *Handler {
	return &Handler{
		databaseConnection: databaseConnection,
		databaseDialect:    databaseDialect,
		migrationSource:    migrationSource,
	}
}

// Routes registers health check routes on the given router.
func Routes(handler *Handler) func(chi.Router) {
	return func(router chi.Router) {
		router.Get("/health/live", handler.Live)
		router.Get("/health/ready", handler.Ready)
	}
}

// Live reports whether the process is running.
func (handler *Handler) Live(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(writer).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("[health:Live] encode response: %v", err)
	}
}

// Ready reports whether the application is ready to serve traffic.
func (handler *Handler) Ready(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/json")

	checks := map[string]string{}

	dbCheck := checkDatabase(request.Context(), handler.databaseConnection)
	checks["database"] = dbCheck

	migrationCheck := checkMigrations(
		request.Context(),
		handler.databaseConnection,
		handler.databaseDialect,
		handler.migrationSource,
	)
	checks["migrations"] = migrationCheck

	allHealthy := true
	for _, status := range checks {
		if status != "ok" {
			allHealthy = false
			break
		}
	}

	response := map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
		"checks": checks,
	}

	if !allHealthy {
		response["status"] = "unhealthy"
		writer.WriteHeader(http.StatusServiceUnavailable)
	} else {
		writer.WriteHeader(http.StatusOK)
	}

	if err := json.NewEncoder(writer).Encode(response); err != nil {
		log.Printf("[health:Ready] encode response: %v", err)
	}
}

func checkDatabase(ctx context.Context, databaseConnection driver.DBTX) string {
	timeoutContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var result int
	if err := databaseConnection.QueryRowContext(timeoutContext, "SELECT 1").Scan(&result); err != nil {
		return "unreachable"
	}

	return "ok"
}

func checkMigrations(
	ctx context.Context,
	databaseConnection driver.DBTX,
	databaseDialect dialect.Dialect,
	migrationSource migrate.Source,
) string {
	_, err := migrate.Validate(ctx, databaseConnection, databaseDialect, migrationSource)
	if err != nil {
		return "pending or invalid"
	}

	return "ok"
}

