# Stage 1: Build React App
FROM node:24-alpine AS web-build

WORKDIR /app

RUN corepack enable pnpm

COPY client-web/package.json .
COPY client-web/pnpm-lock.yaml .
RUN pnpm install --frozen-lockfile

COPY client-web/. .
RUN pnpm run build

# Stage 2: Build Go API
FROM golang:1.25-alpine AS api-build

WORKDIR /src

COPY server/go.mod .
COPY server/go.sum .
COPY server/ .
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o api ./cmd/api

# Stage 3: Runtime image
FROM nginx:1.31-alpine AS production

COPY --from=web-build /app/dist /usr/share/nginx/html
COPY --from=api-build /src/api /usr/local/bin/api

COPY infra/nginx/prod.nginx.conf /etc/nginx/conf.d/default.conf
COPY infra/nginx/start-production.sh /usr/local/bin/start
RUN chmod +x /usr/local/bin/start

EXPOSE 10000

CMD ["/usr/local/bin/start"]
