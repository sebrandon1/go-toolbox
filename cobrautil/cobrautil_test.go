package cobrautil

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExecute(t *testing.T) {
	var ran bool
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error {
		ran = true
		return nil
	}}
	root.SetArgs(nil)
	root.SilenceUsage = true
	if err := Execute(root); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("command did not run")
	}
}

func TestSetVersion(t *testing.T) {
	root := &cobra.Command{Use: "app", RunE: func(*cobra.Command, []string) error { return nil }}
	SetVersion(root, "")
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "app version dev\n" {
		t.Fatalf("output=%q", got)
	}

	root2 := &cobra.Command{Use: "app"}
	SetVersion(root2, "v1.2.3")
	if root2.Version != "v1.2.3" {
		t.Fatalf("version=%q", root2.Version)
	}
}

func TestNewCompletionCommand(t *testing.T) {
	root := &cobra.Command{Use: "app"}
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.AddCommand(NewCompletionCommand(root, "app"))
	root.SetArgs([]string{"completion", "bash"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "bash completion") {
		t.Fatalf("unexpected completion output: %q", buf.String())
	}
}

func TestCompletionInvalidShell(t *testing.T) {
	root := &cobra.Command{Use: "app"}
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.AddCommand(NewCompletionCommand(root, "app"))
	root.SetArgs([]string{"completion", "tcsh"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("err=%v", err)
	}
}
