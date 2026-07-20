ARG PLAYWRIGHT_BASE_IMAGE=m.daocloud.io/mcr.microsoft.com/playwright:v1.58.2-noble
FROM ${PLAYWRIGHT_BASE_IMAGE}

WORKDIR /app

RUN corepack enable && corepack prepare pnpm@10.28.2 --activate

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY services/crawler-worker/package.json services/crawler-worker/package.json

RUN pnpm install --filter @multica/crawler-worker --prod --frozen-lockfile

COPY services/crawler-worker/src services/crawler-worker/src

WORKDIR /app/services/crawler-worker

ENV NODE_ENV=production
EXPOSE 19516

CMD ["xvfb-run", "-a", "node", "src/index.mjs"]
