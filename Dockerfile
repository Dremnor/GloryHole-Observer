FROM golang:1.25-alpine AS gobuilder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Static binary so the runtime stage needs no libc beyond alpine's.
RUN CGO_ENABLED=0 go build -o /out/hnh-map .

FROM node:22-alpine AS frontendbuilder

WORKDIR /frontend

# npm ci installs exactly what package-lock.json pins, so an image built today
# resolves the same dependency tree as one built months from now.
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --legacy-peer-deps

COPY frontend/ ./
RUN npm run build

FROM alpine:3

WORKDIR /hnh-map

COPY --from=gobuilder /out/hnh-map ./
COPY --from=frontendbuilder /frontend/dist ./frontend
COPY templates ./templates
COPY public ./public

# grids.db and the tile images live here and must be writable.
VOLUME /map

EXPOSE 8080
ENTRYPOINT ["/hnh-map/hnh-map"]
CMD ["-grids=/map"]
