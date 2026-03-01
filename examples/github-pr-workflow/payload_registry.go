package githubprworkflow

import "github.com/c360studio/semstreams/component"

// init registers all GitHub entity payload types with the global PayloadRegistry.
// This enables BaseMessage.UnmarshalJSON to recreate typed payloads from JSON
// when the message type matches one of the github entity types.
func init() {
	registrations := []component.PayloadRegistration{
		{
			Domain:      domainGitHub,
			Category:    categoryIssueEntity,
			Version:     schemaVersion,
			Description: "GitHub issue graph entity",
			Factory:     func() any { return &GitHubIssueEntity{} },
		},
		{
			Domain:      domainGitHub,
			Category:    categoryPREntity,
			Version:     schemaVersion,
			Description: "GitHub pull request graph entity",
			Factory:     func() any { return &GitHubPREntity{} },
		},
		{
			Domain:      domainGitHub,
			Category:    categoryReviewEntity,
			Version:     schemaVersion,
			Description: "GitHub code review graph entity",
			Factory:     func() any { return &GitHubReviewEntity{} },
		},
	}

	for i := range registrations {
		if err := component.RegisterPayload(&registrations[i]); err != nil {
			panic("register github entity: " + err.Error())
		}
	}
}
