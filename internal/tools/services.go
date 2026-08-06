package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"mcpd/internal/mcp"
)

// GetServicesToolDefinition returns the MCP Tool definition for systemd services management.
func GetServicesToolDefinition() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_services",
		Description: "Manage systemd services (start, stop, restart, status, enable, disable) using systemctl.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"start", "stop", "restart", "status", "enable", "disable"},
					"description": "The state transition or status query to perform on the service.",
				},
				"service": map[string]any{
					"type":        "string",
					"description": "The name of the service (e.g., 'nginx', 'ssh', 'docker').",
				},
			},
			Required: []string{"action", "service"},
		},
	}
}

// ServicesHandler manages service status using systemctl.
func ServicesHandler(ctx context.Context, arguments json.RawMessage) (mcp.CallToolResult, error) {
	var args struct {
		Action  string `json:"action"`
		Service string `json:"service"`
	}

	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("invalid arguments: %w", err)
	}

	service := strings.TrimSpace(args.Service)
	if service == "" {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "Error: The 'service' name cannot be empty.",
				},
			},
			IsError: true,
		}, nil
	}

	// Verify that systemctl exists in path
	if _, err := exec.LookPath("systemctl"); err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "Error: 'systemctl' command not found. This host does not appear to use systemd.",
				},
			},
			IsError: true,
		}, nil
	}

	slog.Info("Running systemctl command", "action", args.Action, "service", service)
	cmd := exec.CommandContext(ctx, "systemctl", args.Action, service)
	out, err := cmd.CombinedOutput()

	resultText := string(out)

	if err != nil {
		// systemctl status returns non-zero code for inactive services:
		// 0: program is running or service is OK
		// 3: program is not running (service stopped)
		// 4: program or service not found
		// Let's treat standard status/inactive exit codes as success so the user can inspect output.
		if args.Action == "status" {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Service Status Report:\n\n%s", resultText),
					},
				},
				IsError: false,
			}, nil
		}

		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Failed to execute 'systemctl %s %s': %v\nOutput:\n%s", args.Action, service, err, resultText),
				},
			},
			IsError: true,
		}, nil
	}

	// For actions other than status, provide a nice confirmation
	msg := fmt.Sprintf("Successfully executed 'systemctl %s %s'.", args.Action, service)
	if len(resultText) > 0 {
		msg += fmt.Sprintf("\nOutput:\n%s", resultText)
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: msg,
			},
		},
		IsError: false,
	}, nil
}
