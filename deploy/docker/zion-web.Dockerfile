# syntax=docker/dockerfile:1.8
ARG NODE_VERSION=22.22.0
ARG ALPINE_VERSION=3.22

FROM node:${NODE_VERSION}-alpine${ALPINE_VERSION} AS deps
WORKDIR /app
COPY apps/web/package.json apps/web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

FROM node:${NODE_VERSION}-alpine${ALPINE_VERSION} AS builder
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /app/node_modules ./node_modules
COPY apps/web/ ./
ARG NEXT_PUBLIC_ZION_API_URL=http://127.0.0.1:42001
ENV NEXT_PUBLIC_ZION_API_URL=${NEXT_PUBLIC_ZION_API_URL}
RUN npm run build

FROM node:${NODE_VERSION}-alpine${ALPINE_VERSION}
ARG VERSION=v0.1.0-alpha.1
ARG REVISION=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="ZION Web" \
      org.opencontainers.image.description="Replaceable ZION local-node web client" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/kokora3/zion" \
      org.opencontainers.image.licenses="Apache-2.0"
ENV NODE_ENV=production \
    NEXT_TELEMETRY_DISABLED=1 \
    HOSTNAME=0.0.0.0 \
    PORT=3000
WORKDIR /app
COPY --from=builder --chown=node:node /app/.next/standalone ./
COPY --from=builder --chown=node:node /app/.next/static ./.next/static
USER node
EXPOSE 3000/tcp
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=12 \
  CMD node -e "fetch('http://127.0.0.1:3000/').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"
CMD ["node", "server.js"]
