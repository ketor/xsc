package main

import "testing"

func TestRunRejectsMissingArgumentsWithoutPanic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := [][]string{
		{"cat", "session", "--json"},
		{"get", "session", "remote"},
		{"put", "session", "local"},
		{"cp", "session", "source"},
		{"mv", "session", "source"},
	}
	for _, args := range cases {
		if code := run(args); code == 0 {
			t.Errorf("run(%v) unexpectedly succeeded", args)
		}
	}
}

func TestHasJSONFlag(t *testing.T) {
	jsonOutput, args := hasJSONFlag([]string{"session", "--json", "/tmp"})
	if !jsonOutput {
		t.Fatal("--json should enable JSON output")
	}
	if len(args) != 2 || args[0] != "session" || args[1] != "/tmp" {
		t.Fatalf("filtered args = %v", args)
	}
}
