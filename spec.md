# RSS Auto-Print Server Specification

## 1. Project Overview

A Go-based web application that aggregates RSS feeds and automatically (or manually) prints new articles to local network printers. The application is designed to be easily self-hosted via Docker on home server environments like TrueNAS Scale.

## 2. Tech Stack

* **Backend:** Go (Standard library for HTTP routing and handlers).
* **Frontend:** HTML rendered via Go html/template, progressively enhanced with HTMX for Ajax interactions.
* **Styling:** Single style.css file compiled via the Tailwind CSS CLI.
* **Asset Compilation:** Native Go embed package to bundle HTML templates and static assets directly into a single, standalone binary.
* **Database:** SQLite3 via the xorm.io/xorm ORM utilizing a pure-Go, CGO-free driver (modernc.org/sqlite).
* **Session Management:** [github.com/gorilla/sessions](https://github.com/gorilla/sessions) for secure session cookies.

---

## 3. Core Features

### 3.1 Feed Management (Polling)

* Users can add authenticated and unauthenticated RSS feed URLs.
* The server utilizes [github.com/mmcdole/gofeed](https://github.com/mmcdole/gofeed) to parse standard RSS and Atom feeds.
* A background worker routine polls active feeds at a user-configurable interval (e.g., every 15–30 minutes).
* The database tracks parsed article GUIDs to prevent duplicate processing.

### 3.2 Printer Discovery & Document Execution

* The server must operate completely independently of host OS print spoolers (No CUPS dependency).
* **Discovery:** Utilize [github.com/grandcat/zeroconf](https://github.com/grandcat/zeroconf) to scan the local network for IPP printers via mDNS (_ipp._tcp). Discovered printers must be displayed in the UI for easy configuration.
* **Document Generation:** To maintain a minimal Docker footprint without heavy headless browser dependencies (like Chromium or wkhtmltopdf), the server will parse the RSS article content and use a pure-Go PDF library (e.g., [github.com/signintech/gopdf](https://github.com/signintech/gopdf)) to generate a standard printable page layout directly in memory.
* **Execution:** Utilize [github.com/OpenPrinting/goipp](https://github.com/OpenPrinting/goipp) to construct an IPP Print-Job operation message. The raw PDF byte stream is passed with the document-format attribute explicitly set to application/pdf.
* **Settings:** The UI must allow users to configure global or feed-specific print settings, passing these as standard IPP attributes:
* Copies (copies)
* Color vs. Black/White (sides / print-color-mode)
* Duplex / Double-sided printing (sides)
* Font size (Handled programmatically during the PDF layout generation step).



### 3.3 Job State & Automation Lifecycle

* The application tracks print requests using a PrintJobs table with the following states: Pending, Sent, Failed.
* When the background worker finds a new article:
* If "Auto-Print" is enabled for that feed, it automatically generates the PDF, creates a database entry, and attempts delivery to the configured printer.
* If a print job fails due to temporary network issues, the worker will retry delivery up to 3 times with an exponential backoff before marking the job status as Failed.
* Failed or completed states must be visible on an administrator dashboard queue.



### 3.4 User Management

* The application supports multiple users on a single server instance.
* Users must be authenticated to add feeds, view the dashboard, and manage printers.

---

## 4. Architectural & Database Constraints

### 4.1 Concurrency & SQLite Performance

* Because the background worker routine writes to the database at the same time a user may be interacting with the HTMX frontend, the application must be protected against database is locked errors.
* The application must initialize the SQLite connection string with Write-Ahead Logging (_journal_mode=WAL) enabled and configure appropriate busy timeouts.
* Max open connections on the ORM engine must be capped appropriately if required by the driver implementation to guarantee thread-safe writes.

### 4.2 Project Build Script

* The agent must provide a simple shell script or Makefile that handles the front-end compilation workflow before compiling the Go application:
1. Invokes the Tailwind CSS standalone CLI to parse templates, tree-shake unused classes, and output a minified production asset to ./static/css/style.css.
2. Executes go build to package the embedded assets and code into a single executable binary.



---

## 5. Deployment Constraints

* The provided Dockerfile must be a minimal, multi-stage build running on a lightweight base image (such as gcr.io/distroless/static or a minimal alpine image). It must not rely on installing system print drivers, fonts, or external rendering engines.
* Documentation must instruct users to run the container using host networking (network_mode: host in Docker Compose) to ensure mDNS auto-discovery functions correctly across the local subnet.