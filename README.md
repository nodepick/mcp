# nmcpd (Node VM Management MCP Server)

`nmcpd` is a lightweight Model Context Protocol (MCP) server written in Golang. It acts as a standard, secure entry point for AI agents (such as Claude, Gemini, or other host-level clients) to programmatically manage and configure Linux operating systems running inside virtual machines.

It is built using the official [Model Context Protocol Go SDK](https://github.com/modelcontextprotocol/go-sdk), ensuring compliance with the latest MCP specification protocol versions (including the `2026-07-28` protocol iteration).

---

## 🚀 Key Features

* **Official SDK Compliance**: Built directly on top of `github.com/modelcontextprotocol/go-sdk/mcp`, natively exposing standardized transports, session life-cycles, and tool capability validation.
* **Default HTTPS/TLS**: Serves over HTTPS by default. Automatically generates and runs a transient self-signed ECDSA certificate in memory at startup if custom TLS certs are not provided—allowing immediate, secure-by-default, zero-configuration network deployment.
* **Token Authentication & Middleware**: Protects endpoints using API keys. Support is built-in for key validation via `X-API-Key` headers, `Authorization: Bearer <key>` headers, or fallback URL query parameters.
* **Preservation of Unix Permissions**: Automatically reads, stores, and restores original file permissions when writing or patching files in `/etc`, maintaining system security compliance.
* **Smart Service Handling**: Non-zero exits from systemctl queries (like stopped service states) are reported as standard text outputs rather than tool invocation failures.

---

## 🛠️ Expose MCP Tools

`nmcpd` registers and exposes the following tools:

### Package Management

#### `manage_packages`
* **Description**: Manages OS packages using standard package managers.
* **Behavior**: Auto-detects the system's package manager (`apt-get`, `dnf`, `yum`, or `pacman`) or lets you manually override.
* **Arguments**:
  * `action` (string, enum: `"install"`, `"remove"`, `"update"`, `"upgrade"`) - The package action to perform.
  * `packages` (array of strings) - The list of packages (required for `"install"` and `"remove"`).
  * `package_manager` (string, optional, enum: `"auto"`, `"apt-get"`, `"dnf"`, `"yum"`, `"pacman"`) - Manual package manager selection.

### User Management

#### `manage_users`
* **Description**: Adds or deletes system users and assigns access levels.
* **Behavior**: Creates home directories, sets login shells, assigns secondary groups, and securely sets plain passwords by piping to `chpasswd` (preventing cmdline credential leaks).
* **Arguments**:
  * `action` (string, enum: `"add"`, `"delete"`) - Action to perform.
  * `username` (string) - Name of the user.
  * `password` (string, optional) - Login password (only for `"add"`).
  * `groups` (array of strings, optional) - Secondary groups to join (only for `"add"`).
  * `shell` (string, optional) - Default login shell (defaults to `/bin/bash`).

### Service Management

#### `manage_services`
* **Description**: State transition and status query for services via `systemctl`.
* **Arguments**:
  * `action` (string, enum: `"start"`, `"stop"`, `"restart"`, `"status"`, `"enable"`, `"disable"`) - Command action.
  * `service` (string) - Service name (e.g. `"nginx"`, `"sshd"`).

### Command Execution

#### `run_command`
* **Description**: Executes a Linux command or shell script on the VM.
* **Arguments**:
  * `command` (string, required) - The command string to execute.
  * `cwd` (string, optional) - Working directory for the command.
  * `timeout_seconds` (integer, optional) - Execution timeout in seconds (defaults to `60`).

### File & Configuration Management

#### `file_list`
* **Description**: Lists files in a directory path showing sizes and entry types.
* **Arguments**:
  * `path` (string) - Absolute path to directory.

#### `file_read`
* **Description**: Reads configuration and system text files safely. Safety capped at 2MB to prevent memory/context overflow.
* **Arguments**:
  * `path` (string) - Absolute path to file.

#### `file_write`
* **Description**: Overwrites complete configuration files. Preserves original permissions on existing files and supports optional `.bak` backups.
* **Arguments**:
  * `path` (string) - Absolute path to file.
  * `content` (string) - Text content.
  * `create_backup` (boolean, optional) - Create backup before overwriting.

#### `file_patch`
* **Description**: Performs targeted find-and-replace patches within configuration files. Saves bandwidth and preserves file states.
* **Arguments**:
  * `path` (string) - Absolute path to file.
  * `patches` (array of objects) - List of replacement operations, each with `target`, `replacement`, and optional `allow_multiple` boolean.

---

## 🛠️ Build and Installation

### Compile the Binary
Prerequisite: Go 1.25 or newer.

To build the static production binary, run:
```bash
go build -o nmcpd ./cmd/nmcpd
```

### Install in VM
Copy the generated `nmcpd` binary to the virtual machine (for example, to `/usr/local/bin/nmcpd`):
```bash
scp nmcpd user@vm-ip:/usr/local/bin/nmcpd
```

Ensure the binary has executable permissions inside the VM:
```bash
chmod +x /usr/local/bin/nmcpd
```

---

## 🖥️ Running & Using nmcpd

`nmcpd` has two execution modes:
1. **Background Daemon Mode (Default)**: Best for running permanently inside the VM (e.g., at boot time). It automatically forks itself into the background, boots the secure HTTPS server, and redirects logs to `/var/log/nmcpd.log` (falling back to `/tmp/nmcpd.log` if permissions are denied).
2. **Foreground Mode (`-f` / `--foreground`)**: Best for debugging or direct client pipe launching. Defaults to Stdio transport, but can run the HTTPS server in the foreground using `-t http`.

### Command Line Flags

* `-f`, `--foreground`: Runs the server in the foreground instead of daemonizing.
* `-t`, `--transport` (`stdio`, `http`, or `sse`): Selects the MCP transport mechanism (defaults to `stdio` in foreground, `http` in daemon mode).
* `-p`, `--port` (default: `8000`): Port to bind the server to.
* `-h`, `--host` (default: `0.0.0.0` in daemon, `127.0.0.1` in foreground): Bind address for the server.
* `--log` (default: `/var/log/nmcpd.log`): Path to output log files when daemonized.
* `--api-key` (optional): Set a token/API key requirement to protect HTTP/SSE endpoints.
* `--api-key-file` (default: `/usr/local/etc/nmcpd.conf`): Path to file containing the API key. Reads automatically if the file exists.
* `--tls-cert` (optional): Path to custom TLS certificate file in PEM format. If not provided (along with `--tls-key`), a transient self-signed ECDSA certificate will be generated and used to run HTTPS.
* `--tls-key` (optional): Path to custom TLS private key file in PEM format.

---

### 1. Stdio Integration over SSH (Claude Desktop / Local Agents)
You can configure your local Claude Desktop to run `nmcpd` inside the VM via SSH over Stdio. To prevent it from detaching and running as a daemon, **you must pass the `-f` (foreground) flag**. Since the binary runs system commands (`apt-get`, `useradd`, `systemctl`), it must run with root privileges inside the VM.

Add the following to your local `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "vm-manager": {
      "command": "ssh",
      "args": [
        "-t",
        "root@vm-ip-or-hostname",
        "/usr/local/bin/nmcpd",
        "-f"
      ]
    }
  }
}
```
> **Note**: Ensure passwordless SSH keys are configured for `root` on your VM, so the connection establishes instantly without interactive prompts.

### 2. Streamable HTTP Daemon Integration (HTTPS and Auth)
To run `nmcpd` as a background daemon at VM boot, simply execute the binary. It will self-daemonize, write logs to the background log file, and listen for incoming HTTPS connections:

```bash
# Start as a daemon loading API key securely from the default file path (/usr/local/etc/nmcpd.conf)
/usr/local/bin/nmcpd

# Alternatively, specify a custom key file path
/usr/local/bin/nmcpd --api-key-file "/etc/my-custom-key.conf"
```

Logs are written in structured JSON. You can customize the log detail using the `LOG_LEVEL` environment variable:
```bash
LOG_LEVEL=DEBUG /usr/local/bin/nmcpd
```

#### Running under systemd
If running `nmcpd` as a `systemd` service, it is highly recommended to use `Type=simple` and run the binary in the foreground. Since `systemd` tracks the process lifecycle and manages logging, running in daemon mode will cause systemd to think the service has exited (as the parent process exits immediately after launching the daemon child), leading to constant restarts.

Use the following configuration for your systemd service file (typically at `/etc/systemd/system/nmcpd.service`):

```ini
[Unit]
Description=nodepick.ai MCP Daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/sbin/nmcpd --foreground --transport http --host 0.0.0.0
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```
