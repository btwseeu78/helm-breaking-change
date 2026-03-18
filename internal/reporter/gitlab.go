package reporter

import (
	"fmt"

	"check-breaking-change/internal/models"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// GitLabReporter creates GitLab issues when breaking changes are detected.
type GitLabReporter struct {
	client    *gitlab.Client
	projectID string
	mrID      string // optional, for issue title reference
}

// NewGitLabReporter creates a new GitLabReporter.
func NewGitLabReporter(gitlabURL, token, projectID, mrID string) (*GitLabReporter, error) {
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(gitlabURL))
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}
	return &GitLabReporter{
		client:    client,
		projectID: projectID,
		mrID:      mrID,
	}, nil
}

// Report creates a GitLab issue with the breaking change details.
func (r *GitLabReporter) Report(report models.Report) error {
	title := fmt.Sprintf("Breaking subchart changes detected in chart %q", report.ChartName)
	if r.mrID != "" {
		title = fmt.Sprintf("Breaking subchart changes detected in MR !%s for chart %q", r.mrID, report.ChartName)
	}

	body := FormatMarkdown(report)

	labels := gitlab.LabelOptions{"breaking-change", "helm"}
	opts := &gitlab.CreateIssueOptions{
		Title:       gitlab.Ptr(title),
		Description: gitlab.Ptr(body),
		Labels:      &labels,
	}

	issue, _, err := r.client.Issues.CreateIssue(r.projectID, opts)
	if err != nil {
		return fmt.Errorf("failed to create GitLab issue: %w", err)
	}

	fmt.Printf("Created GitLab issue #%d: %s\n", issue.IID, issue.WebURL)
	return nil
}
