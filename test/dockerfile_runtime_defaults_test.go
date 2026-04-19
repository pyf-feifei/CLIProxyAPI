package test

import (
	"strings"
	"testing"
)

func TestDockerfileProvidesDefaultConfigForContainerStartup(t *testing.T) {
	dockerfile := readProxyRepoFile(t, "Dockerfile")

	requiredSnippet := "COPY config.example.yaml /CLIProxyAPI/config.yaml"
	if !strings.Contains(dockerfile, requiredSnippet) {
		t.Fatalf("Dockerfile must include %q so the container can boot with a default config", requiredSnippet)
	}
}
