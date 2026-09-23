package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFilePreservesProcessEnvironment(t *testing.T) {
	const existingKey = "WFC_ENV_TEST_EXISTING"
	const newKey = "WFC_ENV_TEST_NEW"
	t.Setenv(existingKey, "from-shell")
	os.Unsetenv(newKey)
	t.Cleanup(func() { os.Unsetenv(newKey) })

	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(existingKey+"=from-file\n"+newKey+"=loaded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadEnvFile(filepath.Join(t.TempDir(), "missing.env"), path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != path || os.Getenv(existingKey) != "from-shell" || os.Getenv(newKey) != "loaded" {
		t.Fatalf("unexpected dotenv result: loaded=%q existing=%q new=%q", loaded, os.Getenv(existingKey), os.Getenv(newKey))
	}
}
