package config

import (
	"strings"
	"testing"
)

func TestFromEnv(t *testing.T) {
	t.Setenv("TABLE_NAME", "table")
	t.Setenv("GSI_NAME", "")
	t.Setenv("COGNITO_CLIENT_ID", "client")
	t.Setenv("COGNITO_USER_POOL_ID", "")

	_, err := FromEnv()
	if err == nil || !strings.Contains(err.Error(), "[COGNITO_USER_POOL_ID GSI_NAME]") {
		t.Fatalf("err = %v", err)
	}

	t.Setenv("GSI_NAME", "GSI1")
	t.Setenv("COGNITO_USER_POOL_ID", "pool")
	t.Setenv("REFERENCIAS_CROSSREF_URL", "")
	t.Setenv("REFERENCIAS_OPENALEX_URL", "http://openalex.local")
	c, err := FromEnv()
	want := Config{
		TableName: "table", IndexName: "GSI1", CognitoClientID: "client", CognitoUserPoolID: "pool",
		CrossrefURL: "https://api.crossref.org", OpenAlexURL: "http://openalex.local",
	}
	if err != nil || c != want {
		t.Fatalf("FromEnv() = %+v, %v", c, err)
	}
}
