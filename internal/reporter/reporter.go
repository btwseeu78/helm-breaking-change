package reporter

import (
	"check-breaking-change/internal/models"
)

// Reporter defines the interface for reporting breaking change results.
type Reporter interface {
	// Report publishes the breaking change report.
	// Returns an error if reporting fails.
	Report(report models.Report) error
}
