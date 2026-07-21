FROM node:20-alpine AS deps
WORKDIR /src
COPY package.json package-lock.json ./
COPY forge/web/package.json ./forge/web/package.json
COPY packages ./packages
RUN npm ci

FROM node:20-alpine AS build
WORKDIR /src
ARG API_INTERNAL_URL=http://api:8080
ENV API_INTERNAL_URL=$API_INTERNAL_URL
ENV NEXT_TELEMETRY_DISABLED=1
ENV NODE_OPTIONS="--max-old-space-size=4096"
COPY --from=deps /src/node_modules ./node_modules
COPY . .
RUN npm --workspace @forge/web run build

FROM node:20-alpine AS runner

LABEL maintainer="GamePanel Team" \
      version="1.0.0" \
      description="Forge Web - GamePanel Next.js dashboard" \
      org.opencontainers.image.source="https://github.com/gamepanel/forge" \
      org.opencontainers.image.title="Forge Web" \
      org.opencontainers.image.description="GamePanel Next.js dashboard" \
      security.nonroot="true" \
      security.readonly-rootfs="true"

WORKDIR /app
ENV NODE_ENV=production
ENV HOSTNAME=0.0.0.0
ENV PORT=3000
ENV NEXT_TELEMETRY_DISABLED=1

COPY --from=build /src/forge/web/.next/standalone ./
COPY --from=build /src/forge/web/.next/static ./forge/web/.next/static

RUN addgroup -S appgroup && adduser -S appuser -G appgroup && \
    chown -R appuser:appgroup /app

USER appuser

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:3000/ || exit 1

CMD ["node", "forge/web/server.js"]
