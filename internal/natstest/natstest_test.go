package natstest

import (
	"strings"
	"testing"
)

const expectedPinnedNATSImage = "nats:2.15.0-alpine@sha256:ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4"

// spec: openspec/specs/framework-adoption/spec.md#Requirement: Frozen framework adoption preserves qualified product behavior
func TestPinnedImage(t *testing.T) {
	t.Run("embedded_nats_image_matches_semengine_digest", func(t *testing.T) {
		got, err := pinnedImage(imagePin)
		if err != nil {
			t.Fatalf("pinnedImage(imagePin) unexpected error: %v", err)
		}
		if got != expectedPinnedNATSImage {
			t.Fatalf("pinnedImage(imagePin) = %q, want %q", got, expectedPinnedNATSImage)
		}
	})

	t.Run("accepts_tagged_and_untagged_nats_sha256_digests", func(t *testing.T) {
		validDigest := "ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4"
		cases := []struct {
			name string
			raw  string
			want string
		}{
			{
				name: "tagged_with_comments_and_blank_lines",
				raw:  "\n# header comment\n  nats:2.15.0-alpine@sha256:" + validDigest + " \n\n",
				want: "nats:2.15.0-alpine@sha256:" + validDigest,
			},
			{
				name: "untagged_digest_only",
				raw:  "nats@sha256:" + validDigest + "\n",
				want: "nats@sha256:" + validDigest,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := pinnedImage(tc.raw)
				if err != nil {
					t.Fatalf("pinnedImage(%q) unexpected error: %v", tc.raw, err)
				}
				if got != tc.want {
					t.Fatalf("pinnedImage(%q) = %q, want %q", tc.raw, got, tc.want)
				}
			})
		}
	})

	t.Run("rejects_missing_multiple_unpinned_or_non_nats_references", func(t *testing.T) {
		validDigest := "ac8f88a6494bffc2c2a5289a0ca61cb28a9145c11ba5677cf24265d07f46d8d4"
		cases := []struct {
			name    string
			raw     string
			wantSub string
		}{
			{
				name:    "empty_file",
				raw:     "",
				wantSub: "0 image lines",
			},
			{
				name:    "comments_and_whitespace_only",
				raw:     "# comment only\n   \n# second comment\n",
				wantSub: "0 image lines",
			},
			{
				name:    "multiple_image_lines",
				raw:     "nats:2.15.0-alpine@sha256:" + validDigest + "\nnats@sha256:" + validDigest + "\n",
				wantSub: "2 image lines",
			},
			{
				name:    "tag_only_without_digest",
				raw:     "nats:2.15.0-alpine\n",
				wantSub: "not a valid NATS digest reference",
			},
			{
				name:    "wrong_repository_name",
				raw:     "redis:7@sha256:" + validDigest + "\n",
				wantSub: "not a valid NATS digest reference",
			},
			{
				name:    "truncated_sha256_digest",
				raw:     "nats:2.15.0-alpine@sha256:ac8f88a6494bffc2\n",
				wantSub: "not a valid NATS digest reference",
			},
			{
				name:    "uppercase_hex_digest",
				raw:     "nats:2.15.0-alpine@sha256:" + strings.ToUpper(validDigest) + "\n",
				wantSub: "not a valid NATS digest reference",
			},
			{
				name:    "invalid_tag_start",
				raw:     "nats:-badtag@sha256:" + validDigest + "\n",
				wantSub: "not a valid NATS digest reference",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := pinnedImage(tc.raw)
				if err == nil {
					t.Fatalf("pinnedImage(%q) = %q, expected error containing %q", tc.raw, got, tc.wantSub)
				}
				if !strings.Contains(err.Error(), tc.wantSub) {
					t.Fatalf("pinnedImage(%q) error = %q, want substring %q", tc.raw, err.Error(), tc.wantSub)
				}
			})
		}
	})
}

// spec: openspec/specs/framework-adoption/spec.md#Requirement: Frozen framework adoption preserves qualified product behavior
func TestValidateOptions(t *testing.T) {
	cfg, err := validateOptions([]Option{WithMaxPayload(4096)})
	if err != nil {
		t.Fatalf("validateOptions(WithMaxPayload(4096)) unexpected error: %v", err)
	}
	if !cfg.maxPayloadSet || cfg.maxPayload != 4096 {
		t.Fatalf("validateOptions(WithMaxPayload(4096)) = %+v, want maxPayload=4096", cfg)
	}

	for _, bad := range []int64{0, -1, -1024} {
		if _, err := validateOptions([]Option{WithMaxPayload(bad)}); err == nil {
			t.Fatalf("validateOptions(WithMaxPayload(%d)) succeeded, want non-positive payload error", bad)
		}
	}
}
