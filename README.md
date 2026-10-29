# Sverige Nu

**Live at [sverigenu.se](https://sverigenu.se/)**

Open data about Sweden in one place: weather, police events, population,
income, crime, health, schools, elections, the Riksdag, energy, traffic and more.
Everything comes from the open APIs of Swedish government agencies and is
refreshed automatically.

## Features

- **20+ topic pages** — weather (SMHI), police events (Polisen), population and
  income (SCB), crime (BRÅ), public health, schools, courts, elections, Riksdag
  votes, electricity prices, wind power, air quality, traffic and municipal tax
- **County profiles** — rankings and summaries for each of Sweden's 21 counties (län)
- **"Hidden connections"** — correlations between datasets across counties
- **JSON API** — all data is available under `/api/v1/...`
- **Light and self-hosted** — a single Go binary, PostgreSQL, and plain HTML/JS
  with no frontend build step

See [DATA_SOURCES.md](DATA_SOURCES.md) for the full list of sources and their endpoints.

## Tech stack

| Part     | Technology                                         |
|----------|----------------------------------------------------|
| Backend  | Go (chi router, pgx)                               |
| Database | PostgreSQL 16                                      |
| Frontend | Static HTML, CSS and vanilla JavaScript            |
| Serving  | nginx (static files + reverse proxy to the API)    |
| Run      | Docker Compose                                     |

## Quick start

Requirements: [Docker](https://docs.docker.com/get-docker/) with Compose.

```bash
git clone https://github.com/Hamed0406/swedennu.git
cd swedennu
cp .env.example .env      # then edit .env (see below)
docker compose up -d --build
```

Open **http://localhost:8091**. The first data sync starts automatically and
takes a few minutes. The database schema is created on startup.

## Configuration

All settings live in `.env`:

| Variable               | Description                                                         | Default |
|------------------------|---------------------------------------------------------------------|---------|
| `CONTACT_EMAIL`        | Your email, sent in the User-Agent (required by polisen.se's terms) | —       |
| `SYNC_TOKEN`           | Bearer token for `POST /api/v1/sync` and admin stats. Generate with `openssl rand -hex 32` | empty (unprotected) |
| `TRAFIKVERKET_API_KEY` | Free key from [api.trafikverket.se](https://api.trafikverket.se); traffic data is skipped without it | empty |
| `SYNC_INTERVAL`        | How often to fetch data (Go duration, keep ≥ `15m`)                 | `15m`   |
| `FRONTEND_PORT`        | Host port for the website                                           | `8091`  |
| `PORT`                 | Backend port                                                        | `3000`  |
| `BASE_URL`             | Public site URL, used in `sitemap.xml`                              | from request |
| `POSTGRES_PASSWORD`    | Database password — **change it** (and in `DATABASE_URL`)           | `swedennu` |
| `DEBUG`                | `true` for debug logging                                            | `false` |

## API

A few examples:

```bash
curl localhost:8091/api/v1/weather
curl localhost:8091/api/v1/policeevents
curl localhost:8091/api/v1/lan/rankings
curl localhost:8091/api/v1/sync/status

# Trigger a sync manually
curl -X POST -H "Authorization: Bearer $SYNC_TOKEN" localhost:8091/api/v1/sync
```

All routes are listed in [`backend/internal/api/server.go`](backend/internal/api/server.go).

## Development

Run the backend tests:

```bash
cd backend
go test ./...
```

Run the frontend tests (Playwright; needs the stack running on port 8091):

```bash
pip install pytest playwright && playwright install chromium
cd tests
pytest -m "not slow"
```

## Project structure

```
backend/
  cmd/server/        entry point: config, sync loop, HTTP server
  internal/api/      HTTP routes and handlers
  internal/fetcher/  one file per data source (SMHI, SCB, Polisen, ...)
  internal/store/    database reads and writes
  internal/db/       connection and schema
  internal/geo/      county and municipality boundaries
frontend/            static pages, one HTML + JS file per topic
tests/               Playwright end-to-end tests
nginx.conf           serves frontend/ and proxies /api to the backend
```

## Contributing

Issues and pull requests are welcome. Ideas for new data sources are listed
under "Not Started / Planned" in [DATA_SOURCES.md](DATA_SOURCES.md).

## License

[MIT](LICENSE). The data belongs to its respective sources. Please follow each
agency's terms of use.
