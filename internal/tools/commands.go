package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"mcpd/internal/mcp"
)

// GetCommandExecToolDefinition returns the MCP Tool definition for executing Linux commands.
func GetCommandExecToolDefinition() mcp.Tool {
	return mcp.Tool{
		Name:        "run_command",
		Description: "Execute a Linux command or shell script on the host VM.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The command or command line string to execute.",
				},
				"cwd": map[string]any{
					"type":        "string",
					"description": "Working directory for command execution. Optional.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Maximum execution time in seconds. Defaults to 60.",
				},
			},
			Required: []string{"command"},
		},
	}
}

// CommandExecHandler handles the execution of Linux commands.
func CommandExecHandler(ctx context.Context, arguments json.RawMessage) (mcp.CallToolResult, error) {
	var args struct {
		Command        string `json:"command"`
		Cwd            string `json:"cwd"`
		TimeoutSeconds int    `json:"timeout_seconds"`
	}

	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("invalid arguments: %w", err)
	}

	if args.Command == "" {
		return mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{Type: "text", Text: "Error: 'command' argument is required and cannot be empty"},
			},
		}, nil
	}

	timeout := 60 * time.Second
	if args.TimeoutSeconds > 0 {
		timeout = time.Duration(args.TimeoutSeconds) * time.Second
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	shell := "/bin/bash"
	if _, err := os.Stat(shell); err != nil {
		shell = "/bin/sh"
	}

	cmd := exec.CommandContext(execCtx, shell, "-c", args.Command)
	if args.Cwd != "" {
		cmd.Dir = args.Cwd
	}

	slog.Info("Executing command tool", "command", args.Command, "cwd", args.Cwd, "timeout", timeout)

	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	if execCtx.Err() == context.DeadlineExceeded {
		return mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{Type: "text", Text: fmt.Sprintf("Error: Command execution timed out after %v\nOutput before timeout:\n%s", timeout, outputStr)},
			},
		}, nil
	}

	if err != nil {
		return mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				{Type: "text", Text: fmt.Sprintf("Command failed with error: %v\nOutput:\n%s", err, outputStr)},
			},
		}, nil
	}

	if outputStr == "" {
		outputStr = "(no output)"
	}

	return mcp.CallToolResult{
		IsError: false,
		Content: []mcp.Content{
			{Type: "text", Text: outputStr},
		},
	}, nil
}
