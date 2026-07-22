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
ENV DISPLAY=:99
EXPOSE 19516

CMD ["sh", "-lc", "if [ -e /tmp/.X99-lock ]; then unlink /tmp/.X99-lock; fi; if [ -e /tmp/.X11-unix/X99 ]; then unlink /tmp/.X11-unix/X99; fi; Xvfb :99 -screen 0 1280x1024x24 -nolisten tcp >/tmp/xvfb.log 2>&1 & for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do [ -S /tmp/.X11-unix/X99 ] && break; sleep 0.1; done; exec node src/index.mjs"]
