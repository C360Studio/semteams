package research_test

import (
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semteams/cmd/semteams/research"
)

func TestNormalizeOriginID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		channel research.EvidenceChannel
		locator string
		want    string
		wantErr bool
	}{
		{
			name:    "github scheme normalizes owner and repo casing",
			channel: research.ChannelGitHub,
			locator: "github://C360Studio/SemTeams/issues/273",
			want:    "github://c360studio/semteams/issues/273",
		},
		{
			name:    "github https url normalizes to same origin as github scheme",
			channel: research.ChannelGitHub,
			locator: "https://github.com/C360Studio/SemTeams/issues/273#issuecomment-123",
			want:    "github://c360studio/semteams/issues/273",
		},
		{
			name:    "github release preserves tag case",
			channel: research.ChannelGitHub,
			locator: "github://C360Studio/semteams/releases/v0.2.0-RC1",
			want:    "github://c360studio/semteams/releases/v0.2.0-RC1",
		},
		{
			name:    "web strips tracking params and fragment while preserving semantic query",
			channel: research.ChannelWeb,
			locator: "HTTPS://Docs.Example.com/api/v2/?utm_source=news&id=42&fbclid=abc#section-3",
			want:    "https://docs.example.com/api/v2?id=42",
		},
		{
			name:    "semsource preserves entity identifier",
			channel: research.ChannelSemSource,
			locator: "SemSource://c360/semteams/entity/pkg.foo",
			want:    "semsource://c360/semteams/entity/pkg.foo",
		},
		{
			name:    "invalid github locator rejected",
			channel: research.ChannelGitHub,
			locator: "github://C360Studio/onlyrepo",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := research.NormalizeOriginID(tc.channel, tc.locator)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.locator, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeOriginID(%q, %q) error: %v", tc.channel, tc.locator, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeOriginID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEvidenceRecordValidate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
	valid := research.EvidenceRecord{
		Channel:     research.ChannelGitHub,
		Locator:     "github://C360Studio/semteams/issues/273",
		OriginID:    "github://c360studio/semteams/issues/273",
		SourceClass: research.SourceClassPrimary,
		RetrievedAt: now,
		RawBody:     "Issue #273 [open]: program-report: GitHub read adapter",
		Quote:       "GitHub read adapter",
	}
	if err := valid.ValidateToolResult(); err != nil {
		t.Fatalf("valid EvidenceRecord failed ValidateToolResult: %v", err)
	}

	badOrigin := valid
	badOrigin.OriginID = "github://C360Studio/semteams/issues/273"
	if err := badOrigin.Validate(); err == nil || !strings.Contains(err.Error(), "canonical normalization") {
		t.Errorf("bad OriginID should fail normalization check, got: %v", err)
	}

	badQuote := valid
	badQuote.Quote = "fabricated passage absent from raw body"
	if err := badQuote.Validate(); err == nil || !strings.Contains(err.Error(), "does not appear in evidence.raw_body") {
		t.Errorf("fabricated quote should fail Validate, got: %v", err)
	}

	missingBody := valid
	missingBody.RawBody = ""
	if err := missingBody.ValidateToolResult(); err == nil || !strings.Contains(err.Error(), "raw_body required") {
		t.Errorf("missing RawBody should fail ValidateToolResult, got: %v", err)
	}
}
