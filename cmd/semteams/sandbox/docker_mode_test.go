package main

import (
	"strings"
	"testing"
)

func TestResolveDockerMode(t *testing.T) {
	cases := []struct {
		name       string
		flag       string
		env        string
		want       dockerMode
		wantErr    bool
		wantErrSub string
	}{
		{name: "default-none", flag: "none", want: dockerModeNone},
		{name: "explicit-dood", flag: "dood", want: dockerModeDooD},
		{name: "env-overrides-flag", flag: "none", env: "dood", want: dockerModeDooD},
		{name: "case-insensitive", flag: "DooD", want: dockerModeDooD},
		{name: "whitespace-trimmed", flag: "  dood  ", want: dockerModeDooD},
		{name: "empty-flag-defaults-none", flag: "", want: dockerModeNone},
		{name: "dind-rejected", flag: "dind", wantErr: true, wantErrSub: "future release"},
		{name: "garbage-rejected", flag: "wibble", wantErr: true, wantErrSub: "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Skip Setenv when the case wants the env var unset —
			// t.Setenv("", "") still sets the var to "" which works
			// here (resolveDockerMode treats "" as not-set), but
			// staying out of os.Setenv when the case is "no env" is
			// cleaner and avoids surprising future readers who'd
			// expect Setenv("", "") to be a no-op.
			if tc.env != "" {
				t.Setenv(dockerModeEnvVar, tc.env)
			}
			got, err := resolveDockerMode(tc.flag)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error containing %q, got mode=%q nil err", tc.wantErrSub, got)
				}
				if tc.wantErrSub != "" && !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("error %q lacks expected substring %q", err.Error(), tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got mode=%q, want %q", got, tc.want)
			}
		})
	}
}
