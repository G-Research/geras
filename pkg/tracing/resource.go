package tracing

import (
	"context"
	"fmt"
	"os"
	"os/user"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
)

type safeProcessOwnerProvider struct{}

var _ resource.Detector = safeProcessOwnerProvider{}

func (safeProcessOwnerProvider) Detect(context.Context) (*resource.Resource, error) {
	// Re-implement the username logic of
	// https://cs.opensource.google/go/go/+/refs/tags/go1.19.5:src/os/user/lookup_stubs.go
	// so we can use the same rules in CGO mode as well.
	username := os.Getenv("USER")
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	// Instead of returning an empty user, just convert the ID number to a string
	// and use that -- so we always provide some sort of user. (Like the 'id' program
	// would do.) This will allow us tostill provide some form of owner attribute
	// regardless of any mismatch amongst the Docker USER parameter, the
	// /etc/passwd file, the Kubernetes runAsUser setting...
	if username == "" {
		// ...but on Windows id will be -1: ignore all negatives.
		if id := os.Getuid(); id >= 0 {
			username = fmt.Sprintf("%d", id)
		} else {
			// Just give up.
			return resource.Empty(), nil
		}
	}

	return resource.NewWithAttributes(semconv.SchemaURL, semconv.ProcessOwnerKey.String(username)), nil
}

func WithSafeProcessOwner() resource.Option {
	return resource.WithDetectors(safeProcessOwnerProvider{})
}
