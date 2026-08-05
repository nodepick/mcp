package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCommandExecHandler_Success(t *testing.T) {
	args := json.RawMessage(`{"command": "echo hello world"}`)
	res, err := CommandExecHandler(context.Background(), args)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if res.IsError {
		t.Fatalf("Expected IsError to be false, got true")
	}
	if len(res.Content) == 0 {
		t.Fatalf("Expected content in result")
	}
	output := strings.TrimSpace(res.Content[0].Text)
	if output != "hello world" {
		t.Errorf("Expected 'hello world', got %q", output)
	}
}

func TestCommandExecHandler_WithCwd(t *testing.T) {
	tmpDir := t.TempDir()
	args, _ := json.Marshal(map[string]interface{}{
		"command": "pwd",
		"cwd":     tmpDir,
	})
	res, err := CommandExecHandler(context.Background(), args)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if res.IsError {
		t.Fatalf("Expected IsError to be false, got true")
	}
	output := strings.TrimSpace(res.Content[0].Text)
	// On Mac tmpDir might resolve through symlink (e.g. /private/var), check realpath
	realTmp, _ := os.Readlink(tmpDir)
	if realTmp == "" {
		realTmp = tmpDir
	}
	if !strings.HasSuffix(output, tmpDir) && !strings.HasSuffix(output, realTmp) {
		t.Errorf("Expected output to end with %q or %q, got %q", tmpDir, realTmp, output)
	}
}

func TestCommandExecHandler_Error(t *testing.T) {
	args := json.RawMessage(`{"command": "non_existent_command_12345"}`)
	res, err := CommandExecHandler(context.Background(), args)
	if err != nil {
		t.Fatalf("Expected no Go error, got %v", err)
	}
	if !res.IsError {
		t.Fatalf("Expected IsError to be true for failed command")
	}
}

func TestCommandExecHandler_EmptyCommand(t *testing.T) {
	args := json.RawMessage(`{"command": ""}`)
	res, err := CommandExecHandler(context.Background(), args)
	if err != nil {
		t.Fatalf("Expected no Go error, got %v", err)
	}
	if !res.IsError {
		t.Fatalf("Expected IsError to be true for empty command")
	}
}

func TestCommandExecHandler_BashProcessSubstitution(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("/bin/bash not available on host system")
	}
	args := json.RawMessage(`{"command": "cat <(echo 'bash process substitution works')"}`)
	res, err := CommandExecHandler(context.Background(), args)
	if err != nil {
		t.Fatalf("Expected no Go error, got %v", err)
	}
	if res.IsError {
		t.Fatalf("Expected IsError to be false, got true with text %v", res.Content[0].Text)
	}
	output := strings.TrimSpace(res.Content[0].Text)
	if output != "bash process substitution works" {
		t.Errorf("Expected 'bash process substitution works', got %q", output)
	}
}
