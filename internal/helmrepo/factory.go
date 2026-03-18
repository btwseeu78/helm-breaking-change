package helmrepo

import (
	"strings"

	"check-breaking-change/internal/models"
)

// IsOCIRegistry returns true if the repository URL uses the oci:// scheme.
func IsOCIRegistry(repoURL string) bool {
	return strings.HasPrefix(repoURL, "oci://")
}

// NewFetcher creates the appropriate Fetcher based on the repository URL scheme.
// OCI URLs (oci://) get an OCIFetcher; all others get an HTTPFetcher.
func NewFetcher(repoURL string, auth *models.RepoAuth) (Fetcher, error) {
	if IsOCIRegistry(repoURL) {
		return NewOCIFetcher(auth)
	}
	return NewHTTPFetcher(), nil
}
