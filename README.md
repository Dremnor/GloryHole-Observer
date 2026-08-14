# GloryHole Observer

Automapper server for Haven & Hearth. Clients such as Purus Pasta upload map
tiles, character positions and markers; the server stitches them into one map
and serves it as a Leaflet web map.

Forked from [Cediner/hnh-map-vuetify](https://github.com/Cediner/hnh-map-vuetify),
itself a fork of `andyleap/hnh-auto-mapper`. Client-compatible with
[APXEOLOG/hnh-auto-mapper-server](https://github.com/APXEOLOG/hnh-auto-mapper-server).

A Go binary serves everything: the login and admin pages, the client upload API
under `/client/<token>/`, and the Vue frontend under `/map/`. State lives in an
embedded bbolt database (`grids.db`) plus tile images on disk — there is no
separate database server to run.

## Deploying on a VPS

```sh
git clone https://github.com/Dremnor/GloryHole-Observer
cd GloryHole-Observer
$EDITOR deploy/Caddyfile        # put your domain in place of map.example.com
docker compose up -d --build
```

Caddy obtains and renews the TLS certificate itself. Point the domain's DNS at
the VPS before starting, or the certificate request will fail.

To host a second service on the same VPS, add another block to
`deploy/Caddyfile` — Caddy routes by hostname, so both share ports 80 and 443.

Prefer an existing nginx? Use `deploy/nginx.conf` instead, drop the `proxy`
service from `docker-compose.yml`, and publish the map container on
`127.0.0.1:8080`.

### No domain yet? Run it on a port

```sh
docker compose -f docker-compose.yml -f docker-compose.direct.yml up -d --build
```

This skips Caddy and publishes the server on `127.0.0.1:8080`. Reach it through
an SSH tunnel from your own machine:

```sh
ssh -L 8080:127.0.0.1:8080 root@your-server
```

then open `http://localhost:8080`. Create the first user (below), and only then
change the `ports` entry in `docker-compose.direct.yml` to `"8080:8080"` and
re-run the command to publish it.

There is no TLS this way, so logins cross the network in clear text. Move to
the Caddy setup once a domain points at the host.

### Set up the first user before exposing the server

Until the first user exists, **anyone can log in as `admin` / `admin`**. On a
public address that is a race against port scanners, so close it first:

1. Start the stack with the firewall still blocking 80 and 443, or bind the
   proxy to `127.0.0.1` and reach it over an SSH tunnel.
2. Log in as `admin` / `admin`, open the admin portal, add your real user with
   every role you need — at minimum `admin`. Creating it removes the temporary
   admin and logs you out.
3. Log back in as the new user, then open the firewall.

Then set the token prefix (e.g. `https://map.example.com`) in the admin portal
so generated client tokens are copy-pasteable, and add users for everyone else.

### Roles

| Role | Grants |
| --- | --- |
| `map` | View the map |
| `markers` | See markers |
| `point` | See characters |
| `upload` | Send tiles, characters and markers via a client token |
| `writer` | Wipe tiles, rewrite coordinates, hide markers |
| `admin` | Manage users and server settings, wipe, merge, export |
| `g1`–`g5` | Visibility groups (see below) |

A character is shown only to users sharing one of its groups, and its groups
come from whoever uploaded it. **A character with no group is visible to
everyone**, so an upload account without a `g1`–`g5` role bypasses the group
system entirely. The admin user form warns about this, and the server logs it
when such an account is saved.

### If the build fails on DNS

On a host whose resolver list includes an IPv6 nameserver, the build can fail
almost immediately with:

```
go mod download
go: ...: dial udp [2a01:...]:53: connect: network is unreachable
```

Docker's default bridge network is IPv4-only, so a container handed an IPv6
nameserver cannot reach it. Give containers explicit resolvers instead:

```sh
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'EOF'
{
  "dns": ["1.1.1.1", "8.8.8.8"]
}
EOF
systemctl restart docker
```

Verify against a build container rather than `docker run`, since that is where
it failed:

```sh
printf 'FROM alpine\nRUN cat /etc/resolv.conf && nslookup proxy.golang.org\n' \
    | docker build --no-cache --progress=plain -
```

If name resolution now works but the build still cannot reach the network,
containers have no egress at all — check `docker run --rm alpine ping -c2 1.1.1.1`
and `sysctl net.ipv4.ip_forward`, which is a firewall or forwarding problem
rather than DNS.

## Configuration

Flags, all optional:

| Flag | Default | Meaning |
| --- | --- | --- |
| `-grids` | `grids` | Directory holding `grids.db` and the tile images |
| `-port` | `8080` | Listen port (or set `HNHMAP_PORT`) |
| `-merge-min-overlap` | `2` | Overlapping grids that must agree before two maps merge automatically |
| `-trust-proxy-headers` | `false` | Read the client address from `X-Forwarded-For` |
| `-login-max-failures` | `10` | Failed logins from one address before lockout (`0` disables) |
| `-login-window` | `15m` | How long a lockout lasts |

`-trust-proxy-headers` must only be enabled when the port is unreachable except
through the proxy, which is how `docker-compose.yml` is arranged. Turning it on
with the port exposed lets anyone forge the header and evade the login limit.

### Automatic map merging

When a client reports a grid window covering two known maps, the server folds
them together and rewrites the coordinates of every grid in the merged map.
There is no unmerge — recovery means wiping and re-scanning.

`-merge-min-overlap` is how much evidence that takes. At the default of 2, two
grids must agree on the same offset before a merge happens; a lone stray grid,
which a stale or desynced client easily produces, is refused and logged. Maps
whose own grids contradict each other are never merged, and a grid update whose
anchor map contradicts itself is rejected outright rather than written from
untrustworthy coordinates. Setting it to `1` restores the old behaviour of
merging on any overlap.

## Backups

Everything worth keeping is in the `mapdata` volume: `grids.db` and the tile
images. The admin portal offers two downloads — **Export** (grids and markers,
re-importable through Merge) and **Backup** (a consistent copy of the database
plus the base zoom tiles).

For an unattended copy, snapshot the volume while the stack is stopped, or copy
it out with a helper container:

```sh
docker run --rm -v gloryhole-observer_mapdata:/map -v "$PWD":/backup alpine \
    tar czf /backup/map-$(date +%F).tar.gz -C /map .
```

## Development

```sh
go build ./... && go test ./...          # server
cd frontend && npm ci && npm run build   # frontend into frontend/dist
./hnh-map -grids=./grids                 # serves the built frontend at /map/
```

The frontend dev server (`npm run serve`) mocks the API with miragejs, so it
runs without a backend.

## Notes on upgrading an existing install

- The container now runs as UID 10001 instead of root. A named volume picks
  this up automatically; if you bind-mount a host directory, run
  `chown -R 10001:10001 /srv/hnh-map` once.
- Destructive endpoints (wipe, delete user, rebuild zooms, wipe tile, hide
  marker, set coordinates, generate token) now require POST. Any bookmarks or
  scripts calling them over GET will get `405`.
- Changing a password now requires the current one.
