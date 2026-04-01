# sap-mcp-config

![Go Tests](https://github.com/Hochfrequenz/sap-mcp-config/workflows/Go%20Tests/badge.svg)
![Go Coverage](https://github.com/Hochfrequenz/sap-mcp-config/workflows/Go%20Coverage/badge.svg)
![Go Lint](https://github.com/Hochfrequenz/sap-mcp-config/workflows/Go%20Lint/badge.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/Hochfrequenz/sap-mcp-config.svg)](https://pkg.go.dev/github.com/Hochfrequenz/sap-mcp-config)
[![Go Report Card](https://goreportcard.com/badge/github.com/Hochfrequenz/sap-mcp-config)](https://goreportcard.com/report/github.com/Hochfrequenz/sap-mcp-config)
![Python Tests](https://github.com/Hochfrequenz/sap-mcp-config/workflows/Python%20Tests/badge.svg)
![Python Coverage](https://github.com/Hochfrequenz/sap-mcp-config/workflows/Python%20Coverage/badge.svg)
![Python Lint](https://github.com/Hochfrequenz/sap-mcp-config/workflows/Python%20Lint/badge.svg)
![Python Formatting](https://github.com/Hochfrequenz/sap-mcp-config/workflows/Python%20Formatting/badge.svg)
![Python Versions](https://img.shields.io/pypi/pyversions/sap-mcp-config.svg)
![PyPI](https://img.shields.io/pypi/v/sap-mcp-config)

**The standard way to manage SAP credentials for MCP servers.**

If you're building an MCP server that connects to SAP, use this package. It gives you validated, type-safe configuration in both Go and Python with a single shared config file. No more reinventing credential loading, no more inconsistent formats between projects.

Both [mcp-server-abap](https://github.com/Hochfrequenz/mcp-server-abap) (Go) and [sapwebgui.mcp](https://github.com/Hochfrequenz/sapwebgui.mcp) (Python) use this package.

The default config path (`~/.config/sap-mcp/systems.json`) follows the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/).

## Features

- **One config file, two languages** — Go and Python read the same config, guaranteed by shared test fixtures
- **JSON and YAML** — use whichever format you prefer (auto-detected by file extension)
- **Validates eagerly** — reports _all_ errors at once so users fix everything in one pass
- **Passwords never leak in print/log output** — masked in `str()`/`repr()`/`fmt.Println()`/`fmt.Sprintf("%+v")` (Go: `fmt.Formatter`; Python: `pydantic.SecretStr`)
- **Immutable after loading** — frozen Pydantic models in Python; in Go, use the returned structs as read-only
- **`.env` file support** — `SAP_CONFIG_FILE` can be set in a `.env` file
- **Easy to extend** — subclass `SAPSystem` in Python or embed the struct in Go to add project-specific fields

## MCP JSON Configuration

This package integrates naturally with the [MCP JSON configuration standard](https://modelcontextprotocol.io/docs/concepts/transports). Point your MCP server to the shared config file via the `SAP_CONFIG_FILE` environment variable:

```json
{
  "mcpServers": {
    "sap-abap": {
      "command": "mcp-server-abap",
      "env": {
        "SAP_CONFIG_FILE": "/home/user/.config/sap-mcp/systems.json"
      }
    },
    "sap-webgui": {
      "command": "sapwebgui-mcp",
      "env": {
        "SAP_CONFIG_FILE": "/home/user/.config/sap-mcp/systems.json"
      }
    }
  }
}
```

Both servers read the **same config file** with the **same credentials** — no duplication.

## Installation

### Go

```bash
go get github.com/Hochfrequenz/sap-mcp-config
```

### Python

```bash
pip install sap-mcp-config
```

## Configuration File

Create `~/.config/sap-mcp/systems.json` (or `systems.yaml` — format is auto-detected by extension):

### JSON

```json
{
  "default_system": "dev",
  "systems": {
    "dev": {
      "connection_name": "DEV - ERP Development",
      "host": "https://your-sap-system:44300",
      "client": "100",
      "user": "YOUR_USER",
      "password": "YOUR_PASSWORD",
      "language": "DE"
    },
    "prod": {
      "connection_name": "PROD - ERP Production",
      "host": "https://prod-sap:44300",
      "client": "200",
      "user": "PROD_USER",
      "password": "PROD_PASSWORD",
      "language": "EN"
    }
  }
}
```

### YAML

```yaml
default_system: dev
systems:
  dev:
    connection_name: "DEV - ERP Development"
    host: "https://your-sap-system:44300"
    client: "100"
    user: YOUR_USER
    password: YOUR_PASSWORD
    language: DE
  prod:
    connection_name: "PROD - ERP Production"
    host: "https://prod-sap:44300"
    client: "200"
    user: PROD_USER
    password: PROD_PASSWORD
    language: EN
```

Override the config file location via the `SAP_CONFIG_FILE` environment variable:

```bash
export SAP_CONFIG_FILE=/path/to/my/config.yaml
```

This also works from a `.env` file in the current directory.

### Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `connection_name` | string | no | `""` | SAP Logon connection entry name — must match the **bold description text** shown in the SAP Logon pad, not the System ID (SID). Used by desktop backends (e.g. SAP GUI) to open the correct connection. |
| `host` | string | yes | | SAP system base URL (must start with `http://` or `https://`) |
| `client` | string | no | `""` | SAP client/mandant, must be exactly 3 digits (e.g. `"100"`) |
| `user` | string | conditional | `""` | SAP username (omit for OAuth2) |
| `password` | string | conditional | `""` | SAP password (omit for OAuth2) |
| `language` | string | no | `"EN"` | Login language: `"DE"` or `"EN"` |
| `tls_skip_verify` | bool | no | `false` | Skip TLS certificate verification |
| `oauth2_client_id` | string | no | `""` | OAuth2 client ID for token-based auth |

**Important:** The dictionary key (e.g. `"dev"`, `"prod"`) is only used to look up systems in the config. It has no connection to the SAP system itself. The `connection_name` field is what identifies the SAP Logon entry for desktop backends. This distinction allows you to configure multiple entries for the same SAP system with different clients or credentials:

```json
{
  "default_system": "dev-100",
  "systems": {
    "dev-100": {
      "connection_name": "DEV - ERP Development",
      "host": "https://dev-sap.example.com:44300",
      "client": "100",
      "user": "DEV_USER",
      "password": "DEV_PASSWORD"
    },
    "dev-200": {
      "connection_name": "DEV - ERP Development",
      "host": "https://dev-sap.example.com:44300",
      "client": "200",
      "user": "QA_USER",
      "password": "QA_PASSWORD"
    }
  }
}
```

Both entries share the same `connection_name` (same SAP Logon entry) but use different clients and credentials.

**Validation rules:**
- At least one system must be defined
- `default_system` must reference an existing system key
- `host` is required and must start with `http://` or `https://`
- `client`, if set, must be exactly 3 digits
- `language`, if set, must be `"DE"` or `"EN"`
- Either both `user` and `password` must be set, or neither (for OAuth2)

## Usage

### Go

```go
package main

import (
    "fmt"
    "os"

    sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"
)

func main() {
    // Load from SAP_CONFIG_FILE env var or ~/.config/sap-mcp/systems.json
    cfg, err := sapmcpconfig.LoadDefault()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Configuration error:\n%s\n", err)
        os.Exit(1)
    }

    // Access the default system
    dev := cfg.GetDefault()
    fmt.Println(dev.Host, dev.Client, dev.User)

    // Access a specific system
    prod := cfg.Systems["prod"]
    fmt.Println(prod.Host, prod.Client, prod.User)

    // Password is safe to print — it won't leak
    fmt.Println(dev) // Output: SAPSystem{ConnectionName:DEV - ERP Development Host:https://... Client:100 User:DEV_USER Password:*** Language:DE}
}
```

### Python

```python
import sys

from pydantic import ValidationError

from sap_mcp_config import load_default

try:
    # Load from SAP_CONFIG_FILE env var or ~/.config/sap-mcp/systems.json
    cfg = load_default()
except FileNotFoundError:
    print("Config file not found. Create ~/.config/sap-mcp/systems.json")
    sys.exit(1)
except ValidationError as e:
    print(f"Configuration error:\n{e}")
    sys.exit(1)

# Access the default system
dev = cfg.get_default()
print(dev.host, dev.client, dev.user)

# Access a specific system
prod = cfg.systems["prod"]
print(prod.host, prod.client, prod.user)

# Password is a SecretStr — it won't leak in print/logs
print(dev)  # password=SecretStr('**********')

# Access the actual password value when needed
password = dev.password.get_secret_value()
```

### Error Messages

Both implementations validate eagerly and return **all** errors at once. A misconfigured file like this:

```json
{
  "default_system": "missing",
  "systems": {
    "dev": { "host": "ftp://wrong", "client": "1", "user": "u" }
  }
}
```

...will report all problems in a single error:

```
invalid configuration:
  - default_system "missing" not found in systems
  - system "dev": host must start with http:// or https://, got "ftp://wrong"
  - system "dev": client must be a 3-digit string (e.g. "100"), got "1"
  - system "dev": must have both user and password, or neither (for OAuth2)
```

This way users fix everything in one pass instead of playing whack-a-mole.

## Extending the Configuration

Both languages make it easy to add project-specific fields on top of the shared base.

### Python

Subclass `SAPSystem` to add your own fields:

```python
from pydantic import ConfigDict
from sap_mcp_config import SAPSystem

class MySAPSystem(SAPSystem):
    model_config = ConfigDict()  # unfreeze for subclass

    custom_timeout: int = 30
```

### Go

Embed `SAPSystem` in your own struct:

```go
type MySAPSystem struct {
    sapmcpconfig.SAPSystem
    CustomTimeout int `json:"custom_timeout"`
}
```

## Development

### Go

```bash
go test ./...
```

### Python

```bash
pip install -e ".[tests]"
PYTHONPATH=src pytest unittests
```

Or via tox:

```bash
pip install tox
tox -e tests       # unit tests
tox -e linting     # pylint
tox -e type_check  # mypy --strict
tox -e coverage    # coverage with 80% minimum
```

## License

MIT
