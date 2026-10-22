# syntax=docker/dockerfile:1

FROM node:22-alpine AS deps
WORKDIR /src
COPY package.json package-lock.json* ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /src
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=deps /src/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
RUN addgroup -S -g 10001 web && adduser -S -u 10001 -G web web
COPY --from=build --chown=web:web /src/.next/standalone ./
COPY --from=build --chown=web:web /src/.next/static ./.next/static
USER web
EXPOSE 3000
CMD ["node", "server.js"]
