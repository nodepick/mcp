package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"

	"nmcpd/internal/mcp"
)

// GetUserManageToolDefinition returns the MCP Tool definition for user management.
func GetUserManageToolDefinition() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_users",
		Description: "Add or delete Linux OS users, set passwords, and assign groups.",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]any{
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"add", "delete"},
					"description": "The user management action to perform.",
				},
				"username": map[string]any{
					"type":        "string",
					"description": "The login name of the user.",
				},
				"password": map[string]any{
					"type":        "string",
					"description": "Plaintext password for the user (only used when action is 'add').",
				},
				"groups": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "string",
					},
					"description": "List of secondary groups to assign the user to (only used when action is 'add').",
				},
				"shell": map[string]any{
					"type":        "string",
					"description": "The default login shell for the user. Defaults to /bin/bash.",
				},
			},
			Required: []string{"action", "username"},
		},
	}
}

// UserManageHandler executes user management commands.
func UserManageHandler(ctx context.Context, arguments json.RawMessage) (mcp.CallToolResult, error) {
	var args struct {
		Action   string   `json:"action"`
		Username string   `json:"username"`
		Password string   `json:"password"`
		Groups   []string `json:"groups"`
		Shell    string   `json:"shell"`
	}

	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("invalid arguments: %w", err)
	}

	username := strings.TrimSpace(args.Username)
	if username == "" {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: "Error: The 'username' field cannot be empty.",
				},
			},
			IsError: true,
		}, nil
	}

	if args.Action == "add" {
		// 1. Create the user
		shell := args.Shell
		if shell == "" {
			shell = "/bin/bash"
		}

		useraddArgs := []string{"-m", "-s", shell, username}
		slog.Info("Running useradd command", "username", username, "shell", shell)
		cmd := exec.CommandContext(ctx, "useradd", useraddArgs...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Failed to create user '%s': %v\nOutput: %s", username, err, string(out)),
					},
				},
				IsError: true,
			}, nil
		}

		// 2. Set the password if provided
		if args.Password != "" {
			slog.Info("Setting user password", "username", username)
			chpasswdCmd := exec.CommandContext(ctx, "chpasswd")
			stdin, err := chpasswdCmd.StdinPipe()
			if err != nil {
				return mcp.CallToolResult{IsError: true}, fmt.Errorf("failed to open chpasswd stdin: %w", err)
			}

			// Run chpasswd and pipe the password
			if err := chpasswdCmd.Start(); err != nil {
				stdin.Close()
				return mcp.CallToolResult{
					Content: []mcp.Content{
						{
							Type: "text",
							Text: fmt.Sprintf("User '%s' was created, but failed to start chpasswd: %v", username, err),
						},
					},
					IsError: true,
				}, nil
			}

			// Write credentials to stdin
			credentials := fmt.Sprintf("%s:%s\n", username, args.Password)
			_, writeErr := io.WriteString(stdin, credentials)
			stdin.Close()

			if writeErr != nil {
				chpasswdCmd.Wait()
				return mcp.CallToolResult{
					Content: []mcp.Content{
						{
							Type: "text",
							Text: fmt.Sprintf("User '%s' was created, but failed writing to chpasswd: %v", username, writeErr),
						},
					},
					IsError: true,
				}, nil
			}

			if err := chpasswdCmd.Wait(); err != nil {
				return mcp.CallToolResult{
					Content: []mcp.Content{
						{
							Type: "text",
							Text: fmt.Sprintf("User '%s' was created, but failed setting password: %v", username, err),
						},
					},
					IsError: true,
				}, nil
			}
		}

		// 3. Add to secondary groups if provided
		if len(args.Groups) > 0 {
			groupsCSV := strings.Join(args.Groups, ",")
			slog.Info("Assigning user to groups", "username", username, "groups", groupsCSV)
			usermodCmd := exec.CommandContext(ctx, "usermod", "-aG", groupsCSV, username)
			usermodOut, err := usermodCmd.CombinedOutput()
			if err != nil {
				return mcp.CallToolResult{
					Content: []mcp.Content{
						{
							Type: "text",
							Text: fmt.Sprintf("User '%s' was created, but failed to assign groups (%s): %v\nOutput: %s", username, groupsCSV, err, string(usermodOut)),
						},
					},
					IsError: true,
				}, nil
			}
		}

		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Successfully created user '%s' (shell: %s, secondary groups: %v).", username, shell, args.Groups),
				},
			},
			IsError: false,
		}, nil

	} else if args.Action == "delete" {
		// Delete the user and purge home directories (-r)
		slog.Info("Running userdel command", "username", username)
		cmd := exec.CommandContext(ctx, "userdel", "-r", username)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Failed to delete user '%s': %v\nOutput: %s", username, err, string(out)),
					},
				},
				IsError: true,
			}, nil
		}

		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Successfully deleted user '%s' and purged home directory.", username),
				},
			},
			IsError: false,
		}, nil
	}

	return mcp.CallToolResult{IsError: true}, fmt.Errorf("unsupported action: %s", args.Action)
}
