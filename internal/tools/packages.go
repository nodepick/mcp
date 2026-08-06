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

// DetectPackageManager attempts to find the system package manager.
func DetectPackageManager() (string, error) {
	managers := []string{"apt-get", "dnf", "yum", "pacman"}
	for _, m := range managers {
		if _, err := exec.LookPath(m); err == nil {
			return m, nil
		}
	}
	return "", fmt.Errorf("no supported package manager found (checked apt-get, dnf, yum, pacman)")
}

// GetPackageManageToolDefinition returns the MCP Tool definition for package management.
func GetPackageManageToolDefinition() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_packages",
		Description: "Manage system packages (install, remove, update/upgrade) on the host Linux OS.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"install", "remove", "update", "upgrade"},
					"description": "The package management action to perform.",
				},
				"packages": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "List of package names (required for install and remove actions).",
				},
				"package_manager": map[string]any{
					"type":        "string",
					"enum":        []string{"auto", "apt-get", "dnf", "yum", "pacman"},
					"description": "Manually select the package manager. Defaults to auto-detect.",
				},
			},
			Required: []string{"action"},
		},
	}
}

// PackageManageHandler executes package management operations.
func PackageManageHandler(ctx context.Context, arguments json.RawMessage) (mcp.CallToolResult, error) {
	var args struct {
		Action         string   `json:"action"`
		Packages       []string `json:"packages"`
		PackageManager string   `json:"package_manager"`
	}

	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("invalid arguments: %w", err)
	}

	// Resolve the package manager
	pm := args.PackageManager
	if pm == "" || pm == "auto" {
		var err error
		pm, err = DetectPackageManager()
		if err != nil {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Error: %v. You can specify a package manager manually if it's not in the default search path.", err),
					},
				},
				IsError: true,
			}, nil
		}
	}

	// Validate packages parameter for install and remove
	if (args.Action == "install" || args.Action == "remove") && len(args.Packages) == 0 {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "Error: The 'packages' field must contain at least one package name for 'install' or 'remove' actions.",
				},
			},
			IsError: true,
		}, nil
	}

	// Construct the command based on package manager and action
	var cmdName string
	var cmdArgs []string

	switch pm {
	case "apt-get":
		cmdName = "apt-get"
		switch args.Action {
		case "install":
			cmdArgs = append([]string{"install", "-y"}, args.Packages...)
		case "remove":
			cmdArgs = append([]string{"remove", "-y"}, args.Packages...)
		case "update":
			cmdArgs = []string{"update"}
		case "upgrade":
			cmdArgs = []string{"upgrade", "-y"}
		}

	case "dnf":
		cmdName = "dnf"
		switch args.Action {
		case "install":
			cmdArgs = append([]string{"install", "-y"}, args.Packages...)
		case "remove":
			cmdArgs = append([]string{"remove", "-y"}, args.Packages...)
		case "update":
			cmdArgs = []string{"check-update"} // Note: check-update returns exit code 100 if updates exist, let's handle that gracefully
		case "upgrade":
			cmdArgs = []string{"upgrade", "-y"}
		}

	case "yum":
		cmdName = "yum"
		switch args.Action {
		case "install":
			cmdArgs = append([]string{"install", "-y"}, args.Packages...)
		case "remove":
			cmdArgs = append([]string{"remove", "-y"}, args.Packages...)
		case "update":
			cmdArgs = []string{"check-update"}
		case "upgrade":
			cmdArgs = []string{"upgrade", "-y"}
		}

	case "pacman":
		cmdName = "pacman"
		switch args.Action {
		case "install":
			cmdArgs = append([]string{"-Sy", "--noconfirm"}, args.Packages...)
		case "remove":
			cmdArgs = append([]string{"-R", "--noconfirm"}, args.Packages...)
		case "update":
			cmdArgs = []string{"-Sy"}
		case "upgrade":
			cmdArgs = []string{"-Syu", "--noconfirm"}
		}

	default:
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("unsupported package manager: %s", pm)
	}

	slog.Info("Running package manager command", "command", cmdName, "args", strings.Join(cmdArgs, " "))

	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	out, err := cmd.CombinedOutput()

	// Special case: dnf / yum check-update returns exit code 100 if updates are available
	if err != nil && args.Action == "update" && (pm == "dnf" || pm == "yum") {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 100 {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Repository check completed. Updates are available!\n\n%s", string(out)),
					},
				},
				IsError: false,
			}, nil
		}
	}

	resultText := string(out)
	if err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Command failed with error: %v\n\nOutput:\n%s", err, resultText),
				},
			},
			IsError: true,
		}, nil
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: fmt.Sprintf("Successfully executed package action '%s' using %s.\n\nOutput:\n%s", args.Action, pm, resultText),
			},
		},
		IsError: false,
	}, nil
}
