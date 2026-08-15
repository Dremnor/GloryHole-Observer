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

### The host already runs Apache or nginx

Another site on the same host owns ports 80 and 443, so Caddy cannot bind them.
It fails at container start rather than at run time, which shows up as a proxy
stuck in `Created` with **no logs at all** and a site that does not answer:

```
gloryhole-observer-proxy-1   caddy:2-alpine   ...   Created
```

`docker compose up -d proxy` prints the real reason (`failed to bind host port
0.0.0.0:80/tcp: address already in use`), and `ss -tlnp | grep -E ':(80|443) '`
names the process holding them.

Put the map behind that web server instead:

```sh
docker compose -f docker-compose.yml -f docker-compose.behind-proxy.yml up -d --build
```

That publishes the map on `127.0.0.1:8080` and leaves Caddy out. Make it the
default for this checkout, so a later plain `docker compose up -d` cannot
quietly put Caddy back and stop publishing the port:

```sh
echo 'COMPOSE_FILE=docker-compose.yml:docker-compose.behind-proxy.yml' > .env
```

Then use `deploy/apache.conf` or `deploy/nginx.conf` as the virtual host:

```sh
a2enmod proxy proxy_http headers ssl
cp deploy/apache.conf /etc/apache2/sites-available/gloryhole-observer.conf
sed -i 's/map\.example\.com/your.domain/' /etc/apache2/sites-available/gloryhole-observer.conf
a2ensite gloryhole-observer && systemctl reload apache2
certbot --apache -d your.domain
```

`deploy/apache.conf` is plain HTTP on purpose: certbot copies it into a TLS
virtual host of its own and turns this one into a redirect. Shipping a TLS
block up front would name a certificate that does not exist yet, which Apache
refuses to load — and certbot cannot repair it, because it runs
`apache2ctl configtest` before doing anything. Reload and check the site over
`http://` first, then run certbot.

If Apache already serves that hostname from another virtual host, disable it
(`a2dissite <name>`) or the two will fight over the same `ServerName` — and
certbot may well extend the *other* one, leaving TLS pointed somewhere else.

`apache2ctl -S | grep your.domain` is the check. Exactly one entry per port,
both naming this site. A missing `port 443` entry does not fail loudly: Apache
answers with whichever virtual host is first for that port, so the domain
serves an unrelated site rather than an error.

Already have the certificate and would rather write the TLS virtual host than
let certbot rewrite things? Use `deploy/apache-ssl.conf` in place of
`deploy/apache.conf` — same proxy settings, TLS block filled in.

Both configs get two details right that fail quietly otherwise: the live tile
feed is Server-Sent Events, so it must not be buffered (`flushpackets=on` in
Apache, `proxy_buffering off` in nginx) or the map silently stops updating; and
tile uploads run to 100MB against nginx's 1MB default body limit.

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

## Marker icons

Markers are drawn from images addressed by the path the client reports, such as
`gfx/terobjs/mm/thingwall`. Clients gain markers for newly added game objects
before this server ships the matching image; until then those markers show a
placeholder on the map and log the missing file to the browser console.

**Admin portal → Manage icons** lists every image markers refer to, which ones
have no icon, and how many markers use each. Uploading one there stores it with
the map data rather than in the frontend build, so it takes effect immediately,
survives rebuilds, and takes precedence over a built-in image of the same name.
Removing an upload falls back to the built-in image, or to the placeholder if
there is none.

To edit an icon rather than replace it outright, download the current one from
the same page — per key, or all of them as a zip laid out by key — change it,
and upload it back against that key. The download serves whichever image is in
use, so it works for built-in icons as well as previous uploads.

PNG only, up to 1 MiB and 512 px per side. Uploads are decoded and re-encoded
server-side, so a file that merely claims to be a PNG is rejected. Markers draw
at 18 px, so larger images only help on high-density displays.

To ship an icon permanently instead, put it under `frontend/public/` at the same
path and rebuild.

## When something does not show up on the map

The server logs what it could not read, so start there:

```sh
docker compose logs -f map
```

| Log line | What it means |
| --- | --- |
| `markerUpdate from "x": 3 of 40 markers unreadable` | The client sent a field in a shape this server cannot decode. The rest of the batch was stored; the line quotes the first offending marker. |
| `positionUpdate from "x": 1 of 4 characters unreadable` | Same, for character positions. |
| `character "N" is on grid G, which this server does not have` | Nobody has uploaded that ground yet, so there is nowhere to draw them. |
| `skipping marker "N" with invalid grid id` | The grid ID was not something that can be used as a file name. |

Nothing in the log and still nothing on the map? Then it is a matter of what
each account is allowed to see:

- **No markers at all** — the account needs the `markers` role.
- **No players** — the account needs `point`. Characters also carry the
  visibility group of whoever uploaded them, so an account in no `g1`–`g5`
  group sees nothing from an uploader that is in one. The map says which of
  these applies next to the relevant switch.
- **One marker type missing while others are there** — the type has never
  reached the server. **Admin portal → Manage icons** lists every image key in
  the database with a count, which is the quickest way to confirm it.

Each of these lines is written at most once a minute per uploading account, so
a client repeating a bad field cannot fill the disk.

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
through the proxy — either unpublished (`docker-compose.yml`) or bound to
loopback (`docker-compose.behind-proxy.yml`). Turning it on with the port
exposed lets anyone forge the header and evade the login limit. The address is
read from the **last** `X-Forwarded-For` entry, the one the proxy appended
itself, so a client sending a header of its own cannot claim to be someone
else.

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

Leaflet, its marker images, the interface font and the icon font are part of the
build, so the map page loads nothing from a third party. The login and admin
pages, which are Go templates rather than part of the Vue app, still fetch
Materialize and the Material Icons font from a CDN.

## Notes on upgrading an existing install

- The container now runs as UID 10001 instead of root. A named volume picks
  this up automatically; if you bind-mount a host directory, run
  `chown -R 10001:10001 /srv/hnh-map` once.
- Destructive endpoints (wipe, delete user, rebuild zooms, wipe tile, hide
  marker, set coordinates, generate token) now require POST. Any bookmarks or
  scripts calling them over GET will get `405`.
- Changing a password now requires the current one.
- Marker and position uploads are decoded one entry at a time. A batch that used
  to be discarded whole because of one odd value now stores everything else, so
  marker types that never appeared may start turning up on their own.
