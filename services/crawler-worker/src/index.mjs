import crypto from "node:crypto";
import http from "node:http";
import { Buffer } from "node:buffer";
import { pathToFileURL } from "node:url";
import { chromium } from "playwright";
import pg from "pg";
import {
  normalizeAppGrowingMaterialSearchParams,
  normalizeMaterialRules,
} from "./appgrowing-intent.mjs";

const { Pool } = pg;

const port = Number(process.env.CRAWLER_WORKER_PORT || process.env.PORT || 19516);
const host = process.env.HOST || "127.0.0.1";
const publicURL = (process.env.CRAWLER_WORKER_PUBLIC_URL || `http://${host}:${port}`).replace(/\/$/, "");
const apiURL = (process.env.MULTICA_API_URL || "http://127.0.0.1:18080").replace(/\/$/, "");
const databaseURL = process.env.DATABASE_URL || "";
const keyVersion = process.env.BROKER_STATE_KEY_VERSION || "v1";
const loginSessionTTLMS = Number(process.env.CRAWLER_WORKER_LOGIN_TTL_MS || 15 * 60 * 1000);
const remoteBrowserUI = process.env.CRAWLER_WORKER_REMOTE_UI === "true";
const sessionViewport = {
  width: positiveIntegerEnv("CRAWLER_WORKER_VIEWPORT_WIDTH", 1440),
  height: positiveIntegerEnv("CRAWLER_WORKER_VIEWPORT_HEIGHT", 1000),
};
const streamFrameIntervalMS = Math.max(
  100,
  Math.min(1000, positiveIntegerEnv("CRAWLER_WORKER_STREAM_FRAME_MS", 180)),
);

const builtInConnectors = {
  appgrowing: {
    id: "appgrowing",
    loginURL: "https://auth.youcloud.com/login?app_id=en_appgrowing&goto=https%3A%2F%2Fappgrowing-global.youcloud.com%2Fleaflet",
    probeURL: "https://appgrowing-global.youcloud.com/leaflet",
    graphQLURL: "https://api-appgrowing-global.youcloud.com/graphql",
    stateDomains: ["appgrowing-global.youcloud.com", "auth.youcloud.com", "youcloud.com"],
    loginURLPattern: /login|signin|passport|register/i,
    anonymousTextPatterns: [/登录/, /免费试用/, /该账号在其他设备登录/, /已被登出/, /登录重试/],
    sessionHeadlessDefault: false,
    crawlHeadlessDefault: false,
    capabilities: ["profile_verify", "material_search", "material_download", "page_extract"],
    authCheck: {
      url: "https://api-appgrowing-global.youcloud.com/graphql",
      userIDPath: ["data", "userinfo", "user_id"],
      payload: {
        operationName: "userinfo",
        query: `
          query userinfo {
            userinfo {
              user_id
              teamInfo {
                outer_id
                team_name
              }
              purchasePlanInfo {
                id
                expired
              }
            }
          }
        `,
        variables: {},
      },
    },
  },
};

const connectors = loadConnectorRegistry();

function loadConnectorRegistry() {
  const registry = { ...builtInConnectors };
  const raw = String(process.env.MULTICA_CREDENTIAL_CONNECTORS_JSON || "").trim();
  if (!raw) {
    return registry;
  }
  let manifests;
  try {
    manifests = JSON.parse(raw);
  } catch (error) {
    throw new Error(`MULTICA_CREDENTIAL_CONNECTORS_JSON is invalid JSON: ${error.message}`);
  }
  if (!Array.isArray(manifests)) {
    throw new Error("MULTICA_CREDENTIAL_CONNECTORS_JSON must be an array");
  }
  for (const manifest of manifests) {
    const connector = normalizeDeclarativeConnector(manifest);
    if (registry[connector.id]) {
      throw new Error(`connector ${connector.id} is already registered`);
    }
    registry[connector.id] = connector;
  }
  return registry;
}

function normalizeDeclarativeConnector(manifest) {
  if (!manifest || typeof manifest !== "object" || Array.isArray(manifest)) {
    throw new Error("declarative connector entries must be objects");
  }
  const id = String(manifest.id || "").trim().toLowerCase();
  if (!/^[a-z0-9][a-z0-9-]{0,62}$/.test(id)) {
    throw new Error(`invalid connector id ${id || "<empty>"}`);
  }
  const loginURL = connectorHTTPURL(manifest.login_url, `${id}.login_url`);
  const probeURL = connectorHTTPURL(manifest.probe_url || manifest.login_url, `${id}.probe_url`);
  const stateDomains = connectorStringArray(manifest.state_domains);
  if (stateDomains.length === 0) {
    stateDomains.push(new URL(loginURL).hostname);
  }
  const capabilities = connectorStringArray(manifest.capabilities);
  if (capabilities.length === 0) {
    throw new Error(`connector ${id} must declare capabilities`);
  }
  const auth = manifest.auth_check && typeof manifest.auth_check === "object"
    ? manifest.auth_check
    : null;
  return {
    id,
    displayName: String(manifest.display_name || id).trim(),
    loginURL,
    probeURL,
    graphQLURL: String(manifest.graphql_url || "").trim(),
    stateDomains,
    allowedTargetDomains: connectorStringArray(manifest.allowed_target_domains || stateDomains),
    capabilities,
    loginURLPattern: new RegExp(String(manifest.login_url_pattern || "login|signin|passport|register"), "i"),
    anonymousTextPatterns: connectorStringArray(manifest.anonymous_text_patterns).map((pattern) => new RegExp(pattern, "i")),
    sessionHeadlessDefault: manifest.session_headless_default !== false,
    crawlHeadlessDefault: manifest.crawl_headless_default !== false,
    authCheck: auth ? {
      url: connectorHTTPURL(auth.url, `${id}.auth_check.url`),
      userIDPath: connectorStringArray(auth.user_id_path),
      payload: auth.payload && typeof auth.payload === "object" ? auth.payload : {},
    } : null,
  };
}

function connectorHTTPURL(value, field) {
  let parsed;
  try {
    parsed = new URL(String(value || "").trim());
  } catch {
    throw new Error(`${field} must be a valid URL`);
  }
  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw new Error(`${field} must use HTTP or HTTPS`);
  }
  return parsed.toString();
}

function connectorStringArray(value) {
  if (!Array.isArray(value)) {
    return [];
  }
  return [...new Set(value.map((item) => String(item || "").trim()).filter(Boolean))];
}

function connectorForID(id) {
  const connector = connectors[String(id || "").trim()];
  if (!connector) {
    throw userError(`unknown credential connector: ${id || "<empty>"}`, 400);
  }
  return connector;
}

const activeSessions = new Map();
let pool;

function positiveIntegerEnv(name, fallback) {
  const value = Number(process.env[name]);
  return Number.isInteger(value) && value > 0 ? value : fallback;
}

const forbiddenKeyFragments = [
  "authorization",
  "bearer",
  "cookie",
  "localstorage",
  "password",
  "secret",
  "sessionstorage",
  "storagestate",
  "token",
];

function stateKey() {
  const raw = process.env.BROKER_STATE_KEY || process.env.MULTICA_BROKER_STATE_KEY;
  if (raw) {
    const key = Buffer.from(raw, "base64");
    if (key.length !== 32) {
      throw new Error("BROKER_STATE_KEY must be base64 for exactly 32 bytes");
    }
    return key;
  }
  if (process.env.APP_ENV === "production") {
    throw new Error("BROKER_STATE_KEY is required in production");
  }
  console.warn("BROKER_STATE_KEY not set; using deterministic local-dev key");
  return crypto.createHash("sha256").update("multica-local-credential-worker-dev-key").digest();
}

function sealState(state) {
  const plaintext = Buffer.from(JSON.stringify(state), "utf8");
  const nonce = crypto.randomBytes(12);
  const cipher = crypto.createCipheriv("aes-256-gcm", stateKey(), nonce);
  const encrypted = Buffer.concat([cipher.update(plaintext), cipher.final()]);
  const tag = cipher.getAuthTag();
  return Buffer.concat([nonce, encrypted, tag]);
}

function openState(sealed) {
  if (!Buffer.isBuffer(sealed)) {
    sealed = Buffer.from(sealed);
  }
  if (sealed.length < 12 + 16) {
    throw new Error("stored credential state is too short");
  }
  const nonce = sealed.subarray(0, 12);
  const tag = sealed.subarray(sealed.length - 16);
  const encrypted = sealed.subarray(12, sealed.length - 16);
  const decipher = crypto.createDecipheriv("aes-256-gcm", stateKey(), nonce);
  decipher.setAuthTag(tag);
  const plaintext = Buffer.concat([decipher.update(encrypted), decipher.final()]);
  return JSON.parse(plaintext.toString("utf8"));
}

function dbPool() {
  if (!databaseURL) {
    throw new Error("DATABASE_URL is required for credential crawl");
  }
  if (!pool) {
    pool = new Pool({ connectionString: databaseURL });
  }
  return pool;
}

async function readCredentialState(profileID) {
  const result = await dbPool().query(
    "SELECT ciphertext, key_version FROM credential_secret WHERE profile_id = $1",
    [profileID],
  );
  if (result.rowCount === 0) {
    throw new Error("credential secret not found");
  }
  if (result.rows[0].key_version === "local-stub") {
    throw new Error("profile was completed by the old local stub; bind a new profile with the Playwright worker");
  }
  return openState(result.rows[0].ciphertext);
}

function readJSON(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => {
      try {
        const raw = Buffer.concat(chunks).toString("utf8").trim();
        resolve(raw ? JSON.parse(raw) : {});
      } catch (err) {
        reject(err);
      }
    });
    req.on("error", reject);
  });
}

function writeJSON(res, status, body) {
  res.writeHead(status, {
    "content-type": "application/json; charset=utf-8",
    "cache-control": "no-store",
  });
  res.end(JSON.stringify(body));
}

function userError(message, statusCode = 409) {
  const err = new Error(message);
  err.statusCode = statusCode;
  return err;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function normalizeKey(key) {
  return String(key).toLowerCase().replace(/[\s_.-]/g, "");
}

function assertSafeParams(value, path = "$") {
  if (Array.isArray(value)) {
    value.forEach((item, index) => assertSafeParams(item, `${path}[${index}]`));
    return;
  }
  if (value && typeof value === "object") {
    for (const [key, child] of Object.entries(value)) {
      const normalized = normalizeKey(key);
      if (forbiddenKeyFragments.some((fragment) => normalized.includes(fragment))) {
        throw new Error(`unsafe params key: ${path}.${key}`);
      }
      assertSafeParams(child, `${path}.${key}`);
    }
  }
}

function parseSessionPath(pathname) {
  const match = pathname.match(/^\/sessions\/([^/]+)(?:\/([^/]+))?$/);
  if (!match) {
    return null;
  }
  return {
    token: decodeURIComponent(match[1]),
    action: match[2] || "",
  };
}

function getSession(token) {
  const session = activeSessions.get(token);
  if (!session) {
    return null;
  }
  if (Date.now() > session.expiresAt) {
    session.status = "expired";
  }
  return session;
}

function publicSession(session) {
  return {
    profile_id: session.profileID,
    connector_id: session.connectorID,
    status: session.status,
    browser_open: Boolean(session.browser),
    remote_ui: remoteBrowserUI,
    viewport: session.viewport || sessionViewport,
    login_url: session.loginURL,
    current_url: session.page ? session.page.url() : "",
    auth_check: session.authCheck || null,
    scroll: session.scrollState || null,
    auto_completing: Boolean(session.autoCompleting),
    auto_complete_reason: session.autoCompleteReason || "",
    error: session.status === "completed" ? "" : session.error || "",
    expires_at: new Date(session.expiresAt).toISOString(),
  };
}

async function sessionDetails(session) {
  if (session.page) {
    session.scrollState = await pageScrollState(session.page).catch(() => session.scrollState || null);
  }
  return publicSession(session);
}

async function pageScrollState(page) {
  return page.evaluate(() => {
    const canScroll = (element) => {
      if (!element || element === document) {
        return false;
      }
      const rect = element.getBoundingClientRect();
      const style = window.getComputedStyle(element);
      if (style.display === "none" || style.visibility === "hidden" || rect.width < 32 || rect.height < 32) {
        return false;
      }
      return element.scrollHeight - element.clientHeight > 1;
    };
    const candidates = Array.from(document.querySelectorAll("body, body *"))
      .filter(canScroll)
      .sort((left, right) => {
        const leftRange = left.scrollHeight - left.clientHeight;
        const rightRange = right.scrollHeight - right.clientHeight;
        return rightRange - leftRange;
      });
    let scrollingElement = candidates[0] || document.scrollingElement || document.documentElement || document.body;
    if (scrollingElement === document.body || scrollingElement === document.documentElement) {
      scrollingElement = document.scrollingElement || scrollingElement;
    }
    const viewportHeight = scrollingElement?.clientHeight || window.innerHeight || 0;
    const scrollHeight = scrollingElement?.scrollHeight || viewportHeight;
    const rawTop = scrollingElement?.scrollTop ?? window.scrollY ?? 0;
    const maxTop = Math.max(0, scrollHeight - viewportHeight);
    const scrollTop = Math.max(0, Math.min(maxTop, rawTop));
    return {
      scroll_top: scrollTop,
      max_top: maxTop,
      ratio: maxTop > 0 ? scrollTop / maxTop : 0,
      viewport_height: viewportHeight,
      scroll_height: scrollHeight,
      can_scroll: maxTop > 1,
    };
  });
}

function mousePointFromBody(page, session, body) {
  const viewport = page.viewportSize() || session.viewport || sessionViewport;
  return {
    x: clampFiniteNumber(body.x, 0, viewport.width, "x"),
    y: clampFiniteNumber(body.y, 0, viewport.height, "y"),
  };
}

function mouseButtonFromBody(body) {
  return body.button === "right" || body.button === "middle" ? body.button : "left";
}

async function enqueueSessionInput(session, body) {
  const previous = session.inputQueue || Promise.resolve();
  const next = previous
    .catch(() => {})
    .then(() => applySessionInput(session, body));
  session.inputQueue = next.catch(() => {});
  return next;
}

async function scrollPageAtPoint(page, body) {
  const viewport = page.viewportSize() || sessionViewport;
  const x = Number.isFinite(Number(body.x)) ? clampFiniteNumber(body.x, 0, viewport.width, "x") : viewport.width / 2;
  const y = Number.isFinite(Number(body.y)) ? clampFiniteNumber(body.y, 0, viewport.height, "y") : viewport.height / 2;
  const deltaX = clampFiniteNumber(body.deltaX || 0, -2000, 2000, "deltaX");
  const deltaY = clampFiniteNumber(body.deltaY || 0, -2000, 2000, "deltaY");

  await page.evaluate(({ x, y, deltaX, deltaY }) => {
    const canScroll = (element) => {
      if (!element || element === document) {
        return false;
      }
      const rect = element.getBoundingClientRect();
      const style = window.getComputedStyle(element);
      if (style.display === "none" || style.visibility === "hidden" || rect.width < 32 || rect.height < 32) {
        return false;
      }
      const canScrollY = element.scrollHeight - element.clientHeight > 1;
      const canScrollX = element.scrollWidth - element.clientWidth > 1;
      return canScrollY || canScrollX;
    };
    const scrollElement = (element) => {
      if (element === document.body || element === document.documentElement || element === document.scrollingElement) {
        const root = document.scrollingElement || document.documentElement || document.body;
        window.scrollBy({ left: deltaX, top: deltaY, behavior: "auto" });
        root.scrollLeft += deltaX;
        root.scrollTop += deltaY;
        return;
      }
      element.scrollBy({ left: deltaX, top: deltaY, behavior: "auto" });
      element.scrollLeft += deltaX;
      element.scrollTop += deltaY;
    };
    let target = document.elementFromPoint(x, y);
    for (let element = target; element; element = element.parentElement) {
      if (canScroll(element)) {
        scrollElement(element);
        return;
      }
    }
    const candidates = Array.from(document.querySelectorAll("body, body *"))
      .filter(canScroll)
      .sort((left, right) => {
        const leftArea = left.clientWidth * left.clientHeight;
        const rightArea = right.clientWidth * right.clientHeight;
        return rightArea - leftArea;
      });
    if (candidates[0]) {
      scrollElement(candidates[0]);
      return;
    }
    const scrollingElement = document.scrollingElement || document.documentElement || document.body;
    if (scrollingElement) {
      scrollElement(scrollingElement);
    } else {
      window.scrollBy({ left: deltaX, top: deltaY, behavior: "auto" });
    }
  }, { x, y, deltaX, deltaY });
}

async function scrollPageToRatio(page, ratio) {
  await page.evaluate((nextRatio) => {
    const canScroll = (element) => {
      if (!element || element === document) {
        return false;
      }
      const rect = element.getBoundingClientRect();
      const style = window.getComputedStyle(element);
      if (style.display === "none" || style.visibility === "hidden" || rect.width < 32 || rect.height < 32) {
        return false;
      }
      return element.scrollHeight - element.clientHeight > 1;
    };
    const candidates = Array.from(document.querySelectorAll("body, body *"))
      .filter(canScroll)
      .sort((left, right) => {
        const leftRange = left.scrollHeight - left.clientHeight;
        const rightRange = right.scrollHeight - right.clientHeight;
        return rightRange - leftRange;
      });
    let scrollingElement = candidates[0] || document.scrollingElement || document.documentElement || document.body;
    if (!scrollingElement) {
      return;
    }
    if (scrollingElement === document.body || scrollingElement === document.documentElement) {
      scrollingElement = document.scrollingElement || scrollingElement;
    }
    const maxTop = Math.max(0, scrollingElement.scrollHeight - scrollingElement.clientHeight);
    const nextTop = maxTop * nextRatio;
    scrollingElement.scrollTop = nextTop;
    if (scrollingElement === document.scrollingElement || scrollingElement === document.documentElement || scrollingElement === document.body) {
      window.scrollTo({ top: nextTop, behavior: "auto" });
    }
  }, ratio);
}

async function closeSessionBrowser(session) {
  const browser = session.browser;
  session.browser = null;
  session.context = null;
  session.page = null;
  if (browser) {
    await browser.close().catch(() => {});
  }
}

async function openControlledBrowser(session, token) {
  if (session.status === "completed") {
    return publicSession(session);
  }
  if (session.browser) {
    return publicSession(session);
  }
  const connector = connectorForID(session.connectorID);
  const headless = sessionBrowserHeadless(connector);
  session.status = "opening";
  session.error = "";
  const browser = await chromium.launch({
    headless,
    args: ["--disable-blink-features=AutomationControlled"],
  });
  const context = await browser.newContext({
    viewport: session.viewport || sessionViewport,
  });
  await context.addInitScript(() => {
    Object.defineProperty(navigator, "webdriver", {
      get: () => undefined,
    });
  });
  const page = await context.newPage();
  attachAuthWatcher(page, connector, (authCheck) => {
    session.authCheck = authCheck;
    if (authCheck.authenticated) {
      scheduleAutoComplete(session, token, "auth_watcher");
    }
  });
  session.browser = browser;
  session.context = context;
  session.page = page;
  await page.goto(session.loginURL, { waitUntil: "domcontentloaded", timeout: 60_000 });
  session.status = "browser_open";
  return sessionDetails(session);
}

function sessionBrowserHeadless(connector) {
  if (process.env.CRAWLER_WORKER_HEADLESS === "true") {
    return true;
  }
  if (process.env.CRAWLER_WORKER_HEADLESS === "false") {
    return false;
  }
  if (connector && typeof connector.sessionHeadlessDefault === "boolean") {
    return connector.sessionHeadlessDefault;
  }
  return remoteBrowserUI;
}

async function sessionScreenshot(session) {
  if (!session.page) {
    throw userError("open the controlled browser before requesting screenshots");
  }
  const page = session.page;
  const [image, title, scroll] = await Promise.all([
    page.screenshot({ type: "jpeg", quality: 72, fullPage: false, timeout: 15_000 }),
    page.title().catch(() => ""),
    pageScrollState(page).catch(() => session.scrollState || null),
  ]);
  session.scrollState = scroll;
  return {
    mime_type: "image/jpeg",
    image_base64: image.toString("base64"),
    viewport: page.viewportSize() || session.viewport || sessionViewport,
    title,
    current_url: page.url(),
    status: publicSession(session),
  };
}

async function streamSession(session, req, res) {
  if (!session.page) {
    writeJSON(res, 409, { error: "open the controlled browser before streaming" });
    return;
  }
  res.writeHead(200, {
    "content-type": "multipart/x-mixed-replace; boundary=multica-frame",
    "cache-control": "no-store, no-cache, must-revalidate, proxy-revalidate",
    "pragma": "no-cache",
    "connection": "keep-alive",
    "x-accel-buffering": "no",
  });

  let closed = false;
  req.on("close", () => {
    closed = true;
  });

  while (!closed && session.page && session.status !== "completed") {
    try {
      const image = await session.page.screenshot({
        type: "jpeg",
        quality: 68,
        fullPage: false,
        timeout: 8_000,
      });
      if (closed) {
        break;
      }
      res.write(
        `--multica-frame\r\nContent-Type: image/jpeg\r\nContent-Length: ${image.length}\r\n\r\n`,
      );
      res.write(image);
      res.write("\r\n");
    } catch (err) {
      if (session.status === "completed" || !session.page) {
        break;
      }
      session.error = err instanceof Error ? err.message : String(err);
      await sleep(Math.max(streamFrameIntervalMS, 500));
    }
    await sleep(streamFrameIntervalMS);
  }

  if (!closed) {
    res.end();
  }
}

async function applySessionInput(session, body) {
  if (!session.page) {
    throw userError("open the controlled browser before sending input");
  }
  const page = session.page;
  switch (body.type) {
    case "click": {
      const { x, y } = mousePointFromBody(page, session, body);
      const button = mouseButtonFromBody(body);
      await page.mouse.click(x, y, { button });
      break;
    }
    case "mouseDown": {
      const { x, y } = mousePointFromBody(page, session, body);
      await page.mouse.move(x, y);
      await page.mouse.down({ button: mouseButtonFromBody(body) });
      await sleep(60);
      break;
    }
    case "mouseMove": {
      const { x, y } = mousePointFromBody(page, session, body);
      const steps = Number.isFinite(Number(body.steps)) ? Math.max(1, Math.min(Number(body.steps), 12)) : 1;
      await page.mouse.move(x, y, { steps });
      break;
    }
    case "mouseUp": {
      const { x, y } = mousePointFromBody(page, session, body);
      const steps = Number.isFinite(Number(body.steps)) ? Math.max(1, Math.min(Number(body.steps), 12)) : 2;
      await page.mouse.move(x, y, { steps });
      await sleep(30);
      await page.mouse.up({ button: mouseButtonFromBody(body) });
      break;
    }
    case "type": {
      if (typeof body.text !== "string") {
        throw userError("input text is required", 400);
      }
      if (body.text.length > 8000) {
        throw userError("input text is too long", 400);
      }
      await page.keyboard.type(body.text, {
        delay: Number.isFinite(Number(body.delay)) ? Math.max(0, Math.min(Number(body.delay), 200)) : 0,
      });
      break;
    }
    case "insertText": {
      if (typeof body.text !== "string") {
        throw userError("input text is required", 400);
      }
      if (body.text.length > 16000) {
        throw userError("input text is too long", 400);
      }
      await page.keyboard.insertText(body.text);
      break;
    }
    case "press": {
      if (typeof body.key !== "string" || body.key.length === 0 || body.key.length > 80) {
        throw userError("input key is required", 400);
      }
      await page.keyboard.press(body.key);
      break;
    }
    case "wheel": {
      if (Number.isFinite(Number(body.x)) && Number.isFinite(Number(body.y))) {
        const { x, y } = mousePointFromBody(page, session, body);
        await page.mouse.move(x, y);
      }
      await scrollPageAtPoint(page, body);
      break;
    }
    case "scrollToRatio": {
      const ratio = clampFiniteNumber(body.ratio, 0, 1, "ratio");
      await scrollPageToRatio(page, ratio);
      break;
    }
    case "resizeViewport": {
      const width = Math.round(clampFiniteNumber(body.width, 480, 2560, "width"));
      const height = Math.round(clampFiniteNumber(body.height, 360, 1800, "height"));
      const current = page.viewportSize() || session.viewport || sessionViewport;
      if (Math.abs(current.width - width) > 3 || Math.abs(current.height - height) > 3) {
        const nextViewport = { width, height };
        await page.setViewportSize(nextViewport);
        session.viewport = nextViewport;
      }
      break;
    }
    case "back":
      await page.goBack({ waitUntil: "domcontentloaded", timeout: 30_000 }).catch(() => null);
      break;
    case "reload":
      await page.reload({ waitUntil: "domcontentloaded", timeout: 30_000 }).catch(() => null);
      break;
    default:
      throw userError("unsupported input type", 400);
  }
  if (shouldRefreshScrollAfterInput(body.type)) {
    session.scrollState = await pageScrollState(page).catch(() => session.scrollState || null);
  }
  return publicSession(session);
}

function shouldRefreshScrollAfterInput(type) {
  return type === "wheel"
    || type === "scrollToRatio"
    || type === "resizeViewport"
    || type === "back"
    || type === "reload";
}

function clampFiniteNumber(value, min, max, field) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    throw userError(`${field} must be a finite number`, 400);
  }
  return Math.max(min, Math.min(max, number));
}

async function completeSession(session, token) {
  if (session.status === "completed") {
    return {
      ...publicSession(session),
      verification: session.verification || null,
    };
  }
  if (!session.context) {
    throw userError("open the controlled browser and finish login before completing");
  }
  const storageState = await browserStorageState(session.context);
  const sessionStorageState = await captureSessionStorage(session.page);
  const connector = connectorForID(session.connectorID);
  const currentURL = session.page ? session.page.url() : "";
  if (isLoginURL(currentURL, connector)) {
    throw userError("AppGrowing is still on the login page; finish login in the controlled browser before completing");
  }
  if (!hasConnectorState(storageState, connector)) {
    throw userError("AppGrowing login state was not detected; finish login in the controlled browser before completing");
  }
  const authCheck = await refreshSessionAuth(session, connector);
  if (!authCheck.authenticated) {
    throw userError("AppGrowing authenticated user was not detected; finish login in the controlled browser before completing");
  }
  const verification = {
    current_url: currentURL,
    credential_state: summarizeCredentialState(storageState, connector, sessionStorageState),
    auth_check: authCheck,
  };
  const sealed = sealState({
    connector_id: session.connectorID,
    profile_id: session.profileID,
    captured_at: new Date().toISOString(),
    storage_state: storageState,
    session_storage: sessionStorageState,
  });
  const upstream = await fetch(`${apiURL}/api/credential-login-sessions/complete`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({
      session_token: token,
      ciphertext: sealed.toString("base64"),
      key_version: keyVersion,
    }),
  });
  const text = await upstream.text();
  if (!upstream.ok) {
    throw new Error(text || upstream.statusText);
  }
  session.status = "completed";
  session.error = "";
  session.verification = verification;
  await closeSessionBrowser(session);
  return {
    ...JSON.parse(text),
    verification,
  };
}

function scheduleAutoComplete(session, token, reason) {
  if (!token || session.status === "completed" || session.autoCompleting) {
    return;
  }
  session.autoCompleting = true;
  session.autoCompleteReason = reason;
  setTimeout(async () => {
    try {
      await completeSession(session, token);
    } catch (err) {
      session.error = err instanceof Error ? err.message : String(err);
    } finally {
      session.autoCompleting = false;
    }
  }, 800);
}

function isLoginURL(url, connector) {
  if (!url) {
    return true;
  }
  return (connector.loginURLPattern || /login|signin|passport/i).test(url);
}

async function verifyConnectorAuth(context, connector) {
  const check = connector.authCheck;
  if (!check) {
    return { authenticated: true, method: "none" };
  }
  let response;
  try {
    response = await context.request.post(check.url, {
      data: check.payload,
      headers: {
        "content-type": "application/json",
      },
      timeout: 30_000,
    });
  } catch (err) {
    return authCheckUnavailable(err, "worker_graphql_userinfo");
  }
  let body = null;
  try {
    body = await response.json();
  } catch {
    body = null;
  }
  return authCheckFromBody(body, connector, response.status(), "worker_graphql_userinfo");
}

async function browserStorageState(context) {
  try {
    return await context.storageState({ indexedDB: true });
  } catch {
    return context.storageState();
  }
}

async function captureSessionStorage(page) {
  if (!page) {
    return {};
  }
  return page.evaluate(() => {
    const entries = {};
    for (let index = 0; index < sessionStorage.length; index += 1) {
      const key = sessionStorage.key(index);
      if (key) {
        entries[key] = sessionStorage.getItem(key);
      }
    }
    return {
      [window.location.origin]: entries,
    };
  }).catch(() => ({}));
}

async function restoreSessionStorage(context, sessionStorageState) {
  if (!sessionStorageState || Object.keys(sessionStorageState).length === 0) {
    return;
  }
  await context.addInitScript((stateByOrigin) => {
    const entries = stateByOrigin[window.location.origin];
    if (!entries) {
      return;
    }
    for (const [key, value] of Object.entries(entries)) {
      if (value != null) {
        window.sessionStorage.setItem(key, value);
      }
    }
  }, sessionStorageState);
}

async function verifyConnectorPageAuth(page, connector) {
  const check = connector.authCheck;
  if (!check) {
    return { authenticated: true, method: "none" };
  }
  const result = await page.evaluate(async ({ url, payload }) => {
    const response = await fetch(url, {
      method: "POST",
      credentials: "include",
      headers: {
        "content-type": "application/json",
      },
      body: JSON.stringify(payload),
    });
    let body = null;
    try {
      body = await response.json();
    } catch {
      body = null;
    }
    return {
      status: response.status,
      body,
    };
  }, {
    url: check.url,
    payload: check.payload,
  });
  return authCheckFromBody(result.body, connector, result.status, "page_fetch_graphql_userinfo");
}

function authCheckFromBody(body, connector, httpStatus, method) {
  const check = connector.authCheck;
  const userID = valueAtPath(body, check.userIDPath);
  return {
    authenticated: typeof userID === "string" ? userID.trim() !== "" : Boolean(userID),
    method,
    http_status: httpStatus,
    user_id_present: typeof userID === "string" ? userID.trim() !== "" : Boolean(userID),
    team_present: Boolean(valueAtPath(body, ["data", "userinfo", "teamInfo"])),
    plan_present: Boolean(valueAtPath(body, ["data", "userinfo", "purchasePlanInfo"])),
    observed_at: new Date().toISOString(),
  };
}

function authCheckUnavailable(err, method) {
  return {
    authenticated: false,
    method,
    http_status: null,
    user_id_present: false,
    team_present: false,
    plan_present: false,
    probe_error: err instanceof Error ? err.message : String(err),
    observed_at: new Date().toISOString(),
  };
}

function browserStateAuthFallback(authCheck, method) {
  return {
    ...authCheck,
    authenticated: true,
    method,
    browser_state_present: true,
    user_id_present: authCheck?.user_id_present === true,
    observed_at: new Date().toISOString(),
  };
}

function isAuthCheckResponse(response, connector) {
  const check = connector.authCheck;
  if (!check || response.url() !== check.url) {
    return false;
  }
  const postData = response.request().postData() || "";
  return postData.includes('"operationName":"userinfo"') || postData.includes("query userinfo");
}

async function parseAuthCheckResponse(response, connector) {
  if (!isAuthCheckResponse(response, connector)) {
    return null;
  }
  let body = null;
  try {
    body = await response.json();
  } catch {
    return {
      authenticated: false,
      method: "page_graphql_userinfo",
      http_status: response.status(),
      user_id_present: false,
      team_present: false,
      plan_present: false,
      observed_at: new Date().toISOString(),
    };
  }
  return authCheckFromBody(body, connector, response.status(), "page_graphql_userinfo");
}

function attachAuthWatcher(page, connector, onAuthCheck) {
  if (!connector.authCheck) {
    return;
  }
  page.on("response", async (response) => {
    const authCheck = await parseAuthCheckResponse(response, connector).catch(() => null);
    if (authCheck) {
      onAuthCheck(authCheck);
    }
  });
}

async function waitForNextAuthCheck(page, connector, timeout = 15_000) {
  if (!connector.authCheck) {
    return { authenticated: true, method: "none" };
  }
  const response = await page.waitForResponse((candidate) => isAuthCheckResponse(candidate, connector), {
    timeout,
  }).catch(() => null);
  if (!response) {
    return null;
  }
  return parseAuthCheckResponse(response, connector);
}

async function refreshSessionAuth(session, connector) {
  if (session.authCheck?.authenticated) {
    return session.authCheck;
  }
  if (!session.page) {
    return verifyConnectorAuth(session.context, connector);
  }
  const waitForAuth = waitForNextAuthCheck(session.page, connector, 20_000);
  await session.page.goto(connector.probeURL || session.loginURL, {
    waitUntil: "domcontentloaded",
    timeout: 60_000,
  }).catch(() => {});
  const pageAuthCheck = await waitForAuth;
  if (pageAuthCheck) {
    session.authCheck = pageAuthCheck;
    return pageAuthCheck;
  }
  session.authCheck = await verifyConnectorPageAuth(session.page, connector).catch(async () =>
    verifyConnectorAuth(session.context, connector),
  );
  if (!session.authCheck.authenticated && !isLoginURL(session.page.url(), connector)) {
    session.authCheck = browserStateAuthFallback(session.authCheck, "browser_state_non_login_url");
  }
  return session.authCheck;
}

function valueAtPath(value, path) {
  let current = value;
  for (const key of path || []) {
    if (!current || typeof current !== "object" || !(key in current)) {
      return null;
    }
    current = current[key];
  }
  return current;
}

function hasConnectorState(storageState, connector) {
  const domains = connector.stateDomains || [];
  if (domains.length === 0) {
    return true;
  }
  return domains.some((domain) => {
    const needle = domain.toLowerCase();
    const hasCookie = (storageState.cookies || []).some((cookie) =>
      String(cookie.domain || "").toLowerCase().includes(needle),
    );
    const hasOrigin = (storageState.origins || []).some((origin) =>
      String(origin.origin || "").toLowerCase().includes(needle)
      && Array.isArray(origin.localStorage)
      && origin.localStorage.length > 0,
    );
    return hasCookie || hasOrigin;
  });
}

function summarizeCredentialState(storageState, connector, sessionStorageState = {}) {
  const cookieDomains = new Set();
  for (const cookie of storageState.cookies || []) {
    if (cookie.domain) {
      cookieDomains.add(String(cookie.domain).replace(/^\./, ""));
    }
  }
  const origins = (storageState.origins || []).map((origin) => origin.origin).filter(Boolean);
  return {
    captured: hasConnectorState(storageState, connector),
    cookie_count: (storageState.cookies || []).length,
    origin_count: origins.length,
    domains: Array.from(cookieDomains).sort(),
    origins,
    session_storage_origins: Object.keys(sessionStorageState).sort(),
    session_storage_key_count: Object.values(sessionStorageState)
      .reduce((sum, entries) => sum + Object.keys(entries || {}).length, 0),
  };
}

function sessionHTML(token, session) {
  const statusURL = `/sessions/${encodeURIComponent(token)}/status`;
  const openURL = `/sessions/${encodeURIComponent(token)}/open`;
  const completeURL = `/sessions/${encodeURIComponent(token)}/complete`;
  const screenshotURL = `/sessions/${encodeURIComponent(token)}/screenshot`;
  const streamURL = `/sessions/${encodeURIComponent(token)}/stream`;
  const inputURL = `/sessions/${encodeURIComponent(token)}/input`;
  return `<!doctype html>
<meta charset="utf-8">
<title>Multica Credential Broker</title>
<style>
*{box-sizing:border-box}
html,body{height:100%}
body{font:14px/1.5 ui-sans-serif,system-ui,-apple-system,Segoe UI,sans-serif;margin:0;color:#161616;background:#111}
.shell{display:flex;height:100vh;min-height:0;flex-direction:column;background:#111}
.topbar{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:12px 14px;border-bottom:1px solid #d8d6cd;background:#fff}
.brand{min-width:0}
h1{font-size:15px;line-height:1.2;margin:0}
.meta{margin-top:3px;color:#656565;font-size:12px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:56vw}
.statusline{display:flex;align-items:center;gap:10px;min-width:0}
.pill{display:inline-flex;align-items:center;height:28px;border-radius:999px;padding:0 10px;background:#eceff3;color:#333;font-size:12px;font-weight:600;white-space:nowrap}
.pill[data-tone="ok"]{background:#e6f5ea;color:#17602b}
.pill[data-tone="warn"]{background:#fff2cc;color:#6d4d00}
.pill[data-tone="error"]{background:#fde7e7;color:#8a1f1f}
.url{max-width:34vw;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:#707070;font-size:12px}
.viewport{flex:1;min-height:0;overflow:hidden;padding:10px;background:#151515}
.stage{position:relative;display:grid;place-items:center;height:100%;min-height:420px;background:#202020;border:1px solid #2d2d2d;overflow:hidden;outline:none}
.stage:focus{box-shadow:0 0 0 2px #2d6cdf inset}
.stage img{display:block;max-width:100%;max-height:100%;object-fit:contain;user-select:none}
.scrollbar{position:absolute;top:12px;right:10px;bottom:12px;width:16px;border-radius:999px;background:rgba(255,255,255,.22);border:1px solid rgba(0,0,0,.28);box-shadow:0 4px 16px rgba(0,0,0,.28);cursor:pointer;opacity:.72;pointer-events:none;transition:opacity .12s ease}
.stage:hover .scrollbar,.scrollbar[data-dragging="true"]{opacity:.96}
.scrollbar[data-visible="true"]{pointer-events:auto}
.scrollbar[data-visible="false"]{display:none}
.scroll-thumb{position:absolute;left:2px;right:2px;min-height:32px;border-radius:999px;background:#f8f8f8;box-shadow:0 1px 4px rgba(0,0,0,.32)}
.empty{position:absolute;color:#f5f2ea;background:rgba(0,0,0,.55);padding:10px 12px;border-radius:4px}
.debug{display:none}
@media (max-width: 720px){.topbar{align-items:flex-start;flex-direction:column}.statusline{width:100%}.url{max-width:100%}.meta{max-width:88vw}.stage{min-height:360px}}
</style>
<div class="shell">
<header class="topbar">
  <div class="brand">
    <h1>AppGrowing credential binding</h1>
    <div class="meta">Profile ${escapeHTML(session.profileID)} expires ${escapeHTML(new Date(session.expiresAt).toLocaleString())}</div>
  </div>
  <div class="statusline">
    <span id="statusPill" class="pill">Starting</span>
    <span id="urlLine" class="url"></span>
  </div>
</header>
<main class="viewport">
  <section id="stage" class="stage" tabindex="0" aria-label="Remote browser">
    <img id="shot" alt="Remote browser screenshot">
    <div id="scrollbar" class="scrollbar" data-visible="false" role="scrollbar" aria-orientation="vertical" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0">
      <div id="scrollThumb" class="scroll-thumb"></div>
    </div>
    <div id="empty" class="empty">Starting remote browser...</div>
  </section>
</main>
<pre id="out" class="debug">Starting remote browser...</pre>
</div>
<script>
const out = document.querySelector("#out");
const shot = document.querySelector("#shot");
const stage = document.querySelector("#stage");
const empty = document.querySelector("#empty");
const statusPill = document.querySelector("#statusPill");
const urlLine = document.querySelector("#urlLine");
const scrollbar = document.querySelector("#scrollbar");
const scrollThumb = document.querySelector("#scrollThumb");
const urls = {
  status: ${JSON.stringify(statusURL)},
  open: ${JSON.stringify(openURL)},
  complete: ${JSON.stringify(completeURL)},
  screenshot: ${JSON.stringify(screenshotURL)},
  stream: ${JSON.stringify(streamURL)},
  input: ${JSON.stringify(inputURL)}
};
let polling = 0;
let screenshotBusy = false;
let remoteFocused = false;
let opening = false;
let completeBusy = false;
let completed = false;
let lastAutoCompleteAt = 0;
let lastViewport = ${JSON.stringify(session.viewport || sessionViewport)};
let pointerDown = false;
let pointerButton = "left";
let lastPointerMoveAt = 0;
let pointerMoveSending = false;
let pendingPointerMove = null;
let streamStarted = false;
let lastScroll = null;
let scrollDragging = false;
let lastScrollDragAt = 0;
let browserOpen = false;
let resizeTimer = 0;
let resizeBusy = false;
let pendingResize = null;
let lastResize = { width: 0, height: 0 };
let inputChain = Promise.resolve();
let completionPosted = false;

async function parseResponse(resp) {
  const text = await resp.text();
  try { return JSON.parse(text); }
  catch { return { raw: text }; }
}

async function show(resp) {
  const body = await parseResponse(resp);
  renderStatus(body);
  return body;
}

async function status() {
  const body = await show(await fetch(urls.status));
  if (body.browser_open) {
    startStream();
    startPolling();
    scheduleViewportResize(0);
  } else {
    openBrowser();
  }
}

function renderStatus(body) {
  const url = body.current_url || body.verification?.current_url || "";
  if (body.viewport) {
    lastViewport = body.viewport;
  }
  browserOpen = body.browser_open === true || Boolean(body.verification);
  renderScroll(body.scroll || body.status?.scroll || null);
  if (body.verification) {
    completed = true;
    if (polling) {
      window.clearInterval(polling);
      polling = 0;
    }
    setStatus("Saved", "ok", url);
    notifyCompleted(body);
    debug({
      status: body.status,
      current_url: body.verification.current_url,
      auth_check: body.verification.auth_check,
      credential_state: body.verification.credential_state
    });
    return;
  }

  if (body.status === "completed") {
    completed = true;
    if (polling) {
      window.clearInterval(polling);
      polling = 0;
    }
    setStatus("Saved", "ok", url);
    notifyCompleted(body);
  } else if (body.error) {
    setStatus("Action needed", "error", url);
  } else if (body.auto_completing) {
    setStatus("Saving", "warn", url);
  } else if (body.auth_check?.authenticated) {
    setStatus("Login detected", "ok", url);
  } else if (body.browser_open) {
    setStatus("Waiting for login", "warn", url);
  } else {
    setStatus("Starting", "warn", url);
  }

  debug({
    status: body.status || body.error || "pending",
    browser_open: body.browser_open === true,
    current_url: url || null,
    auth_check: body.auth_check || null,
    error: body.error || null
  });

  if (!completed && body.auth_check?.authenticated) {
    autoComplete("auth_detected", false);
    return;
  }
  if (!completed && looksReadyForCompletion(url)) {
    autoComplete("app_page_detected", false);
  }
}

function renderShot(body) {
  shot.src = "data:" + body.mime_type + ";base64," + body.image_base64;
  lastViewport = body.viewport || lastViewport;
  empty.hidden = true;
  renderStatus({
    ...(body.status || {}),
    current_url: body.current_url,
  });
  if (body.status?.auth_check?.authenticated) {
    autoComplete("auth_detected", false);
    return;
  }
  if (looksReadyForCompletion(body.current_url)) {
    autoComplete("app_page_detected", false);
  }
}

function startStream() {
  if (streamStarted || completed) {
    return;
  }
  streamStarted = true;
  shot.onload = () => {
    empty.hidden = true;
    if (!lastResize.width) {
      scheduleViewportResize(0);
    }
  };
  shot.onerror = () => {
    streamStarted = false;
    if (!completed) {
      empty.hidden = false;
      empty.textContent = "Reconnecting remote browser...";
      window.setTimeout(startStream, 800);
    }
  };
  shot.src = urls.stream + "?t=" + Date.now();
}

function setStatus(label, tone, url) {
  statusPill.textContent = label;
  statusPill.dataset.tone = tone || "";
  urlLine.textContent = url || "";
}

function notifyCompleted(body) {
  if (completionPosted) {
    return;
  }
  completionPosted = true;
  window.parent?.postMessage({
    type: "multica:credential-session-completed",
    profile_id: body.profile_id,
    connector_id: body.connector_id,
    status: body.status,
    verification: body.verification || null,
  }, "*");
}

function debug(value) {
  out.textContent = JSON.stringify(value, null, 2);
}

function renderScroll(scroll) {
  if (!scroll || scroll.can_scroll !== true) {
    lastScroll = scroll || null;
    scrollbar.dataset.visible = "false";
    scrollbar.setAttribute("aria-valuenow", "0");
    return;
  }
  lastScroll = scroll;
  const scrollHeight = Number(scroll.scroll_height) || 0;
  const viewportHeight = Number(scroll.viewport_height) || 0;
  const ratio = Math.max(0, Math.min(1, Number(scroll.ratio) || 0));
  const thumbPercent = scrollHeight > 0
    ? Math.max(8, Math.min(100, viewportHeight / scrollHeight * 100))
    : 100;
  const topPercent = ratio * (100 - thumbPercent);
  scrollbar.dataset.visible = "true";
  scrollbar.setAttribute("aria-valuenow", String(Math.round(ratio * 100)));
  scrollThumb.style.height = thumbPercent + "%";
  scrollThumb.style.top = topPercent + "%";
}

function scrollRatioFromEvent(event) {
  const rect = scrollbar.getBoundingClientRect();
  if (rect.height <= 0) {
    return 0;
  }
  const currentThumbHeight = scrollThumb.getBoundingClientRect().height || 32;
  const usableHeight = Math.max(1, rect.height - currentThumbHeight);
  const rawTop = event.clientY - rect.top - currentThumbHeight / 2;
  return Math.max(0, Math.min(1, rawTop / usableHeight));
}

function renderLocalScrollRatio(ratio) {
  if (!lastScroll || lastScroll.can_scroll !== true) {
    return;
  }
  renderScroll({
    ...lastScroll,
    ratio,
    scroll_top: Number(lastScroll.max_top || 0) * ratio,
  });
}

async function sendScrollRatio(ratio, force) {
  const now = Date.now();
  if (!force && now - lastScrollDragAt < 45) {
    return;
  }
  lastScrollDragAt = now;
  await sendInput({type: "scrollToRatio", ratio}, {render: force === true});
}

function desiredViewportSize() {
  const rect = stage.getBoundingClientRect();
  let width = Math.round(rect.width);
  let height = Math.round(rect.height);
  if (!width || !height) {
    return null;
  }
  const scale = Math.min(1, 2400 / width, 1600 / height);
  width = Math.max(480, Math.round(width * scale));
  height = Math.max(360, Math.round(height * scale));
  return { width, height };
}

function scheduleViewportResize(delay = 120) {
  if (!browserOpen || completed) {
    return;
  }
  window.clearTimeout(resizeTimer);
  resizeTimer = window.setTimeout(() => {
    const next = desiredViewportSize();
    if (!next) {
      return;
    }
    const changedFromLast = Math.abs(next.width - lastResize.width) > 8
      || Math.abs(next.height - lastResize.height) > 8;
    const changedFromRemote = Math.abs(next.width - Number(lastViewport.width || 0)) > 8
      || Math.abs(next.height - Number(lastViewport.height || 0)) > 8;
    if (!changedFromLast && !changedFromRemote) {
      return;
    }
    pendingResize = next;
    runViewportResize();
  }, delay);
}

async function runViewportResize() {
  if (resizeBusy || !pendingResize || !browserOpen || completed) {
    return;
  }
  resizeBusy = true;
  const next = pendingResize;
  pendingResize = null;
  lastResize = next;
  try {
    await sendInput({type: "resizeViewport", ...next}, {render: true});
  } catch (err) {
    debug({ status: "resize_failed", error: err instanceof Error ? err.message : String(err) });
  } finally {
    resizeBusy = false;
    if (pendingResize) {
      runViewportResize();
    }
  }
}

function looksReadyForCompletion(rawURL) {
  try {
    const parsed = new URL(rawURL);
    return parsed.hostname === "appgrowing-global.youcloud.com";
  } catch {
    return false;
  }
}

async function refreshShot() {
  if (screenshotBusy || completed) {
    return;
  }
  screenshotBusy = true;
  try {
    const resp = await fetch(urls.screenshot);
    const body = await parseResponse(resp);
    if (!resp.ok) {
      const statusResp = await fetch(urls.status).catch(() => null);
      if (statusResp?.ok) {
        renderStatus(await parseResponse(statusResp));
      } else {
        renderStatus(body);
      }
      return;
    }
    renderShot(body);
  } finally {
    screenshotBusy = false;
  }
}

function startPolling() {
  if (polling) {
    return;
  }
  polling = window.setInterval(async () => {
    const resp = await fetch(urls.status).catch(() => null);
    if (resp?.ok) {
      renderStatus(await parseResponse(resp));
    }
  }, 1000);
}

async function openBrowser() {
  if (opening || completed) {
    return;
  }
  opening = true;
  empty.hidden = false;
  empty.textContent = "Starting remote browser...";
  setStatus("Starting", "warn", "");
  try {
    await show(await fetch(urls.open, {method: "POST"}));
    startStream();
    startPolling();
    scheduleViewportResize(0);
  } finally {
    opening = false;
  }
}

async function autoComplete(reason, force) {
  if (completed || completeBusy) {
    return;
  }
  const now = Date.now();
  if (!force && now - lastAutoCompleteAt < 5000) {
    return;
  }
  lastAutoCompleteAt = now;
  completeBusy = true;
  setStatus("Saving", "warn", urlLine.textContent);
  try {
    const resp = await fetch(urls.complete, {method: "POST"});
    const body = await parseResponse(resp);
    renderStatus(body);
    if (!resp.ok) {
      if (!force) {
        setStatus("Waiting for login", "warn", urlLine.textContent);
        debug({ status: "waiting", auto_complete_attempt: reason, error: body.error || body.raw || null });
      }
      completeBusy = false;
      return;
    }
    completed = true;
  } finally {
    completeBusy = false;
  }
}

async function sendInput(action, options = {}) {
  const next = inputChain
    .catch(() => {})
    .then(() => sendInputNow(action, options));
  inputChain = next.catch(() => {});
  return next;
}

async function sendInputNow(action, options = {}) {
  const refresh = options.refresh === true;
  const render = options.render !== false;
  const resp = await fetch(urls.input, {
    method: "POST",
    headers: {"content-type": "application/json"},
    body: JSON.stringify(action)
  });
  if (render || !resp.ok) {
    await show(resp);
  }
  if (refresh) {
    await refreshShot();
  }
}

async function sendPointerMove(action) {
  pendingPointerMove = action;
  if (pointerMoveSending) {
    return;
  }
  pointerMoveSending = true;
  try {
    while (pointerDown && pendingPointerMove) {
      const next = pendingPointerMove;
      pendingPointerMove = null;
      await sendInput(next, {refresh: false, render: false});
    }
  } finally {
    pointerMoveSending = false;
    if (pointerDown && pendingPointerMove) {
      sendPointerMove(pendingPointerMove);
    }
  }
}

function remotePoint(event) {
  const rect = shot.getBoundingClientRect();
  if (!shot.src || rect.width <= 0 || rect.height <= 0) {
    return null;
  }
  return {
    x: (event.clientX - rect.left) * lastViewport.width / rect.width,
    y: (event.clientY - rect.top) * lastViewport.height / rect.height,
  };
}

async function pasteText(text) {
  const value = typeof text === "string" ? text : "";
  if (!value) {
    return false;
  }
  remoteFocused = true;
  stage.focus();
  await sendInput({type: "insertText", text: value});
  return true;
}

async function pasteFromClipboard() {
  try {
    const text = await navigator.clipboard.readText();
    return pasteText(text);
  } catch {
    return false;
  }
}

scrollbar.addEventListener("pointerdown", (event) => {
  if (scrollbar.dataset.visible !== "true") {
    return;
  }
  event.preventDefault();
  event.stopPropagation();
  remoteFocused = true;
  stage.focus();
  scrollDragging = true;
  scrollbar.dataset.dragging = "true";
  try { scrollbar.setPointerCapture?.(event.pointerId); } catch {}
  const ratio = scrollRatioFromEvent(event);
  renderLocalScrollRatio(ratio);
  sendScrollRatio(ratio, false);
});

scrollbar.addEventListener("pointermove", (event) => {
  if (!scrollDragging) {
    return;
  }
  event.preventDefault();
  event.stopPropagation();
  const ratio = scrollRatioFromEvent(event);
  renderLocalScrollRatio(ratio);
  sendScrollRatio(ratio, false);
});

async function releaseScrollDrag(event) {
  if (!scrollDragging) {
    return;
  }
  event.preventDefault();
  event.stopPropagation();
  scrollDragging = false;
  scrollbar.dataset.dragging = "false";
  try { scrollbar.releasePointerCapture?.(event.pointerId); } catch {}
  const ratio = scrollRatioFromEvent(event);
  renderLocalScrollRatio(ratio);
  await sendScrollRatio(ratio, true);
}

scrollbar.addEventListener("pointerup", releaseScrollDrag);
scrollbar.addEventListener("pointercancel", releaseScrollDrag);

stage.addEventListener("pointerdown", async (event) => {
  const point = remotePoint(event);
  if (!point) {
    return;
  }
  event.preventDefault();
  remoteFocused = true;
  stage.focus();
  pointerDown = true;
  pointerButton = event.button === 2 ? "right" : event.button === 1 ? "middle" : "left";
  try { stage.setPointerCapture?.(event.pointerId); } catch {}
  await sendInput({type: "mouseDown", ...point, button: pointerButton}, {refresh: false, render: false});
});

stage.addEventListener("pointermove", (event) => {
  if (!pointerDown) {
    return;
  }
  const now = Date.now();
  if (now - lastPointerMoveAt < 20) {
    return;
  }
  const point = remotePoint(event);
  if (!point) {
    return;
  }
  lastPointerMoveAt = now;
  event.preventDefault();
  sendPointerMove({type: "mouseMove", ...point, steps: 3});
});

async function releasePointer(event) {
  if (!pointerDown) {
    return;
  }
  const point = remotePoint(event);
  pointerDown = false;
  pendingPointerMove = null;
  try { stage.releasePointerCapture?.(event.pointerId); } catch {}
  if (!point) {
    return;
  }
  event.preventDefault();
  await sendInput({type: "mouseUp", ...point, button: pointerButton, steps: 2}, {render: false});
}

stage.addEventListener("pointerup", releasePointer);
stage.addEventListener("pointercancel", releasePointer);
stage.addEventListener("pointerleave", (event) => {
  if (pointerDown && event.buttons === 0) {
    releasePointer(event);
  }
});

stage.addEventListener("wheel", (event) => {
  const point = remotePoint(event);
  if (!point) {
    return;
  }
  remoteFocused = true;
  stage.focus();
  event.preventDefault();
  if (lastScroll?.can_scroll === true && Number(lastScroll.max_top) > 0) {
    const nextTop = Math.max(0, Math.min(Number(lastScroll.max_top), Number(lastScroll.scroll_top || 0) + event.deltaY));
    renderLocalScrollRatio(nextTop / Number(lastScroll.max_top));
  }
  sendInput({type: "wheel", ...point, deltaX: event.deltaX, deltaY: event.deltaY}, {render: false});
}, {passive: false});

stage.addEventListener("contextmenu", (event) => event.preventDefault());

window.addEventListener("keydown", (event) => {
  if (remoteFocused && (event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "v") {
    event.preventDefault();
    pasteFromClipboard();
    return;
  }
  if (!remoteFocused || event.metaKey || event.ctrlKey || event.altKey) {
    return;
  }
  event.preventDefault();
  if (event.key.length === 1) {
    sendInput({type: "type", text: event.key});
    return;
  }
  sendInput({type: "press", key: event.key});
});

window.addEventListener("paste", (event) => {
  const text = event.clipboardData?.getData("text");
  if (!text) {
    return;
  }
  event.preventDefault();
  pasteText(text);
});

window.addEventListener("resize", () => scheduleViewportResize(120));
if ("ResizeObserver" in window) {
  new ResizeObserver(() => scheduleViewportResize(120)).observe(stage);
}

status();
</script>`;
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function runAppGrowingMaterialSearch(page, connector, params = {}) {
  params = normalizeAppGrowingMaterialSearchParams(params);
  const competitors = normalizeCompetitors(params.competitors);
  if (competitors.length === 0) {
    throw userError("material_search params.competitors is required", 400);
  }
  const priorityCompetitors = new Set(
    normalizeCompetitors(params.priority_competitors).map(normalizeCompetitorKey),
  );
  const totalLimit = positiveIntegerParam(params.limit, 25, 1, 200);
  const pageLimit = positiveIntegerParam(params.pages_per_competitor, 1, 1, 5);
  const priorityPageLimit = positiveIntegerParam(params.priority_pages_per_competitor, Math.max(2, pageLimit), 1, 8);
  const captureTimeoutMS = positiveIntegerParam(params.capture_timeout_ms, 12_000, 3_000, 60_000);
  const rules = normalizeMaterialRules(params.selection_rules || params.rules);

  const captured = [];
  const materials = [];
  for (const competitor of competitors) {
    const priority = priorityCompetitors.has(normalizeCompetitorKey(competitor));
    const pages = priority ? priorityPageLimit : pageLimit;
    for (let pageNumber = 1; pageNumber <= pages; pageNumber += 1) {
      const capture = await captureAppGrowingMaterialPage(page, connector, {
        competitor,
        pageNumber,
        params,
        captureTimeoutMS,
      });
      captured.push({
        competitor,
        priority,
        page: pageNumber,
        url: capture.url,
        graphQL_operations: capture.operations,
        graphQL_responses: capture.responses,
        page_snapshot: capture.snapshot,
        needs_reauth: capture.needsReauth,
        materials_found: capture.materials.length,
      });
      for (const material of capture.materials) {
        materials.push({
          ...material,
          competitor,
          priority,
        });
      }
    }
  }

  const selection = selectAppGrowingMaterials(materials, rules, totalLimit, {
    fallbackToTopMaterials: params.fallback_to_top_materials === true,
  });
  const needsReauth = captured.some((capture) => capture.needs_reauth === true);
  return {
    status: needsReauth ? "need_reauth" : "completed",
    downloaded: 0,
    output_prefix: `local://credential-broker/${params.profile_id || "appgrowing"}/materials`,
    message: needsReauth
      ? "AppGrowing reported that the account was logged out; re-authentication is required"
      : `selected ${selection.selected.length} AppGrowing materials from ${competitors.length} competitors; asset download storage is not configured`,
    raw: {
      connector_id: connector.id,
      capability: "material_search",
      strategy: {
        competitors,
        priority_competitors: Array.from(priorityCompetitors),
        rules: {
          new_materials: materialRuleSummary(rules.new_materials),
          volume_materials: materialRuleSummary(rules.volume_materials),
        },
        total_limit: totalLimit,
        pages_per_competitor: pageLimit,
        priority_pages_per_competitor: priorityPageLimit,
        fallback_to_top_materials: params.fallback_to_top_materials === true,
      },
      captured,
      totals: {
        materials_seen: materials.length,
        unique_materials: selection.unique.length,
        new_candidates: selection.newCandidates.length,
        volume_candidates: selection.volumeCandidates.length,
        selected: selection.selected.length,
        missing_duration_days: selection.unique.filter((item) => !Number.isFinite(Number(item.duration_days))).length,
        missing_impression_estimate: selection.unique.filter((item) => !Number.isFinite(Number(item.impression_estimate))).length,
      },
      material_samples: summarizeMaterialSamples(selection.unique),
      selected_materials: selection.selected,
    },
  };
}

async function captureAppGrowingMaterialPage(page, connector, options) {
  const url = appGrowingMaterialURL(connector, options);
  const captured = [];
  const operations = new Set();
  const responses = [];

  const onResponse = async (response) => {
    if (!response.url().startsWith(connector.graphQLURL)) {
      return;
    }
    let operationNames = [];
    try {
      const postData = response.request().postData() || "";
      if (postData) {
        operationNames = graphQLOperationNames(postData);
      }
    } catch {
      operationNames = [];
    }
    for (const operationName of operationNames) {
      operations.add(operationName);
    }
    responses.push({
      status: response.status(),
      operations: operationNames,
    });
    try {
      const body = await response.json();
      const materials = extractAppGrowingMaterials(body);
      if (materials.length > 0) {
        captured.push(...materials);
      }
    } catch {
      // Ignore non-JSON GraphQL responses and keep the crawl best-effort.
    }
  };

  page.on("response", onResponse);
  try {
    await page.goto(url, { waitUntil: "domcontentloaded", timeout: 60_000 });
    await Promise.race([
      page.waitForLoadState("networkidle", { timeout: options.captureTimeoutMS }).catch(() => null),
      sleep(options.captureTimeoutMS),
    ]);
    await stimulateAppGrowingMaterialList(page, options.captureTimeoutMS);
  } finally {
    page.off("response", onResponse);
  }

  const snapshot = await appGrowingPageSnapshot(page);
  return {
    url,
    operations: Array.from(operations).sort(),
    responses,
    snapshot,
    needsReauth: pageSnapshotHasAnonymousText(snapshot, connector),
    materials: captured,
  };
}

async function appGrowingPageSnapshot(page) {
  const text = await page.locator("body").innerText({ timeout: 3000 }).catch(() => "");
  return {
    url: page.url(),
    title: await page.title().catch(() => ""),
    text: text.replace(/\s+/g, " ").trim().slice(0, 1000),
  };
}

function pageSnapshotHasAnonymousText(snapshot, connector) {
  const text = snapshot?.text || "";
  return (connector.anonymousTextPatterns || []).some((pattern) => pattern.test(text));
}

function graphQLOperationNames(postData) {
  const payload = JSON.parse(postData);
  const entries = Array.isArray(payload) ? payload : [payload];
  return entries
    .map((entry) => String(entry?.operationName || ""))
    .filter(Boolean);
}

async function stimulateAppGrowingMaterialList(page, captureTimeoutMS) {
  const waitMS = Math.min(2000, Math.max(500, Math.floor(captureTimeoutMS / 6)));
  await page.mouse.wheel(0, 900).catch(() => {});
  await sleep(waitMS);
  await page.keyboard.press("End").catch(() => {});
  await sleep(waitMS);
  await page.keyboard.press("Home").catch(() => {});
  await sleep(waitMS);
}

function appGrowingMaterialURL(connector, { competitor, pageNumber, params }) {
  const competitorURL = appGrowingCompetitorURL(connector, competitor, params);
  if (competitorURL) {
    competitorURL.searchParams.set("page", String(pageNumber));
    applyAppGrowingMaterialFilters(competitorURL.searchParams, params);
    return competitorURL.toString();
  }

  const url = new URL(connector.probeURL);
  const search = url.searchParams;
  search.set("purpose", String(params.purpose ?? 2));
  search.set("keyword", competitor);
  search.set("daterange", String(params.daterange || params.date_range || "-29,0"));
  if (typeof params.startDate === "string" || typeof params.start_date === "string") {
    search.set("startDate", params.startDate || params.start_date);
  }
  if (typeof params.endDate === "string" || typeof params.end_date === "string") {
    search.set("endDate", params.endDate || params.end_date);
  }
  search.set("order", String(params.order || "_score_desc"));
  search.set("page", String(pageNumber));
  search.set("viewType", String(params.viewType || "material"));
  search.set("field", String(params.field || "all"));
  search.set("accurateSearch", String(params.accurateSearch ?? 1));
  search.set("isSearchAiScene", String(params.isSearchAiScene ?? params.is_search_ai_scene ?? 0));
  applyAppGrowingMaterialFilters(search, params);
  return url.toString();
}

function applyAppGrowingMaterialFilters(search, params) {
  appendSearchParamList(search, "media", firstDefined(params.media, params.media_ids));
  appendSearchParamList(search, "area", firstDefined(params.area, params.areas, params.regions));
  appendSearchParamList(search, "language", firstDefined(params.language, params.languages));
  appendSearchParamList(search, "platform", firstDefined(params.platform, params.platforms, params.device, params.devices));
  appendSearchParamList(search, "format", firstDefined(params.format, params.formats));
  appendSearchParamList(search, "creativeType", firstDefined(params.creativeType, params.creative_type, params.creative_types));
}

function firstDefined(...values) {
  return values.find((value) => value !== undefined && value !== null);
}

function appendSearchParamList(search, key, value) {
  const values = normalizeSearchParamList(value);
  if (values.length === 0) {
    return;
  }
  search.set(key, values.join(","));
}

function normalizeSearchParamList(value) {
  if (value === undefined || value === null) {
    return [];
  }
  const raw = Array.isArray(value) ? value : String(value).split(",");
  return raw
    .map((item) => String(item ?? "").trim())
    .filter(Boolean);
}

function appGrowingCompetitorURL(connector, competitor, params) {
  const maps = [
    params.competitor_urls,
    params.competitor_brand_urls,
    params.brand_urls,
  ];
  for (const map of maps) {
    const value = lookupCompetitorParam(map, competitor);
    if (value) {
      return appGrowingURLFromParam(connector, value);
    }
  }
  return null;
}

function lookupCompetitorParam(map, competitor) {
  if (!map || typeof map !== "object" || Array.isArray(map)) {
    return "";
  }
  if (typeof map[competitor] === "string") {
    return map[competitor];
  }
  const wanted = normalizeCompetitorKey(competitor);
  for (const [key, value] of Object.entries(map)) {
    if (normalizeCompetitorKey(key) === wanted && typeof value === "string") {
      return value;
    }
  }
  return "";
}

function appGrowingURLFromParam(connector, value) {
  let url;
  try {
    url = new URL(String(value || "").trim(), connector.probeURL);
  } catch {
    throw userError("material_search competitor_urls must contain valid AppGrowing URLs", 400);
  }
  const allowed = new URL(connector.probeURL);
  if (url.origin !== allowed.origin || !(url.pathname === "/leaflet" || url.pathname.endsWith("/leaflet"))) {
    throw userError("material_search competitor_urls must point to AppGrowing leaflet pages", 400);
  }
  return url;
}

function normalizeCompetitors(value) {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) => {
      if (typeof item === "string") {
        return item.trim();
      }
      if (item && typeof item === "object" && typeof item.name === "string") {
        return item.name.trim();
      }
      return "";
    })
    .filter(Boolean)
    .slice(0, 30);
}

function normalizeCompetitorKey(value) {
  return String(value || "").trim().toLowerCase();
}

function positiveIntegerParam(value, fallback, min, max) {
  const number = Number(value);
  if (!Number.isInteger(number)) {
    return fallback;
  }
  return Math.max(min, Math.min(max, number));
}

function finiteNumberParam(value, fallback, min, max) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return fallback;
  }
  return Math.max(min, Math.min(max, number));
}

function extractAppGrowingMaterials(value) {
  const out = [];
  const seen = new Set();
  collectMaterialObjects(value, out, seen);
  return out;
}

function collectMaterialObjects(value, out, seen) {
  if (!value || typeof value !== "object") {
    return;
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      collectMaterialObjects(item, out, seen);
    }
    return;
  }
  const nestedMaterial = firstObjectAtAnyKey(value, ["material"]);
  if (nestedMaterial && nestedMaterial !== value) {
    collectMaterialObjects(nestedMaterial, out, seen);
    for (const [key, child] of Object.entries(value)) {
      if (key !== "material") {
        collectMaterialObjects(child, out, seen);
      }
    }
    return;
  }
  const normalized = normalizeAppGrowingMaterial(value);
  if (normalized) {
    const key = normalized.resource_url || normalized.material_id || JSON.stringify(normalized);
    if (!seen.has(key)) {
      seen.add(key);
      out.push(normalized);
    }
  }
  for (const child of Object.values(value)) {
    collectMaterialObjects(child, out, seen);
  }
}

function normalizeAppGrowingMaterial(item) {
  const durationDays = numberAtAnyKey(item, [
    "duration",
    "duration_days",
    "delivery_days",
    "put_days",
    "running_days",
    "online_days",
    "days_count",
  ]);
  const impression = numberAtAnyKey(item, [
    "impression_inc_2y",
    "impression",
    "impressions",
    "impression_count",
    "impression_num",
    "cnt_dt",
    "show_count",
    "exposure",
    "exposure_count",
    "impression_estimate",
    "estimated_impression",
    "estimated_exposure",
  ]);
  const creative = firstObjectAtAnyKey(item, ["creative", "creative_info", "material", "resource_info"]);
  const campaign = firstObjectAtAnyKey(item, ["campaign", "appBrand", "application", "homePage"]);
  const resources = appGrowingResourceCandidates(item, creative);
  const resource = resources[0] || {};
  const resourceURL = normalizeAppGrowingResourceURL(
    stringAtAnyKeyOf(resources, [
      "path",
      "url",
      "src",
      "download_url",
      "downloadUrl",
      "file_url",
      "fileUrl",
      "cdn_url",
      "cdnUrl",
      "image_url",
      "imageUrl",
      "video_url",
      "videoUrl",
    ])
    || stringAtAnyKey(item, ["resource_path", "resource_url", "material_url", "creative_url"])
    || stringAtAnyKey(creative || {}, ["path", "url", "src", "resource_path", "resource_url", "material_url", "creative_url"]),
  );
  const posterURL = normalizeAppGrowingResourceURL(
    stringAtAnyKeyOf(resources, [
      "poster",
      "poster_url",
      "posterUrl",
      "cover",
      "cover_url",
      "coverUrl",
      "cover_pic_url",
      "coverPicUrl",
      "snapshot",
      "screenshot",
      "thumb",
      "thumb_url",
      "thumbUrl",
    ])
    || stringAtAnyKey(item, ["poster", "poster_url", "cover", "cover_url", "cover_pic_url", "snapshot"])
    || stringAtAnyKey(creative || {}, ["poster", "poster_url", "cover", "cover_url", "cover_pic_url", "snapshot"]),
  );
  const materialID = stringAtAnyKey(item, ["id", "material_id", "creative_id", "ad_id"]);
  const title = stringAtAnyKey(item, ["title", "name", "app_name", "brand_name"])
    || stringAtAnyKey(creative || {}, ["slogan", "description", "title", "name"])
    || stringAtAnyKey(campaign || {}, ["name", "title"]);
  const resourceFormat = stringAtAnyKey(resource, ["format", "type", "mime_type", "mimeType"]);
  const assetType = inferAppGrowingAssetType(resourceFormat, resourceURL);
  const media = appGrowingEntityList(item, ["media"]);
  const platforms = appGrowingEntityList(item, ["platform"]);
  const areas = appGrowingEntityList(item, ["area"]);
  const languages = appGrowingEntityList(item, ["language"]);

  if (!Number.isFinite(durationDays) && !Number.isFinite(impression)) {
    return null;
  }
  if (!resourceURL && !posterURL && !materialID && !title) {
    return null;
  }

  return {
    material_id: materialID || "",
    title,
    duration_days: Number.isFinite(durationDays) ? durationDays : null,
    impression_estimate: Number.isFinite(impression) ? impression : null,
    asset_type: assetType,
    preview_url: resourceURL || posterURL || "",
    resource_url: resourceURL || "",
    poster_url: posterURL || "",
    resource_format: resourceFormat,
    width: numberAtAnyKey(resource, ["width", "w"]),
    height: numberAtAnyKey(resource, ["height", "h"]),
    media_ids: entityValues(media, ["id"]),
    media_names: entityValues(media, ["name"]),
    platform_ids: entityValues(platforms, ["id"]),
    platform_names: entityValues(platforms, ["name"]),
    area_codes: entityValues(areas, ["cc", "code"]),
    area_names: entityValues(areas, ["name"]),
    language_codes: entityValues(languages, ["code"]),
    language_names: entityValues(languages, ["name"]),
    landing_url: stringAtAnyKey(item, ["landing_url", "redirect_url", "target_url"])
      || stringAtAnyKey(firstObjectAtAnyKey(item, ["landingPage", "landing_page"]), ["link", "url"]),
  };
}

function inferAppGrowingAssetType(resourceFormat, resourceURL) {
  const text = `${resourceFormat || ""} ${resourceURL || ""}`.toLowerCase();
  if (/(video|mp4|mov|webm|m3u8)(?:\b|[.?&#/])/.test(text)) {
    return "video";
  }
  if (/(image|jpeg|jpg|png|gif|webp|bmp|svg)(?:\b|[.?&#/])/.test(text)) {
    return "image";
  }
  return "";
}

function appGrowingEntityList(value, keys) {
  const out = [];
  for (const key of keys) {
    addObjectCandidates(value?.[key], out);
  }
  return out;
}

function entityValues(items, keys) {
  const seen = new Set();
  const out = [];
  for (const item of items) {
    if (!item || typeof item !== "object") {
      continue;
    }
    for (const key of keys) {
      const value = item[key];
      if (value === undefined || value === null || value === "") {
        continue;
      }
      const normalized = typeof value === "number" ? value : String(value).trim();
      if (normalized === "" || seen.has(normalized)) {
        continue;
      }
      seen.add(normalized);
      out.push(normalized);
      break;
    }
  }
  return out;
}

function appGrowingResourceCandidates(item, creative) {
  const out = [];
  addObjectCandidates(valueAtPath(item, ["creative", "resource"]), out);
  addObjectCandidates(valueAtPath(item, ["material", "creative", "resource"]), out);
  addObjectCandidates(valueAtPath(item, ["material", "resource"]), out);
  addObjectCandidates(valueAtPath(item, ["resource"]), out);
  addObjectCandidates(valueAtPath(item, ["resource_info"]), out);
  addObjectCandidates(valueAtPath(creative || {}, ["resource"]), out);
  addObjectCandidates(valueAtPath(creative || {}, ["resource_info"]), out);
  return out;
}

function selectAppGrowingMaterials(materials, rules, totalLimit, options = {}) {
  const unique = uniqueMaterials(materials);
  const newCandidates = unique
    .filter((item) =>
      matchesUpperBound(item.duration_days, rules.new_materials, "duration_days")
      && matchesLowerBound(item.impression_estimate, rules.new_materials, "impression"),
    )
    .sort(sortNewMaterial);
  const volumeCandidates = unique
    .filter((item) =>
      matchesLowerBound(item.duration_days, rules.volume_materials, "duration_days")
      && matchesLowerBound(item.impression_estimate, rules.volume_materials, "impression"),
    )
    .sort(sortVolumeMaterial);
  const newLimit = Math.round(totalLimit * Number(rules.new_materials.ratio));
  const volumeLimit = Math.max(0, totalLimit - newLimit);
  const selected = [];
  const selectedKeys = new Set();

  takeCandidates(newCandidates, newLimit, "new", selected, selectedKeys);
  takeCandidates(volumeCandidates, volumeLimit, "volume", selected, selectedKeys);
  if (selected.length < totalLimit) {
    takeCandidates([...newCandidates, ...volumeCandidates], totalLimit - selected.length, "fallback", selected, selectedKeys);
  }
  if (selected.length === 0 && options.fallbackToTopMaterials === true) {
    takeCandidates([...unique].sort(sortTopMaterial), totalLimit, "other", selected, selectedKeys);
  }

  return {
    unique,
    newCandidates,
    volumeCandidates,
    selected,
  };
}

function matchesUpperBound(value, rule, prefix) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return false;
  }
  const lt = rule[`${prefix}_lt`];
  if (Number.isFinite(lt) && !(number < lt)) {
    return false;
  }
  const lte = rule[`${prefix}_lte`];
  if (Number.isFinite(lte) && !(number <= lte)) {
    return false;
  }
  return Number.isFinite(lt) || Number.isFinite(lte);
}

function matchesLowerBound(value, rule, prefix) {
  const number = Number(value);
  if (!Number.isFinite(number)) {
    return false;
  }
  const gt = rule[`${prefix}_gt`];
  if (Number.isFinite(gt) && !(number > gt)) {
    return false;
  }
  const gte = rule[`${prefix}_gte`];
  if (Number.isFinite(gte) && !(number >= gte)) {
    return false;
  }
  return Number.isFinite(gt) || Number.isFinite(gte);
}

function materialRuleSummary(rule) {
  const out = {};
  for (const key of [
    "ratio",
    "duration_days_lt",
    "duration_days_lte",
    "duration_days_gt",
    "duration_days_gte",
    "impression_gt",
    "impression_gte",
  ]) {
    if (Number.isFinite(rule[key])) {
      out[key] = rule[key];
    }
  }
  return out;
}

function uniqueMaterials(materials) {
  const seen = new Set();
  const out = [];
  for (const material of materials) {
    const key = material.resource_url || material.material_id || `${material.competitor}:${material.title}:${material.duration_days}:${material.impression_estimate}`;
    if (!key || seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(material);
  }
  return out;
}

function takeCandidates(candidates, limit, bucket, selected, selectedKeys) {
  let remaining = limit;
  for (const candidate of candidates) {
    if (remaining <= 0) {
      break;
    }
    const key = candidate.resource_url || candidate.material_id;
    if (key && selectedKeys.has(key)) {
      continue;
    }
    if (key) {
      selectedKeys.add(key);
    }
    selected.push({
      bucket: bucket === "fallback" ? inferMaterialBucket(candidate) : bucket,
      competitor: candidate.competitor,
      priority_competitor: candidate.priority === true,
      material_id: candidate.material_id,
      title: candidate.title,
      duration_days: candidate.duration_days,
      impression_estimate: candidate.impression_estimate,
      asset_type: candidate.asset_type,
      preview_url: candidate.preview_url,
      resource_url: candidate.resource_url,
      poster_url: candidate.poster_url,
      resource_format: candidate.resource_format,
      width: candidate.width,
      height: candidate.height,
      media_ids: candidate.media_ids,
      media_names: candidate.media_names,
      platform_ids: candidate.platform_ids,
      platform_names: candidate.platform_names,
      area_codes: candidate.area_codes,
      area_names: candidate.area_names,
      language_codes: candidate.language_codes,
      language_names: candidate.language_names,
      landing_url: candidate.landing_url,
    });
    if (selected.length >= 500) {
      break;
    }
    remaining -= 1;
  }
}

function inferMaterialBucket(item) {
  if (Number(item.duration_days) < 7) {
    return "new";
  }
  if (Number(item.duration_days) > 30) {
    return "volume";
  }
  return "other";
}

function sortNewMaterial(left, right) {
  return priorityWeight(right) - priorityWeight(left)
    || Number(right.impression_estimate || 0) - Number(left.impression_estimate || 0)
    || Number(left.duration_days || 0) - Number(right.duration_days || 0);
}

function sortVolumeMaterial(left, right) {
  return priorityWeight(right) - priorityWeight(left)
    || Number(right.impression_estimate || 0) - Number(left.impression_estimate || 0)
    || Number(right.duration_days || 0) - Number(left.duration_days || 0);
}

function sortTopMaterial(left, right) {
  return priorityWeight(right) - priorityWeight(left)
    || Number(right.impression_estimate || 0) - Number(left.impression_estimate || 0)
    || Number(right.duration_days || 0) - Number(left.duration_days || 0);
}

function priorityWeight(item) {
  return item.priority === true ? 1 : 0;
}

function objectAtPath(value, path) {
  let current = value;
  for (const key of path) {
    if (!current || typeof current !== "object" || !(key in current)) {
      return null;
    }
    current = current[key];
  }
  return current && typeof current === "object" && !Array.isArray(current) ? current : null;
}

function firstObjectAtAnyKey(value, keys) {
  if (!value || typeof value !== "object") {
    return null;
  }
  for (const key of keys) {
    const child = value[key];
    const object = firstObjectFromValue(child);
    if (object) {
      return object;
    }
  }
  return null;
}

function objectAtAnyKey(value, keys) {
  if (!value || typeof value !== "object") {
    return null;
  }
  for (const key of keys) {
    const child = value[key];
    if (child && typeof child === "object" && !Array.isArray(child)) {
      return child;
    }
  }
  return null;
}

function firstObjectFromValue(value) {
  if (!value || typeof value !== "object") {
    return null;
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      const object = firstObjectFromValue(item);
      if (object) {
        return object;
      }
    }
    return null;
  }
  return value;
}

function addObjectCandidates(value, out) {
  if (!value || typeof value !== "object") {
    return;
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      addObjectCandidates(item, out);
    }
    return;
  }
  out.push(value);
}

function stringAtAnyKey(value, keys) {
  if (!value || typeof value !== "object") {
    return "";
  }
  for (const key of keys) {
    const raw = value[key];
    if (typeof raw === "string" && raw.trim() !== "") {
      return raw.trim();
    }
  }
  return "";
}

function stringAtAnyKeyOf(values, keys) {
  for (const value of values) {
    const text = stringAtAnyKey(value, keys);
    if (text) {
      return text;
    }
  }
  return "";
}

function normalizeAppGrowingResourceURL(value) {
  const text = String(value || "").trim();
  if (!text) {
    return "";
  }
  if (text.startsWith("//")) {
    return `https:${text}`;
  }
  return text;
}

function numberAtAnyKey(value, keys) {
  if (!value || typeof value !== "object") {
    return NaN;
  }
  const aliases = new Set(keys.map(normalizeKey));
  const queue = [{ value, depth: 0 }];
  const seen = new Set();
  while (queue.length > 0) {
    const current = queue.shift();
    if (!current.value || typeof current.value !== "object" || seen.has(current.value)) {
      continue;
    }
    seen.add(current.value);
    for (const [key, child] of Object.entries(current.value)) {
      const normalizedKey = normalizeKey(key);
      if (matchesNumericAlias(normalizedKey, aliases)) {
        const number = parseNumericValue(child);
        if (Number.isFinite(number)) {
          return number;
        }
      }
      if (child && typeof child === "object" && current.depth < 4) {
        queue.push({ value: child, depth: current.depth + 1 });
      }
    }
  }
  return NaN;
}

function matchesNumericAlias(key, aliases) {
  if (aliases.has(key)) {
    return true;
  }
  for (const alias of aliases) {
    if (alias.length >= 8 && (key.endsWith(alias) || key.includes(alias))) {
      return true;
    }
  }
  return false;
}

function summarizeMaterialSamples(materials) {
  return materials.slice(0, 5).map((material) => ({
    competitor: material.competitor,
    material_id: material.material_id,
    title: material.title,
    duration_days: material.duration_days,
    impression_estimate: material.impression_estimate,
    asset_type: material.asset_type,
    preview_url: material.preview_url,
    resource_url: material.resource_url,
  }));
}

function numericFromObject(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return NaN;
  }
  for (const key of ["value", "count", "total", "estimate", "estimated", "min", "max", "num"]) {
    const number = parseNumericValue(value[key]);
    if (Number.isFinite(number)) {
      return number;
    }
  }
  return NaN;
}

function parseNumericValue(value) {
  if (typeof value === "number") {
    return Number.isFinite(value) ? value : NaN;
  }
  if (value && typeof value === "object") {
    return numericFromObject(value);
  }
  if (typeof value !== "string") {
    return NaN;
  }
  const normalized = value
    .trim()
    .replaceAll(",", "")
    .replace(/\s+/g, "")
    .toLowerCase();
  if (normalized === "") {
    return NaN;
  }
  const match = normalized.match(/(-?\d+(?:\.\d+)?)(k|m|b|万|億|亿)?/u);
  if (!match) {
    return NaN;
  }
  const base = Number.parseFloat(match[1]);
  if (!Number.isFinite(base)) {
    return NaN;
  }
  const multiplier = {
    k: 1_000,
    m: 1_000_000,
    b: 1_000_000_000,
    "万": 10_000,
    "億": 100_000_000,
    "亿": 100_000_000,
  }[match[2] || ""] || 1;
  return base * multiplier;
}

async function runCrawl(body) {
  assertSafeParams(body.params || {});
  const connector = connectorForID(body.connector_id);
  const capability = String(body.capability || "profile_verify").trim();
  if (!connector.capabilities.includes(capability)) {
    throw userError(`connector ${connector.id} does not support capability ${capability}`, 400);
  }
  const stored = await readCredentialState(body.profile_id);
  const storageState = stored.storage_state;
  const sessionStorageState = stored.session_storage || {};
  if (!storageState || !Array.isArray(storageState.cookies)) {
    throw new Error("credential state is not a Playwright storageState");
  }
  if (!hasConnectorState(storageState, connector)) {
    throw new Error(`stored credential state does not contain ${connector.id} browser state; bind again`);
  }
  const credentialState = summarizeCredentialState(storageState, connector, sessionStorageState);
  let authCheck = { authenticated: false, method: "not_checked" };
  const browser = await chromium.launch({
    headless: crawlerHeadless(connector),
    args: ["--disable-blink-features=AutomationControlled"],
  });
  try {
    const context = await browser.newContext({ storageState });
    await restoreSessionStorage(context, sessionStorageState);
    const page = await context.newPage();
    attachAuthWatcher(page, connector, (latestAuthCheck) => {
      authCheck = latestAuthCheck;
    });
    const targetURL = connectorTargetURL(connector, body.params?.url || connector.probeURL || connector.loginURL);
    const waitForAuth = waitForNextAuthCheck(page, connector, 20_000);
    const response = await page.goto(targetURL, { waitUntil: "domcontentloaded", timeout: 60_000 });
    authCheck = await waitForAuth || authCheck;
    const title = await page.title().catch(() => "");
    const currentURL = page.url();
    if (!authCheck.authenticated) {
      authCheck = await verifyConnectorPageAuth(page, connector).catch(async () =>
        verifyConnectorAuth(context, connector),
      );
    }
    const loginDetected = isLoginURL(currentURL, connector);
    if (!authCheck.authenticated && !loginDetected) {
      authCheck = browserStateAuthFallback(authCheck, "browser_state_non_login_url");
    }
    const needsReauth = !authCheck.authenticated || loginDetected;
    const authProbe = {
      url: currentURL,
      title,
      http_status: response ? response.status() : null,
      login_detected: loginDetected,
      auth_check: authCheck,
    };
    if (!needsReauth && capability === "material_search") {
      if (connector.id !== "appgrowing") {
        throw userError(`connector ${connector.id} has no material_search executor`, 400);
      }
      const materialResult = await runAppGrowingMaterialSearch(page, connector, {
        ...(body.params || {}),
        profile_id: body.profile_id,
      });
      await context.close();
      return {
        ...materialResult,
        raw: {
          ...materialResult.raw,
          credential_state: credentialState,
          auth_probe: authProbe,
        },
      };
    }
    let extracted = {};
    if (!needsReauth && capability === "page_extract") {
      await runDeclarativePageSteps(page, body.params?.steps);
      extracted = await extractDeclarativePageData(page, body.params);
    }
    await context.close();
    const profileVerify = capability === "profile_verify";
    return {
      status: needsReauth ? "need_reauth" : "completed",
      downloaded: 0,
      output_prefix: `local://credential-broker/${body.profile_id || "unknown"}`,
      message: needsReauth
        ? "stored browser state reached a login page; re-authentication is required"
        : profileVerify
          ? `stored browser state verified for ${connector.id}`
          : `page_extract completed for ${connector.id}`,
      raw: {
        connector_id: body.connector_id,
        capability: body.capability,
        credential_state: credentialState,
        auth_probe: authProbe,
        extracted,
      },
    };
  } finally {
    await browser.close().catch(() => {});
  }
}

function connectorTargetURL(connector, rawURL) {
  let parsed;
  try {
    parsed = new URL(String(rawURL || "").trim());
  } catch {
    throw userError("crawl params.url must be a valid URL", 400);
  }
  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw userError("crawl params.url must use HTTP or HTTPS", 400);
  }
  const domains = connector.allowedTargetDomains?.length
    ? connector.allowedTargetDomains
    : connector.stateDomains;
  const hostname = parsed.hostname.toLowerCase();
  const allowed = domains.some((domain) => {
    const normalized = String(domain || "").trim().toLowerCase().replace(/^\*\./, "");
    return normalized && (hostname === normalized || hostname.endsWith(`.${normalized}`));
  });
  if (!allowed) {
    throw userError(`crawl target host ${hostname} is not allowed for connector ${connector.id}`, 400);
  }
  return parsed.toString();
}

async function runDeclarativePageSteps(page, rawSteps) {
  if (!Array.isArray(rawSteps)) {
    return;
  }
  if (rawSteps.length > 30) {
    throw userError("page_extract steps cannot exceed 30", 400);
  }
  for (const rawStep of rawSteps) {
    if (!rawStep || typeof rawStep !== "object" || Array.isArray(rawStep)) {
      throw userError("page_extract steps must be objects", 400);
    }
    const action = String(rawStep.action || "").trim();
    const selector = String(rawStep.selector || "").trim();
    if (selector.length > 300) {
      throw userError("page_extract step selector is too long", 400);
    }
    switch (action) {
      case "click":
        await page.locator(selector).first().click({ timeout: 10_000 });
        break;
      case "fill":
        await page.locator(selector).first().fill(String(rawStep.value || ""), { timeout: 10_000 });
        break;
      case "select":
        await page.locator(selector).first().selectOption(String(rawStep.value || ""), { timeout: 10_000 });
        break;
      case "wait_for":
        await page.locator(selector).first().waitFor({ state: "visible", timeout: 15_000 });
        break;
      case "press":
        await page.locator(selector).first().press(String(rawStep.key || "Enter"), { timeout: 10_000 });
        break;
      case "scroll": {
        const deltaY = Math.max(-5000, Math.min(5000, Number(rawStep.delta_y) || 800));
        await page.mouse.wheel(0, deltaY);
        break;
      }
      default:
        throw userError(`unsupported page_extract action: ${action || "<empty>"}`, 400);
    }
  }
}

async function extractDeclarativePageData(page, params) {
  const extracted = {};
  const fields = Array.isArray(params?.extract) ? params.extract.slice(0, 50) : [];
  for (const [index, field] of fields.entries()) {
    if (!field || typeof field !== "object" || Array.isArray(field)) {
      continue;
    }
    const selector = String(field.selector || "").trim();
    if (!selector || selector.length > 300) {
      continue;
    }
    const name = String(field.name || `field_${index + 1}`).trim().slice(0, 100);
    const attribute = String(field.attribute || "text").trim();
    const locator = page.locator(selector);
    const count = field.all === true ? Math.min(await locator.count(), 200) : Math.min(await locator.count(), 1);
    const values = [];
    for (let itemIndex = 0; itemIndex < count; itemIndex += 1) {
      const item = locator.nth(itemIndex);
      let value;
      if (attribute === "text") {
        value = await item.textContent({ timeout: 3000 }).catch(() => null);
      } else if (["href", "src", "title", "alt", "value"].includes(attribute)) {
        value = await item.getAttribute(attribute, { timeout: 3000 }).catch(() => null);
      } else {
        throw userError(`unsupported page_extract attribute: ${attribute}`, 400);
      }
      values.push(typeof value === "string" ? value.trim() : value);
    }
    extracted[name] = field.all === true ? values : (values[0] ?? null);
  }
  if (fields.length === 0 && Array.isArray(params?.selectors)) {
    for (const selector of params.selectors.slice(0, 20)) {
      if (typeof selector !== "string" || selector.length > 200) {
        continue;
      }
      extracted[selector] = await page.locator(selector).first().textContent({ timeout: 3000 }).catch(() => null);
    }
  }
  return extracted;
}

function crawlerHeadless(connector) {
  if (process.env.CRAWLER_WORKER_CRAWL_HEADLESS === "true") {
    return true;
  }
  if (process.env.CRAWLER_WORKER_CRAWL_HEADLESS === "false") {
    return false;
  }
  return connector.crawlHeadlessDefault !== false;
}

const server = http.createServer(async (req, res) => {
  try {
    const url = new URL(req.url || "/", publicURL);

    if (req.method === "GET" && (url.pathname === "/healthz" || url.pathname === "/readyz")) {
      writeJSON(res, 200, {
        ok: true,
        service: "crawler-worker",
        playwright: true,
        remote_ui: remoteBrowserUI,
      });
      return;
    }

    if (req.method === "POST" && url.pathname === "/login-sessions") {
      const body = await readJSON(req);
      if (!body.profile_id || !body.connector_id || !body.session_token) {
        writeJSON(res, 400, { error: "profile_id, connector_id and session_token are required" });
        return;
      }
      const connector = connectorForID(body.connector_id);
      activeSessions.set(body.session_token, {
        profileID: body.profile_id,
        connectorID: body.connector_id,
        loginURL: connector.loginURL || body.login_url,
        status: "pending",
        createdAt: Date.now(),
        expiresAt: Date.now() + loginSessionTTLMS,
        browser: null,
        context: null,
        page: null,
        viewport: sessionViewport,
        error: "",
      });
      writeJSON(res, 200, {
        browser_url: `${publicURL}/sessions/${encodeURIComponent(body.session_token)}`,
        expires_in_seconds: Math.floor(loginSessionTTLMS / 1000),
      });
      return;
    }

    const sessionPath = parseSessionPath(url.pathname);
    if (sessionPath) {
      const session = getSession(sessionPath.token);
      if (!session) {
        writeJSON(res, 404, { error: "login session not found in worker; start a new binding session" });
        return;
      }
      if (req.method === "GET" && sessionPath.action === "") {
        res.writeHead(200, {
          "content-type": "text/html; charset=utf-8",
          "cache-control": "no-store",
        });
        res.end(sessionHTML(sessionPath.token, session));
        return;
      }
      if (req.method === "GET" && sessionPath.action === "status") {
        writeJSON(res, 200, await sessionDetails(session));
        return;
      }
      if (req.method === "POST" && sessionPath.action === "open") {
        writeJSON(res, 200, await openControlledBrowser(session, sessionPath.token));
        return;
      }
      if (req.method === "POST" && sessionPath.action === "complete") {
        writeJSON(res, 200, await completeSession(session, sessionPath.token));
        return;
      }
      if (req.method === "GET" && sessionPath.action === "screenshot") {
        writeJSON(res, 200, await sessionScreenshot(session));
        return;
      }
      if (req.method === "GET" && sessionPath.action === "stream") {
        await streamSession(session, req, res);
        return;
      }
      if (req.method === "POST" && sessionPath.action === "input") {
        writeJSON(res, 200, await enqueueSessionInput(session, await readJSON(req)));
        return;
      }
    }

    if (req.method === "POST" && url.pathname === "/crawl") {
      writeJSON(res, 200, await runCrawl(await readJSON(req)));
      return;
    }

    writeJSON(res, 404, { error: "not found" });
  } catch (err) {
    writeJSON(res, err.statusCode || 500, { error: err instanceof Error ? err.message : String(err) });
  }
});

export {
  appGrowingMaterialURL,
  connectorForID,
  connectorTargetURL,
  extractAppGrowingMaterials,
  normalizeDeclarativeConnector,
  normalizeAppGrowingMaterial,
};

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  server.listen(port, host, () => {
    console.log(`crawler-worker listening on ${publicURL}`);
    console.log(`completion callback target: ${apiURL}/api/credential-login-sessions/complete`);
    console.log(`remote browser UI: ${remoteBrowserUI ? "enabled" : "disabled"}`);
    if (databaseURL) {
      console.log("credential DB store enabled");
    } else {
      console.log("DATABASE_URL not set; crawl cannot read stored credential state");
    }
  });
}
