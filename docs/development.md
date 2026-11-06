# Local development

Run the full stack in Docker with `just start`, or run only PostgreSQL and RustFS in Docker and start Go and Vite on your machine. Use one workflow at a time because they share ports.

## Prerequisites

- Docker with Docker Compose and [`just`](https://just.systems/).
- For the manual workflow, install Go, Node.js 24, and pnpm 12.3.4.

## Set up local environment files

From the repository root, copy the examples on a fresh checkout. If you already have these files, edit them in place.

```bash
cp .env-example .env
cp server/.env-example server/.env
cp client-web/.env-example client-web/.env
```

Use a local `SESSION_SECRET` and keep the database name consistent. The root example sets `POSTGRES_DB=lensamity`, but its `DATABASE_URL` points to `test`. The examples below use `lensamity` and the example's local database credentials. Use your actual local database name if a volume has already been initialized.

## Option 1 — Run everything in Docker

In the root `.env`, use the service names for connections from the Go container and the browser-facing address for signed storage URLs.

```dotenv
POSTGRES_DB=lensamity
DATABASE_URL=postgres://postgres:postgres@db:5432/lensamity
S3_INTERNAL_ENDPOINT=http://rustfs:9000
S3_PUBLIC_ENDPOINT=http://s3.localhost:9000
```

Leave `VITE_BASE_API_URL` empty to route API requests through Nginx. The build recipe also reads `server/.env`, so keep that file in place even when running everything in Docker.

Start the full stack from the repository root.

```bash
just start
```

This builds the images, starts the services, and runs the database migrations. Open [http://localhost:8080](http://localhost:8080) to use the app. The RustFS console is at [http://storage.localhost:9001](http://storage.localhost:9001).

Stop the stack with `just down`. Run `just logs api --tail=100` to see recent API logs.

## Option 2 — Run Go and Vite locally

Set `POSTGRES_DB=lensamity` in the root `.env`. Configure `server/.env` to connect to the ports published on your machine.

```dotenv
DATABASE_URL=postgres://postgres:postgres@localhost:5432/lensamity
S3_INTERNAL_ENDPOINT=http://localhost:9000
S3_PUBLIC_ENDPOINT=http://localhost:9000
```

Set `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, and `S3_REGION` in `server/.env` to the matching `RUSTFS_ACCESS_KEY`, `RUSTFS_SECRET_KEY`, and `RUSTFS_REGION` from the root `.env`. Use the same `S3_BUCKET` in both files.

From the repository root, start PostgreSQL and RustFS.

```bash
docker compose -f docker-compose.dev.yml run --rm -d --name lensamity-dev-db -p 5432:5432 db
docker compose -f docker-compose.dev.yml run --rm -d --name lensamity-dev-rustfs -p 9000:9000 -p 9001:9001 rustfs
```

Wait for PostgreSQL to be healthy in `docker ps`. From the repository root, run the migrations and start the API in the same terminal.

```bash
cd server
just migrate && just serv
```

In a second terminal, start Vite from the repository root.

```bash
cd client-web
pnpm install --frozen-lockfile
pnpm run dev
```

Open the URL printed by Vite, usually [http://localhost:5173](http://localhost:5173). Leave `VITE_BASE_API_URL` unset or empty so Vite proxies `/api` and `/health` to the Go server on port 8080. The RustFS console is at [http://localhost:9001](http://localhost:9001).

Press Ctrl+C in both app terminals, then stop the containers.

```bash
docker stop lensamity-dev-db lensamity-dev-rustfs
```

## Create the storage bucket

Before the first upload, sign in to the RustFS console for your workflow using the `RUSTFS_ACCESS_KEY` and `RUSTFS_SECRET_KEY` from the root `.env`. Create a private bucket with the name set in `S3_BUCKET` (`lensamity-dev` in the examples).

## Reset local data

Use this command from the repository root only when you want to remove the local PostgreSQL volume and orphan containers. It deletes the local database. RustFS files are stored separately under `infra/rustfs/`.

```bash
docker compose -f docker-compose.dev.yml down --remove-orphans -v
```

Start your chosen workflow again to recreate the database and apply migrations.
