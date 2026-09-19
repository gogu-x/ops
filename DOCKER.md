# Docker deployment

The Compose stack includes the ops backend, Vue frontend, and MongoDB.
The backend uses the sibling `tree` module through `go.mod`, so Compose builds
the backend from the `gotree` parent directory automatically.

```bash
cp .env.example .env
# Change all passwords and OPS_JWT_SECRET in .env.
docker compose up -d --build
```

Open `http://127.0.0.1:8090` by default.

```bash
docker compose ps
docker compose logs -f ops-backend
docker compose down
```

MongoDB data is stored in the `mongo-data` volume. `docker compose down` keeps
it; `docker compose down -v` explicitly removes it.
