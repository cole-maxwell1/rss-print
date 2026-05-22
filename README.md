# RSS Auto-Print Server

A self-hosted, Docker-ready Go web application that polls RSS feeds and automatically prints new articles to your local network IPP printers.

Built completely independent of CUPS, using a pure-Go tech stack.

## Tech Stack
- **Backend:** Go 1.22+ (net/http router)
- **Frontend:** HTMX, Tailwind CSS, Go `html/template`
- **Database:** Pure-Go SQLite (`modernc.org/sqlite`)
- **Printer Discovery:** mDNS / Bonjour (`zeroconf`)
- **PDF Generation:** Pure-Go PDF lib (`gopdf`)
- **Printing:** Raw IPP (`goipp`)

## Features
- Add RSS feeds with configurable polling intervals.
- Toggle "Auto-Print" for individual feeds.
- Automatically discovers local network IPP printers.
- Generates minimalist printable PDFs.

## Deployment

The provided `Dockerfile` builds a minimal Alpine container.

### Important: Host Networking
For the mDNS network printer discovery (`zeroconf`) to work correctly, the Docker container **must** be run using host networking mode.

### Docker Compose Example

```yaml
version: '3.8'

services:
  rss-print:
    build: .
    container_name: rss-print
    network_mode: "host" # REQUIRED for mDNS printer discovery
    volumes:
      - ./data:/app/data
    restart: unless-stopped
    environment:
      - PORT=8080
      - DB_PATH=/app/data/rss-print.db
```

### Initial Setup
On first run, the server will automatically create a default admin user:
- **Username:** `admin`
- **Password:** `admin`

Please change this password or add a new user and delete the admin immediately after logging in.

## Build from Source (Local)
Requires Go 1.22+.

```bash
make all
./rss-print
```
