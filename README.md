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

## Inviting people

Admins can create invite links under Settings > Users > Invite. Each link can be single-use or
multi-use, expires after a chosen time, and can either let people in automatically or hold them
for manual approval. Pending accounts cannot sign in until approved.

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

### Verifying release artifacts

Release images and binaries carry SLSA provenance and keyless Sigstore signatures produced by the release workflow.

Verify the image provenance and signature:

```sh
gh attestation verify oci://ghcr.io/quad4-software/quad4-id:<tag> -R Quad4-Software/Quad4-ID
cosign verify ghcr.io/quad4-software/quad4-id:<tag> \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/Quad4-Software/Quad4-ID/\.github/workflows/.*\.yml@refs/(heads/master|tags/v.*)$'
```

Verify a downloaded binary or checksums.txt, both through its provenance attestation and its signature bundle:

```sh
gh attestation verify quad4-id_linux_amd64 -R Quad4-Software/Quad4-ID
cosign verify-blob quad4-id_linux_amd64 \
  --bundle quad4-id_linux_amd64.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/Quad4-Software/Quad4-ID/\.github/workflows/release\.yml@refs/tags/v.*$'
```

## License

BSD 2-Clause. See LICENSE.
