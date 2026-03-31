# sap-mcp-config

Shared Go + Python configuration models for MCP servers that connect to SAP systems.

Both [mcp-server-abap](https://github.com/Hochfrequenz/mcp-server-abap) (Go) and [sapwebgui.mcp](https://github.com/Hochfrequenz/sapwebgui.mcp) (Python) connect to the same SAP systems with the same credentials. This package provides a single canonical configuration format so that both servers read from the same config file.

## Configuration Format

```json
{
  "default_system": "dev",
  "systems": {
    "dev": {
      "host": "https://your-sap-system:44300",
      "client": "100",
      "user": "YOUR_USER",
      "password": "YOUR_PASSWORD",
      "language": "DE",
      "tls_skip_verify": false
    },
    "prod": {
      "host": "https://prod-sap:44300",
      "client": "200",
      "user": "PROD_USER",
      "password": "PROD_PASSWORD",
      "language": "EN"
    }
  }
}
```

Default config file location: `~/.config/sap-mcp/systems.json`

Override via environment variable: `SAP_CONFIG_FILE=/path/to/config.json`

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `host` | string | yes | SAP system base URL |
| `client` | string | no | SAP client/mandant (e.g. `"100"`) |
| `user` | string | conditional | SAP username (omit for OAuth2) |
| `password` | string | conditional | SAP password (omit for OAuth2) |
| `language` | string | no | Login language (`"DE"`, `"EN"`, default `"EN"`) |
| `tls_skip_verify` | bool | no | Skip TLS certificate verification (default `false`) |
| `oauth2_client_id` | string | no | OAuth2 client ID for token-based auth |

Either both `user` and `password` must be set, or neither (for OAuth2).

## Usage

### Go

```go
import sapmcpconfig "github.com/Hochfrequenz/sap-mcp-config"

cfg, err := sapmcpconfig.Load("~/.config/sap-mcp/systems.json")
dev := cfg.Systems["dev"]
fmt.Println(dev.Host, dev.Client, dev.User)
```

### Python

```python
from sap_mcp_config import load

cfg = load("~/.config/sap-mcp/systems.json")
dev = cfg.systems["dev"]
print(dev.host, dev.client, dev.user)
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

## License

MIT
