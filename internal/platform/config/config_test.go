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
	c, err := FromEnv()
	if err != nil || c != (Config{"table", "GSI1", "client", "pool"}) {
		t.Fatalf("FromEnv() = %+v, %v", c, err)
	}
}
