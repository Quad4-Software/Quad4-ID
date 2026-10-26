# Quad4 ID

Quad4 ID is a passkey-only OpenID Connect and OAuth 2.0 provider that lets users sign in to your applications without passwords.

Quad4 ID is based on [Pocket ID](https://github.com/pocket-id/pocket-id). The code is derived from it under the BSD 2-Clause license.

## Install

The recommended way to run Quad4 ID is Docker Compose:

```yaml
services:
  quad4-id:
    image: ghcr.io/quad4-software/quad4-id:latest
    restart: unless-stopped
    env_file: .env
    ports:
      - 1411:1411
    user: "65532:65532"
    read_only: true
    tmpfs:
      - /tmp
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    volumes:
      - "./data:/app/data"
```

The container runs rootless as uid 65532. If you bind-mount a ./data directory, it must be writable by that uid:

```sh
mkdir -p data && chown 65532:65532 data
```

Copy `.env.example` to `.env` and set at least `APP_URL` and `ENCRYPTION_KEY`.

The images use zstd-compressed layers, so pulling them requires Docker Engine 23 or newer, or any containerd 1.5+ based runtime.

## Build from source

Requires Go and Node.js with pnpm (via Vite+ vp).

```sh
pnpm install --frozen-lockfile
pnpm -C frontend run build
cd backend
CGO_ENABLED=0 go build -o quad4-id ./cmd
```

Run the backend tests with `go test -tags=exclude_frontend,unit ./...` from `backend/`.

## Releases

To cut a release, bump `.version` and the `version` field in `frontend/package.json` to `X.Y.Z`, commit, tag `vX.Y.Z`, and push the tag. The release workflow builds binaries and the container image and generates release notes from the commit history without usernames or issue numbers.

## License

BSD 2-Clause. See LICENSE.
