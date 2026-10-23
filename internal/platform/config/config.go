package config

import (
	"fmt"
	"os"
	"sort"
)

type Config struct {
	TableName         string
	IndexName         string
	CognitoClientID   string
	CognitoUserPoolID string

	CrossrefURL string
	OpenAlexURL string
	// ContactEmail identifies us to Crossref and OpenAlex, which serve
	// identified clients first. Optional, like OpenAlexAPIKey.
	ContactEmail   string
	OpenAlexAPIKey string

	// UploadsBucket is optional: without it, file fields keep only the name.
	UploadsBucket string
	// S3PathStyle is for a local S3 (tests), which has no per-bucket host.
	S3PathStyle bool
}

func FromEnv() (Config, error) {
	c := Config{
		TableName:         os.Getenv("TABLE_NAME"),
		IndexName:         os.Getenv("GSI_NAME"),
		CognitoClientID:   os.Getenv("COGNITO_CLIENT_ID"),
		CognitoUserPoolID: os.Getenv("COGNITO_USER_POOL_ID"),
		CrossrefURL:       envOr("REFERENCIAS_CROSSREF_URL", "https://api.crossref.org"),
		OpenAlexURL:       envOr("REFERENCIAS_OPENALEX_URL", "https://api.openalex.org"),
		ContactEmail:      os.Getenv("REFERENCIAS_CONTATO_EMAIL"),
		OpenAlexAPIKey:    os.Getenv("OPENALEX_API_KEY"),
		UploadsBucket:     os.Getenv("UPLOADS_BUCKET"),
		S3PathStyle:       os.Getenv("AWS_ENDPOINT_URL_S3") != "",
	}
	var missing []string
	for name, value := range map[string]string{
		"TABLE_NAME":           c.TableName,
		"GSI_NAME":             c.IndexName,
		"COGNITO_CLIENT_ID":    c.CognitoClientID,
		"COGNITO_USER_POOL_ID": c.CognitoUserPoolID,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Config{}, fmt.Errorf("missing required environment variables: %v", missing)
	}
	return c, nil
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
