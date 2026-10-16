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
}

func FromEnv() (Config, error) {
	c := Config{
		TableName:         os.Getenv("TABLE_NAME"),
		IndexName:         os.Getenv("GSI_NAME"),
		CognitoClientID:   os.Getenv("COGNITO_CLIENT_ID"),
		CognitoUserPoolID: os.Getenv("COGNITO_USER_POOL_ID"),
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
