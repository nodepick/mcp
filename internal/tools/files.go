package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"mcpd/internal/mcp"
)

// GetFilesToolDefinitions returns MCP definitions for all file tools.
func GetFilesToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "file_list",
			Description: "List files and directories inside a specific directory path.",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute path to the directory to list.",
					},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "file_read",
			Description: "Read the contents of a text configuration or system file.",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute path to the file to read.",
					},
				},
				Required: []string{"path"},
			},
		},
		{
			Name:        "file_write",
			Description: "Write or overwrite the complete contents of a file (retains original permissions if file exists).",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute path to the file to write.",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "The text content to write to the file.",
					},
					"create_backup": map[string]any{
						"type":        "boolean",
						"description": "If true, copies the existing file to <path>.bak before overwriting.",
					},
				},
				Required: []string{"path", "content"},
			},
		},
		{
			Name:        "file_patch",
			Description: "Perform target-specific find-and-replace edits within a configuration file.",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Absolute path to the file to patch.",
					},
					"patches": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"target": map[string]any{
									"type":        "string",
									"description": "The exact string to find in the file.",
								},
								"replacement": map[string]any{
									"type":        "string",
									"description": "The string to replace the target with.",
								},
								"allow_multiple": map[string]any{
									"type":        "boolean",
									"description": "Allow multiple replacements if the target string occurs more than once. Defaults to false.",
								},
							},
							"required": []string{"target", "replacement"},
						},
						"description": "List of find-and-replace patch operations.",
					},
				},
				Required: []string{"path", "patches"},
			},
		},
	}
}

// FileListHandler lists files in a directory.
func FileListHandler(ctx context.Context, arguments json.RawMessage) (mcp.CallToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("invalid arguments: %w", err)
	}

	absPath := filepath.Clean(args.Path)
	slog.Info("Listing directory", "path", absPath)

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Failed to list directory '%s': %v", absPath, err),
				},
			},
			IsError: true,
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Directory listing for: %s\n\n", absPath))
	sb.WriteString(fmt.Sprintf("%-20s %-12s %s\n", "Type", "Size (bytes)", "Name"))
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			sb.WriteString(fmt.Sprintf("%-20s %-12s %s (error reading info)\n", "unknown", "-", entry.Name()))
			continue
		}

		typeStr := "file"
		if entry.IsDir() {
			typeStr = "dir"
		} else if info.Mode()&os.ModeSymlink != 0 {
			typeStr = "symlink"
		}

		sizeStr := fmt.Sprintf("%d", info.Size())
		if entry.IsDir() {
			sizeStr = "-"
		}

		sb.WriteString(fmt.Sprintf("%-20s %-12s %s\n", typeStr, sizeStr, entry.Name()))
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: sb.String(),
			},
		},
		IsError: false,
	}, nil
}

// FileReadHandler reads a text file's contents with safety limits.
func FileReadHandler(ctx context.Context, arguments json.RawMessage) (mcp.CallToolResult, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("invalid arguments: %w", err)
	}

	absPath := filepath.Clean(args.Path)
	slog.Info("Reading file", "path", absPath)

	info, err := os.Stat(absPath)
	if err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("File not found or inaccessible: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	if info.IsDir() {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Error: Path '%s' is a directory. Use file_list to list directories.", absPath),
				},
			},
			IsError: true,
		}, nil
	}

	// Safety check: Limit reading to 2MB to avoid bloated contexts and stdio buffer overflows.
	const maxReadSize = 2 * 1024 * 1024 // 2MB
	if info.Size() > maxReadSize {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Error: File size (%d bytes) exceeds the safety limit of %d bytes (2MB).", info.Size(), maxReadSize),
				},
			},
			IsError: true,
		}, nil
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Failed to read file: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: string(content),
			},
		},
		IsError: false,
	}, nil
}

// FileWriteHandler writes or overwrites file contents.
func FileWriteHandler(ctx context.Context, arguments json.RawMessage) (mcp.CallToolResult, error) {
	var args struct {
		Path         string `json:"path"`
		Content      string `json:"content"`
		CreateBackup bool   `json:"create_backup"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("invalid arguments: %w", err)
	}

	absPath := filepath.Clean(args.Path)
	slog.Info("Writing file", "path", absPath, "create_backup", args.CreateBackup)

	// Keep track of old file permissions
	var perm fs.FileMode = 0644
	oldInfo, statErr := os.Stat(absPath)
	fileExists := statErr == nil

	if fileExists {
		perm = oldInfo.Mode().Perm()

		if args.CreateBackup {
			backupPath := absPath + ".bak"
			slog.Info("Creating file backup", "from", absPath, "to", backupPath)
			oldContent, readErr := os.ReadFile(absPath)
			if readErr != nil {
				return mcp.CallToolResult{
					Content: []mcp.Content{
						{
							Type: "text",
							Text: fmt.Sprintf("Failed to read old file for backup: %v", readErr),
						},
					},
					IsError: true,
				}, nil
			}

			if writeErr := os.WriteFile(backupPath, oldContent, perm); writeErr != nil {
				return mcp.CallToolResult{
					Content: []mcp.Content{
						{
							Type: "text",
							Text: fmt.Sprintf("Failed to write backup file '%s': %v", backupPath, writeErr),
						},
					},
					IsError: true,
				}, nil
			}
		}
	} else {
		// Ensure parent directories exist
		parentDir := filepath.Dir(absPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Failed to create parent directories: %v", err),
					},
				},
				IsError: true,
			}, nil
		}
	}

	// Write new contents with preserved or default permissions
	if err := os.WriteFile(absPath, []byte(args.Content), perm); err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Failed to write file '%s': %v", absPath, err),
				},
			},
			IsError: true,
		}, nil
	}

	msg := fmt.Sprintf("Successfully wrote file '%s'.", absPath)
	if fileExists {
		msg += fmt.Sprintf(" Kept original file permissions (%04o).", perm)
		if args.CreateBackup {
			msg += " Created backup file."
		}
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

// PatchOp represents a single find-and-replace operation.
type PatchOp struct {
	Target        string `json:"target"`
	Replacement   string `json:"replacement"`
	AllowMultiple bool   `json:"allow_multiple"`
}

// FilePatchHandler performs targeted find-and-replace replacements.
func FilePatchHandler(ctx context.Context, arguments json.RawMessage) (mcp.CallToolResult, error) {
	var args struct {
		Path    string    `json:"path"`
		Patches []PatchOp `json:"patches"`
	}
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.CallToolResult{IsError: true}, fmt.Errorf("invalid arguments: %w", err)
	}

	absPath := filepath.Clean(args.Path)
	slog.Info("Patching file", "path", absPath, "num_patches", len(args.Patches))

	info, err := os.Stat(absPath)
	if err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("File not found or inaccessible: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	perm := info.Mode().Perm()
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Failed to read file for patching: %v", err),
				},
			},
			IsError: true,
		}, nil
	}

	content := string(contentBytes)

	for i, patch := range args.Patches {
		if patch.Target == "" {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Error in patch #%d: 'target' string cannot be empty.", i+1),
					},
				},
				IsError: true,
			}, nil
		}

		count := strings.Count(content, patch.Target)
		if count == 0 {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Error in patch #%d: Target string not found in the file.\nTarget: %q", i+1, patch.Target),
					},
				},
				IsError: true,
			}, nil
		}

		if count > 1 && !patch.AllowMultiple {
			return mcp.CallToolResult{
				Content: []mcp.Content{
					{
						Type: "text",
						Text: fmt.Sprintf("Error in patch #%d: Target string occurred %d times in the file, but allow_multiple was set to false. Be more specific in matching the text.\nTarget: %q", i+1, count, patch.Target),
					},
				},
				IsError: true,
			}, nil
		}

		content = strings.ReplaceAll(content, patch.Target, patch.Replacement)
	}

	// Write back the modified content
	if err := os.WriteFile(absPath, []byte(content), perm); err != nil {
		return mcp.CallToolResult{
			Content: []mcp.Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Failed to write patched contents back to '%s': %v", absPath, err),
				},
			},
			IsError: true,
		}, nil
	}

	return mcp.CallToolResult{
		Content: []mcp.Content{
			{
				Type: "text",
				Text: fmt.Sprintf("Successfully applied %d patch(es) to '%s' (preserved permissions: %04o).", len(args.Patches), absPath, perm),
			},
		},
		IsError: false,
	}, nil
}
