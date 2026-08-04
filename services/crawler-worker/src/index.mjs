import crypto from "node:crypto";
import fs from "node:fs";
import http from "node:http";
import { Buffer } from "node:buffer";
import { pathToFileURL } from "node:url";
import { chromium } from "playwright";
import pg from "pg";
import { createBrowserCapacity } from "./browser-capacity.mjs";
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
const browserProxyServer = String(process.env.CRAWLER_WORKER_BROWSER_PROXY || "").trim();
const sessionViewport = {
  width: positiveIntegerEnv("CRAWLER_WORKER_VIEWPORT_WIDTH", 1440),
  height: positiveIntegerEnv("CRAWLER_WORKER_VIEWPORT_HEIGHT", 1000),
};
const streamFrameIntervalMS = Math.max(
  100,
  Math.min(1000, positiveIntegerEnv("CRAWLER_WORKER_STREAM_FRAME_MS", 180)),
);
const appGrowingMaterialSearchBudgetMS = positiveIntegerEnv("APPGROWING_MATERIAL_SEARCH_BUDGET_MS", 12 * 60_000);
const appGrowingGraphQLTimeoutMS = positiveIntegerEnv("APPGROWING_GRAPHQL_TIMEOUT_MS", 12_000);
const appGrowingBrowserFallbackMinBudgetMS = positiveIntegerEnv("APPGROWING_BROWSER_FALLBACK_MIN_BUDGET_MS", 75_000);

function chromiumLaunchOptions(headless) {
  return {
    headless,
    args: ["--disable-blink-features=AutomationControlled"],
    ...(browserProxyServer ? { proxy: { server: browserProxyServer } } : {}),
  };
}

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


const APPGROWING_SEARCH_APP_QUERY = `
query searchApp(
  $purpose: Int
  $keyword: String!
  $accurateSearch: Int
  $hadAdvert: Int
  $page: Int
) {
  searchAppBrand(
    purpose: $purpose
    keyword: $keyword
    accurateSearch: $accurateSearch
    hadAdvert: $hadAdvert
    page: $page
  ) {
    page
    limit
    total
    pageTotal
    data {
      appBrand {
        id
        name
        icon
        types
        developer {
          id
          name
        }
        bundle_id
        app_id
      }
      highlight
      hadAdvert
    }
  }
}
`;

const APPGROWING_APP_MATERIAL_LIST_QUERY = `
query appMaterialList(
  $purpose: Int!
  $startDate: LocalDate
  $endDate: LocalDate
  $isNew: Int
  $field: String
  $order: MaterialListSort!
  $page: Int
  $accurateSearch: Int
  $appBrand: String
) {
  materialList(
    purpose: $purpose
    startDate: $startDate
    endDate: $endDate
    isNew: $isNew
    field: $field
    order: $order
    page: $page
    accurateSearch: $accurateSearch
    appBrand: $appBrand
  ) {
    page
    total
    limit
    data {
      material {
        id
        type
        startDate
        endDate
        duration
        cnt_ad_id
        impression_inc_2y
        area {
          cc
          name
          icon
        }
        creative {
          id
          type
          slogan
          description
          txtUrl
          resource {
            width
            height
            format
            path
            poster
            duration
            id
          }
        }
        platform {
          id
          name
        }
        campaign {
          ... on App {
            id
            name
          }
          ... on AppBrand {
            id
            name
          }
          ... on Website {
            id
            name
          }
          ... on Playlet {
            id
            name
          }
          ... on Novel {
            id
            name
          }
        }
      }
    }
  }
}
`;

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

const browserCapacity = createBrowserCapacity(
  positiveIntegerEnv("CRAWLER_WORKER_MAX_OPEN_BROWSERS", 1),
);
const crawlerBrowserTimeoutMS = positiveIntegerEnv(
  "CRAWLER_WORKER_CRAWL_BROWSER_TIMEOUT_MS",
  15 * 60 * 1000,
);

function acquireBrowserSlot() {
  const release = browserCapacity.tryAcquire();
  if (!release) {
    throw userError("crawler worker is busy; wait for the active browser task to finish", 429);
  }
  return release;
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
  const releaseBrowserSlot = session.releaseBrowserSlot;
  session.browser = null;
  session.context = null;
  session.page = null;
  session.releaseBrowserSlot = null;
  if (browser) {
    await browser.close().catch(() => {});
  }
  releaseBrowserSlot?.();
}

async function closeRemoteBrowser(session) {
  await closeSessionBrowser(session);
  if (session.status !== "completed" && session.status !== "expired") {
    session.status = "pending";
    session.error = "";
    session.autoCompleting = false;
    session.autoCompleteReason = "";
  }
  return publicSession(session);
}

async function expireSession(token) {
  const session = activeSessions.get(token);
  if (!session || session.status === "completed" || Date.now() < session.expiresAt) {
    return;
  }
  session.status = "expired";
  session.error = "login session expired";
  await closeSessionBrowser(session);
}

function scheduleSessionExpiry(token, expiresAt) {
  const expireWhenDue = () => {
    const remaining = expiresAt - Date.now();
    if (remaining > 0) {
      setTimeout(expireWhenDue, remaining);
      return;
    }
    void expireSession(token);
  };
  setTimeout(expireWhenDue, Math.max(0, expiresAt - Date.now()));
}

function assertSessionNotExpired(session) {
  if (session.status === "expired" || Date.now() >= session.expiresAt) {
    throw userError("login session expired; start a new binding session", 410);
  }
}

async function openControlledBrowser(session, token) {
  if (session.status === "completed") {
    return publicSession(session);
  }
  assertSessionNotExpired(session);
  if (session.browser) {
    return publicSession(session);
  }
  const connector = connectorForID(session.connectorID);
  const headless = sessionBrowserHeadless(connector);
  session.status = "opening";
  session.error = "";
  const releaseBrowserSlot = acquireBrowserSlot();
  let browser;
  try {
    browser = await chromium.launch(chromiumLaunchOptions(headless));
    const context = await browser.newContext({
      viewport: session.viewport || sessionViewport,
      ...(connector.id === "appgrowing" ? { locale: "en" } : {}),
    });
    await context.addInitScript(() => {
      Object.defineProperty(navigator, "webdriver", {
        get: () => undefined,
      });
    });
    const page = await context.newPage();
    attachAuthWatcher(page, connector, (authCheck) => {
      session.authCheck = authCheck;
    });
    session.browser = browser;
    session.context = context;
    session.page = page;
    session.releaseBrowserSlot = releaseBrowserSlot;
    await page.goto(session.loginURL, { waitUntil: "domcontentloaded", timeout: 60_000 });
    session.status = "browser_open";
    return sessionDetails(session);
  } catch (error) {
    if (session.browser === browser) {
      session.browser = null;
      session.context = null;
      session.page = null;
      session.releaseBrowserSlot = null;
    }
    await browser?.close().catch(() => {});
    releaseBrowserSlot();
    session.status = "error";
    session.error = error instanceof Error ? error.message : String(error);
    throw error;
  }
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
  assertSessionNotExpired(session);
  session.error = "";
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
  let authCheck = await refreshSessionAuth(session, connector);
  const identityVerificationError = connectorAuthVerificationError(authCheck);
  if (identityVerificationError) {
    throw userError(`AppGrowing user verification failed: ${identityVerificationError}`);
  }
  if (!authCheck.authenticated) {
    throw userError("AppGrowing authenticated user was not detected; finish login in the controlled browser before completing");
  }
  const businessProbe = await verifyAppGrowingBusinessAccess(session.page || session.context, connector, authCheck);
  authCheck = businessProbe.authCheck;
  if (businessProbe.needsReauth) {
    throw userError("AppGrowing material access reports an expired login; finish login in the controlled browser before completing");
  }
  if (businessProbe.verificationError) {
    throw userError(`AppGrowing material access verification failed: ${businessProbe.verificationError}`);
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
      headers: connectorGraphQLHeaders(connector, check.payload?.operationName || ""),
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

function connectorGraphQLHeaders(connector, operationName = "") {
  const headers = {
    "content-type": "application/json",
  };
  if (operationName) {
    headers["x-operation-name"] = operationName;
  }
  if (connector.id === "appgrowing") {
    headers["accept-language"] = "en";
    headers.origin = "https://appgrowing-global.youcloud.com";
    headers.referer = "https://appgrowing-global.youcloud.com/";
    headers["user-agent"] = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121 Safari/537.36";
  }
  return headers;
}

function authCheckFromBody(body, connector, httpStatus, method) {
  const check = connector.authCheck;
  const userID = valueAtPath(body, check.userIDPath);
  const upstreamError = connector.id === "appgrowing" ? appGrowingGraphQLErrorMessage(body) : "";
  const userIDPresent = typeof userID === "string" ? userID.trim() !== "" : Boolean(userID);
  return {
    authenticated: userIDPresent && !upstreamError,
    method,
    http_status: httpStatus,
    user_id_present: userIDPresent,
    team_present: Boolean(valueAtPath(body, ["data", "userinfo", "teamInfo"])),
    plan_present: Boolean(valueAtPath(body, ["data", "userinfo", "purchasePlanInfo"])),
    upstream_error: upstreamError || undefined,
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
  if (pageAuthCheck?.authenticated) {
    session.authCheck = pageAuthCheck;
    return pageAuthCheck;
  }
  const pageCheck = await verifyConnectorPageAuth(session.page, connector).catch(() => pageAuthCheck);
  if (pageCheck?.authenticated) {
    session.authCheck = pageCheck;
    return pageCheck;
  }
  const workerCheck = await verifyConnectorAuth(session.context, connector);
  session.authCheck = workerCheck.authenticated ? workerCheck : (pageCheck || workerCheck);
  return session.authCheck;
}

function connectorAuthVerificationError(authCheck) {
  if (authCheck?.probe_error) {
    return String(authCheck.probe_error);
  }
  const upstreamError = String(authCheck?.upstream_error || "").trim();
  if (upstreamError && !/05:403005|login has expired|please log in again|account was logged out|session (?:has )?expired/i.test(upstreamError)) {
    return upstreamError;
  }
  const status = Number(authCheck?.http_status);
  if (Number.isFinite(status) && (status < 200 || status >= 300)) {
    return `userinfo verification returned HTTP ${status}`;
  }
  return "";
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

function unavailableSessionHTML() {
  return `<!doctype html>
<meta charset="utf-8">
<title>Multica Credential Broker</title>
<style>
html,body{height:100%}body{display:grid;place-items:center;margin:0;background:#151515;color:#f5f2ea;font:14px/1.5 ui-sans-serif,system-ui,-apple-system,Segoe UI,sans-serif}
.message{max-width:520px;padding:20px;border:1px solid #393939;background:#202020}.message strong{display:block;margin-bottom:6px;font-size:16px}
</style>
<div class="message"><strong>Browser session ended</strong>Close this window. Start a new binding only if the credential was not saved.</div>`;
}

function unavailableSessionImage() {
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720" viewBox="0 0 1280 720">
<rect width="1280" height="720" fill="#202020"/>
<text x="640" y="338" fill="#f5f2ea" font-family="Arial, sans-serif" font-size="30" font-weight="600" text-anchor="middle">Browser session ended</text>
<text x="640" y="386" fill="#b9b9b9" font-family="Arial, sans-serif" font-size="20" text-anchor="middle">Close this window. Start a new binding only if the credential was not saved.</text>
</svg>`;
}

function sessionHTML(token, session) {
  const statusURL = `/sessions/${encodeURIComponent(token)}/status`;
  const openURL = `/sessions/${encodeURIComponent(token)}/open`;
  const closeURL = `/sessions/${encodeURIComponent(token)}/close`;
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
.errorline{max-width:34vw;color:#8a1f1f;font-size:12px;line-height:1.35}
.retry{height:28px;border:1px solid #c9c9c9;border-radius:4px;padding:0 10px;background:#fff;color:#242424;font:600 12px ui-sans-serif,system-ui,-apple-system,Segoe UI,sans-serif;cursor:pointer;white-space:nowrap}
.retry:hover{background:#f4f4f4}.retry:disabled{cursor:default;opacity:.55}
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
    <span id="errorLine" class="errorline" hidden></span>
    <button id="retryButton" class="retry" type="button" hidden>Retry save</button>
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
const errorLine = document.querySelector("#errorLine");
const retryButton = document.querySelector("#retryButton");
const scrollbar = document.querySelector("#scrollbar");
const scrollThumb = document.querySelector("#scrollThumb");
const urls = {
  status: ${JSON.stringify(statusURL)},
  open: ${JSON.stringify(openURL)},
  close: ${JSON.stringify(closeURL)},
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
let sessionUnavailable = false;

function closeRemoteBrowser() {
  if (completed) {
    return;
  }
  if (navigator.sendBeacon) {
    navigator.sendBeacon(urls.close, new Blob(["{}"], {type: "application/json"}));
    return;
  }
  fetch(urls.close, {method: "POST", keepalive: true}).catch(() => {});
}

window.addEventListener("pagehide", closeRemoteBrowser);

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
  const error = body.error || "";
  errorLine.textContent = error;
  errorLine.hidden = !error;
  retryButton.hidden = !error;
  retryButton.disabled = completeBusy;
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

  if (!completed && !body.error && body.auth_check?.authenticated) {
    autoComplete("auth_detected", false);
    return;
  }
  if (!completed && !body.error && looksReadyForCompletion(url)) {
    autoComplete("app_page_detected", false);
  }
}

function stopRemoteSession(message, body = {}) {
  sessionUnavailable = true;
  streamStarted = false;
  browserOpen = false;
  completed = true;
  if (polling) {
    window.clearInterval(polling);
    polling = 0;
  }
  shot.onerror = null;
  shot.removeAttribute("src");
  shot.hidden = true;
  empty.hidden = false;
  empty.textContent = message;
  setStatus("Session ended", "error", "");
  debug({ status: "session_unavailable", ...body });
  window.parent?.postMessage({
    type: "multica:credential-session-unavailable",
    status: body.status || null,
    error: body.error || message,
  }, "*");
}

async function recoverStream() {
  const resp = await fetch(urls.status).catch(() => null);
  if (!resp) {
    if (!completed && !sessionUnavailable) {
      window.setTimeout(startStream, 1200);
    }
    return;
  }
  const body = await parseResponse(resp);
  if (!resp.ok) {
    stopRemoteSession(
      "This browser session has ended. Close this window and start a new binding only if needed.",
      { status: resp.status, error: body.error || body.raw || null },
    );
    return;
  }
  renderStatus(body);
  if (!completed && !sessionUnavailable) {
    window.setTimeout(startStream, 800);
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
  if (!body.status?.error && body.status?.auth_check?.authenticated) {
    autoComplete("auth_detected", false);
    return;
  }
  if (!body.status?.error && looksReadyForCompletion(body.current_url)) {
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
    if (!completed && !sessionUnavailable) {
      empty.hidden = false;
      empty.textContent = "Reconnecting remote browser...";
      recoverStream();
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
    } else if (resp) {
      const body = await parseResponse(resp);
      stopRemoteSession(
        "This browser session has ended. Close this window and start a new binding only if needed.",
        { status: resp.status, error: body.error || body.raw || null },
      );
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
  errorLine.hidden = true;
  retryButton.hidden = true;
  setStatus("Saving", "warn", urlLine.textContent);
  try {
    const resp = await fetch(urls.complete, {method: "POST"});
    const body = await parseResponse(resp);
    renderStatus(body);
    if (!resp.ok) {
      debug({ status: "action_needed", auto_complete_attempt: reason, error: body.error || body.raw || null });
      completeBusy = false;
      retryButton.disabled = false;
      return;
    }
    completed = true;
  } finally {
    completeBusy = false;
    retryButton.disabled = false;
  }
}

retryButton.addEventListener("click", () => autoComplete("manual_retry", true));

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

async function runAppGrowingMaterialSearch(page, context, connector, params = {}) {
  params = normalizeAppGrowingMaterialSearchParams(params);
  const competitors = normalizeCompetitors(params.competitors);
  if (competitors.length === 0) {
    throw userError("material_search params.competitors is required", 400);
  }
  const priorityCompetitors = new Set(
    normalizeCompetitors(params.priority_competitors).map(normalizeCompetitorKey),
  );
  const totalLimit = positiveIntegerParam(params.limit, 25, 1, 200);
  const pageLimit = positiveIntegerParam(params.pages_per_competitor, 3, 1, 5);
  const priorityPageLimit = positiveIntegerParam(params.priority_pages_per_competitor, Math.max(5, pageLimit), 1, 8);
  const novelOnly = params.novel_only === true;
  const maxPageLimit = novelOnly
    ? positiveIntegerParam(params.max_pages_per_competitor, Math.max(10, pageLimit), pageLimit, 30)
    : pageLimit;
  const maxPriorityPageLimit = novelOnly
    ? positiveIntegerParam(params.max_priority_pages_per_competitor, Math.max(15, priorityPageLimit), priorityPageLimit, 30)
    : priorityPageLimit;
  const excludedDedupeKeys = new Set(normalizeSearchParamList(params.exclude_dedupe_keys));
  const captureTimeoutMS = positiveIntegerParam(params.capture_timeout_ms, 12_000, 3_000, 60_000);
  const graphQLTimeoutMS = positiveIntegerParam(params.graphql_timeout_ms, appGrowingGraphQLTimeoutMS, 3_000, 30_000);
  const searchBudgetMS = positiveIntegerParam(
    params.material_search_budget_ms ?? params.crawl_budget_ms,
    appGrowingMaterialSearchBudgetMS,
    30_000,
    14 * 60_000,
  );
  const startedAt = Date.now();
  const deadline = startedAt + searchBudgetMS;
  const plannedPages = appGrowingPlannedMaterialPages(competitors, priorityCompetitors, pageLimit, priorityPageLimit);
  const browserFallbackEnabled = shouldUseAppGrowingBrowserFallback(params, plannedPages);
  const adaptiveMode = params.adaptive_mode !== false && params.adaptive_material_search !== false;
  const adaptiveMemory = appGrowingAdaptiveStrategyMemory(params);
  const rules = normalizeMaterialRules(params.selection_rules || params.rules);

  const captured = [];
  const materials = [];
  const autoAdjustments = [];
  const learnedStrategies = [];
  const learnedStrategyKeys = new Set();
  let timeBudgetExhausted = false;
  const markBudgetExhausted = (source, competitor = "", priority = false, pageNumber = null) => {
    timeBudgetExhausted = true;
    captured.push({
      competitor,
      priority,
      page: pageNumber,
      source,
      skipped: true,
      error: "material_search_time_budget_exhausted",
      remaining_budget_ms: appGrowingRemainingBudgetMS(deadline),
      materials_found: 0,
    });
  };

  if (params.graphql_material_search !== false) {
    const brandCache = new Map();
    graphqlSearch:
    for (let pageNumber = 1; pageNumber <= maxPriorityPageLimit; pageNumber += 1) {
      for (const competitor of competitors) {
        const priority = priorityCompetitors.has(normalizeCompetitorKey(competitor));
        const pages = priority ? maxPriorityPageLimit : maxPageLimit;
        if (pageNumber > pages) {
          continue;
        }
        if (!appGrowingHasBudget(deadline, Math.min(5_000, graphQLTimeoutMS))) {
          markBudgetExhausted("graphql_api", competitor, priority, pageNumber);
          break graphqlSearch;
        }
        const capture = await captureAppGrowingMaterialPageViaGraphQL(context, connector, {
          page,
          competitor,
          pageNumber,
          params,
          brandCache,
          deadline,
          graphQLTimeoutMS,
        });
        captured.push({
          competitor,
          priority,
          page: pageNumber,
          source: "graphql_api",
          url: capture.url,
          app_brand_id: capture.appBrandID,
          app_brand_name: capture.appBrandName,
          app_brand_source: capture.appBrandSource,
          graphQL_operations: capture.operations,
          graphQL_responses: capture.responses,
          total: capture.total,
          limit: capture.limit,
          needs_reauth: capture.needsReauth === true,
          error: capture.error,
          materials_found: capture.materials.length,
        });
        if (capture.needsReauth) {
          break graphqlSearch;
        }
        if (capture.appBrandSource === "strategy_memory") {
          appGrowingAddAutoAdjustment(autoAdjustments, `used learned AppGrowing brand id for ${competitor}`);
        }
        if (capture.materials.length > 0) {
          appGrowingAddLearnedBrandStrategy(learnedStrategies, learnedStrategyKeys, competitor, capture, "graphql_api");
        }
        for (const material of capture.materials) {
          materials.push({
            ...material,
            competitor,
            priority,
          });
        }
      }
      if (pageNumber >= Math.max(pageLimit, priorityPageLimit)
        && appGrowingNovelTargetMet(materials, rules, totalLimit, excludedDedupeKeys)) {
        break;
      }
    }
  }

  const graphQLNeedsReauth = captured.some((capture) => capture.needs_reauth === true);
  const browserFallbackCompetitors = graphQLNeedsReauth || appGrowingNovelTargetMet(materials, rules, totalLimit, excludedDedupeKeys)
    ? []
    : appGrowingBrowserFallbackCompetitors(
      competitors,
      captured,
      params,
      adaptiveMemory,
      browserFallbackEnabled,
      adaptiveMode,
    );
  const adaptiveBrowserFallbackEnabled = adaptiveMode
    && !browserFallbackEnabled
    && browserFallbackCompetitors.length > 0;
  if (adaptiveBrowserFallbackEnabled) {
    appGrowingAddAutoAdjustment(
      autoAdjustments,
      `enabled browser fallback for ${browserFallbackCompetitors.join(", ")} after their AppGrowing API pages returned no material`,
    );
  }

  if (browserFallbackCompetitors.length > 0) {
    if (!appGrowingHasBudget(deadline, appGrowingBrowserFallbackMinBudgetMS)) {
      markBudgetExhausted("browser_network");
    } else {
      let browserCapturePage = page;
      let browserFallbackStopped = false;
      browserFallback:
      for (let pageNumber = 1; pageNumber <= maxPriorityPageLimit; pageNumber += 1) {
        for (const competitor of browserFallbackCompetitors) {
          if (browserFallbackStopped) {
            break browserFallback;
          }
          const priority = priorityCompetitors.has(normalizeCompetitorKey(competitor));
          const pages = priority ? maxPriorityPageLimit : maxPageLimit;
          if (pageNumber > pages) {
            continue;
          }
          if (!appGrowingHasBudget(deadline, appGrowingBrowserFallbackMinBudgetMS)) {
            markBudgetExhausted("browser_network", competitor, priority, pageNumber);
            break browserFallback;
          }
          const capture = await captureAppGrowingMaterialPage(browserCapturePage, connector, {
            competitor,
            pageNumber,
            params,
            captureTimeoutMS,
            pageTimeoutMS: appGrowingRequestTimeoutMS(deadline, 60_000),
          });
          captured.push({
            competitor,
            priority,
            page: pageNumber,
            source: "browser_network",
            url: capture.url,
            graphQL_operations: capture.operations,
            graphQL_responses: capture.responses,
            page_snapshot: capture.snapshot,
            needs_reauth: capture.needsReauth,
            error: capture.error,
            page_crashed: capture.pageCrashed,
            materials_found: capture.materials.length,
          });
          if (capture.materials.length > 0) {
            appGrowingAddLearnedSourceStrategy(learnedStrategies, learnedStrategyKeys, competitor, "browser_network", capture.materials.length);
          }
          for (const material of capture.materials) {
            materials.push({
              ...material,
              competitor,
              priority,
            });
          }
          if (capture.pageCrashed) {
            const replacementPage = await recreateAppGrowingCapturePage(context, browserCapturePage)
              .catch(() => null);
            if (!replacementPage) {
              browserFallbackStopped = true;
              break;
            }
            browserCapturePage = replacementPage;
          }
        }
        if (pageNumber >= Math.max(pageLimit, priorityPageLimit)
          && appGrowingNovelTargetMet(materials, rules, totalLimit, excludedDedupeKeys)) {
          break;
        }
      }
    }
  } else if (materials.length === 0 && params.browser_capture_fallback !== false) {
    captured.push({
      source: "browser_network",
      skipped: true,
      error: "browser_fallback_disabled_for_bulk_material_search",
      planned_pages: plannedPages,
      materials_found: 0,
    });
  }

  const strictSelection = selectAppGrowingMaterials(materials, rules, totalLimit, {
    fallbackToTopMaterials: false,
    excludedKeys: excludedDedupeKeys,
  });
  let selection = strictSelection;
  let supplementCount = 0;
  const explicitTopFallback = params.fallback_to_top_materials === true;
  if (explicitTopFallback || (adaptiveMode && strictSelection.selected.length === 0 && strictSelection.unique.length > 0)) {
    const fallbackSelection = selectAppGrowingMaterials(materials, rules, totalLimit, {
      fallbackToTopMaterials: true,
      excludedKeys: excludedDedupeKeys,
    });
    if (fallbackSelection.selected.length > strictSelection.selected.length) {
      selection = fallbackSelection;
      supplementCount = fallbackSelection.selected.length - strictSelection.selected.length;
      if (!explicitTopFallback) {
        appGrowingAddAutoAdjustment(autoAdjustments, "strict rules selected no material; added top materials as supplemental candidates");
      }
    }
  }
  const strictMaterialKeys = new Set(strictSelection.selected.map(appGrowingMaterialIdentityKey));
  const selectedMaterials = selection.selected.map((material) => ({
    ...material,
    dedupe_key: appGrowingMaterialDedupeKey(material),
    selection_match: strictMaterialKeys.has(appGrowingMaterialIdentityKey(material)) ? "strict" : "supplement",
  }));
  const selectionSummary = appGrowingSelectionMixSummary(selectedMaterials, rules, totalLimit);
  const competitorDiagnostics = appGrowingCompetitorDiagnostics(
    competitors,
    priorityCompetitors,
    pageLimit,
    priorityPageLimit,
    captured,
    selectedMaterials,
  );
  const needsReauth = captured.some((capture) => capture.needs_reauth === true);
  const blockingError = appGrowingMaterialSearchBlockingError(captured, materials);
  const budgetNote = timeBudgetExhausted ? "; stopped early because the material_search time budget was exhausted" : "";
  const mixNote = selectionSummary.ratio_target_met
    ? `actual mix new ${selectionSummary.actual.new_materials}, volume ${selectionSummary.actual.volume_materials}; target met`
    : `actual mix new ${selectionSummary.actual.new_materials}, volume ${selectionSummary.actual.volume_materials}; target ${selectionSummary.target.new_materials}/${selectionSummary.target.volume_materials} not met`;
  return {
    status: needsReauth ? "need_reauth" : blockingError ? "failed" : "completed",
    downloaded: 0,
    output_prefix: `local://credential-broker/${params.profile_id || "appgrowing"}/materials`,
    message: needsReauth
      ? "AppGrowing reported that the account was logged out; re-authentication is required"
      : blockingError
        ? `AppGrowing material_search failed before reading material data: ${blockingError}`
        : `selected ${selection.selected.length} new AppGrowing materials from ${competitors.length} competitors after filtering ${selection.excludedCount} previously seen assets; ${mixNote}; asset download storage is not configured${budgetNote}`,
    selection_summary: selectionSummary,
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
        max_pages_per_competitor: maxPageLimit,
        max_priority_pages_per_competitor: maxPriorityPageLimit,
        novel_only: novelOnly,
        planned_pages: plannedPages,
        graphql_timeout_ms: graphQLTimeoutMS,
        time_budget_ms: searchBudgetMS,
        elapsed_ms: Date.now() - startedAt,
        remaining_budget_ms: appGrowingRemainingBudgetMS(deadline),
        time_budget_exhausted: timeBudgetExhausted,
        browser_capture_fallback: browserFallbackEnabled,
        adaptive_mode: adaptiveMode,
        adaptive_browser_fallback: adaptiveBrowserFallbackEnabled,
        browser_fallback_competitors: browserFallbackCompetitors,
        adaptive_memory_scopes: Object.keys(adaptiveMemory.memories || {}).sort(),
        fallback_to_top_materials: params.fallback_to_top_materials === true,
      },
      captured,
      competitor_diagnostics: competitorDiagnostics,
      auto_adjustments: autoAdjustments,
      learned_strategies: learnedStrategies,
      selection_summary: selectionSummary,
      totals: {
        materials_seen: materials.length,
        unique_materials: selection.unique.length,
        new_candidates: selection.newCandidates.length,
        volume_candidates: selection.volumeCandidates.length,
        strict_count: strictSelection.selected.length,
        supplement_count: supplementCount,
        selected: selection.selected.length,
        historical_materials_filtered: selection.excludedCount,
        missing_duration_days: selection.unique.filter((item) => !Number.isFinite(Number(item.duration_days))).length,
        missing_impression_estimate: selection.unique.filter((item) => !Number.isFinite(Number(item.impression_estimate))).length,
      },
      material_samples: summarizeMaterialSamples(selection.unique),
      selected_materials: selectedMaterials,
    },
  };
}

function appGrowingMaterialSearchBlockingError(captured, materials) {
  if (materials.length > 0) {
    return "";
  }
  const attempts = captured.filter((capture) => capture && !capture.skipped);
  if (attempts.length === 0) {
    return "";
  }
  const successfulAttempts = attempts.filter((capture) => !capture.error);
  if (successfulAttempts.length > 0) {
    return "";
  }
  const blockingErrors = attempts
    .map((capture) => String(capture.error || "").trim())
    .filter((error) => error && !appGrowingMaterialSearchNonBlockingError(error));
  if (blockingErrors.length === 0) {
    return "";
  }
  const counts = new Map();
  for (const error of blockingErrors) {
    counts.set(error, (counts.get(error) || 0) + 1);
  }
  const [primary] = Array.from(counts.entries()).sort((a, b) => b[1] - a[1])[0] || [];
  return `${primary || blockingErrors[0]} (${blockingErrors.length} capture attempts failed)`;
}

function appGrowingMaterialSearchNonBlockingError(error) {
  return error === "app_brand_not_found" || error === "browser_fallback_disabled_for_bulk_material_search";
}

function appGrowingAdaptiveStrategyMemory(params = {}) {
  const raw = params?._adaptive_strategy_memory;
  if (!raw || typeof raw !== "object" || raw.enabled === false) {
    return { enabled: false, memories: {} };
  }
  const memories = raw.memories && typeof raw.memories === "object" ? raw.memories : {};
  return { enabled: true, memories };
}

function appGrowingStrategyMemoryForCompetitor(params = {}, competitor = "") {
  const memory = appGrowingAdaptiveStrategyMemory(params).memories || {};
  const key = normalizeCompetitorKey(competitor);
  const entry = memory[key];
  return entry && typeof entry === "object" ? entry : null;
}

function appGrowingBrandFromStrategyMemory(params, competitor) {
  const memory = appGrowingStrategyMemoryForCompetitor(params, competitor);
  const brandID = stringParam(memory?.brand_id);
  if (!brandID) {
    return null;
  }
  return {
    id: brandID,
    name: stringParam(memory.brand_name) || competitor,
    source: "strategy_memory",
  };
}

function appGrowingAliasesFromStrategyMemory(params, competitor) {
  const memory = appGrowingStrategyMemoryForCompetitor(params, competitor);
  if (!Array.isArray(memory?.aliases)) {
    return [];
  }
  return uniqueNonEmptyValues(memory.aliases);
}

function appGrowingShouldUseAdaptiveBrowserFallback(captured, adaptiveMemory) {
  const memories = adaptiveMemory?.memories || {};
  const prefersBrowser = Object.values(memories).some((memory) => memory?.preferred_source === "browser_network");
  if (prefersBrowser) {
    return true;
  }
  const attempts = captured.filter((capture) => capture?.source === "graphql_api" && !capture.skipped);
  return attempts.length > 0 && attempts.every((capture) => String(capture.error || "").trim() !== "");
}

function appGrowingCompetitorDiagnostics(
  competitors,
  priorityCompetitors,
  pageLimit,
  priorityPageLimit,
  captured,
  selectedMaterials = [],
) {
  return competitors.map((competitor) => {
    const competitorKey = normalizeCompetitorKey(competitor);
    const priority = priorityCompetitors.has(competitorKey);
    const requestedPages = priority ? priorityPageLimit : pageLimit;
    const attempts = captured.filter((capture) => (
      normalizeCompetitorKey(capture.competitor) === competitorKey
      && Number.isInteger(Number(capture.page))
    ));
    const pageResults = Array.from({ length: requestedPages }, (_, index) => {
      const page = index + 1;
      const pageAttempts = attempts.filter((capture) => Number(capture.page) === page);
      const materialsFound = pageAttempts.reduce(
        (total, capture) => total + Math.max(0, Number(capture.materials_found) || 0),
        0,
      );
      const needsReauth = pageAttempts.some((capture) => capture.needs_reauth === true);
      const budgetExhausted = pageAttempts.some((capture) => capture.error === "material_search_time_budget_exhausted");
      const attempted = pageAttempts.some((capture) => capture.skipped !== true);
      const succeeded = pageAttempts.some((capture) => capture.skipped !== true && !capture.error);
      let status = "not_attempted";
      if (needsReauth) {
        status = "need_reauth";
      } else if (materialsFound > 0) {
        status = "found";
      } else if (budgetExhausted) {
        status = "budget_exhausted";
      } else if (succeeded) {
        status = "no_match";
      } else if (attempted) {
        status = "failed";
      }
      return {
        page,
        status,
        materials_found: materialsFound,
        attempts: pageAttempts.map((capture) => ({
          source: capture.source,
          materials_found: Math.max(0, Number(capture.materials_found) || 0),
          error: capture.error || "",
          needs_reauth: capture.needs_reauth === true,
          skipped: capture.skipped === true,
        })),
      };
    });
    const materialsFound = pageResults.reduce((total, result) => total + result.materials_found, 0);
    const errors = Array.from(new Set(
      attempts.map((capture) => capture.error).filter(Boolean),
    ));
    const needsReauth = pageResults.some((result) => result.status === "need_reauth");
    const budgetExhausted = pageResults.some((result) => result.status === "budget_exhausted");
    const coverageComplete = pageResults.every((result) => result.status !== "not_attempted" && result.status !== "budget_exhausted");
    let status = "no_match";
    if (needsReauth) {
      status = "need_reauth";
    } else if (materialsFound > 0) {
      status = coverageComplete ? "found" : "found_incomplete";
    } else if (budgetExhausted) {
      status = "budget_exhausted";
    } else if (pageResults.some((result) => result.status === "failed")) {
      status = "failed";
    } else if (!coverageComplete) {
      status = "not_attempted";
    }
    return {
      competitor,
      priority,
      requested_pages: requestedPages,
      coverage_complete: coverageComplete,
      status,
      materials_found: materialsFound,
      selected: selectedMaterials.filter(
        (material) => normalizeCompetitorKey(material.competitor) === competitorKey,
      ).length,
      browser_fallback_used: attempts.some((capture) => capture.source === "browser_network"),
      errors,
      pages: pageResults,
    };
  });
}

function appGrowingBrowserFallbackCompetitors(
  competitors,
  captured,
  params = {},
  adaptiveMemory = {},
  browserFallbackEnabled = false,
  adaptiveMode = true,
) {
  if (params.browser_capture_fallback === false) {
    return [];
  }
  const memories = adaptiveMemory?.memories || {};
  return competitors.filter((competitor) => {
    const key = normalizeCompetitorKey(competitor);
    const attempts = captured.filter((capture) =>
      capture?.source === "graphql_api"
      && normalizeCompetitorKey(capture.competitor) === key
      && !capture.skipped,
    );
    if (attempts.some((capture) => Number(capture.materials_found) > 0)) {
      return false;
    }
    if (browserFallbackEnabled) {
      return true;
    }
    if (!adaptiveMode) {
      return false;
    }
    if (memories[key]?.preferred_source === "browser_network") {
      return true;
    }
    return attempts.length > 0;
  });
}

function appGrowingAddAutoAdjustment(adjustments, message) {
  const text = String(message || "").trim();
  if (!text || adjustments.includes(text)) {
    return;
  }
  adjustments.push(text);
}

function appGrowingAddLearnedBrandStrategy(out, seen, competitor, capture, preferredSource) {
  const brandID = stringParam(capture.appBrandID);
  if (!brandID || capture.materials.length === 0) {
    return;
  }
  const scopeKey = normalizeCompetitorKey(competitor);
  const key = `appgrowing_brand:${scopeKey}:${brandID}:${preferredSource}`;
  if (seen.has(key)) {
    return;
  }
  seen.add(key);
  out.push({
    strategy_type: "appgrowing_brand",
    scope_key: scopeKey,
    competitor,
    value: {
      brand_id: brandID,
      brand_name: stringParam(capture.appBrandName) || competitor,
      source: stringParam(capture.appBrandSource) || "unknown",
      preferred_source: preferredSource,
      materials_found: capture.materials.length,
    },
  });
}

function appGrowingAddLearnedSourceStrategy(out, seen, competitor, preferredSource, materialsFound) {
  const scopeKey = normalizeCompetitorKey(competitor);
  const key = `appgrowing_source:${scopeKey}:${preferredSource}`;
  if (!scopeKey || seen.has(key)) {
    return;
  }
  seen.add(key);
  out.push({
    strategy_type: "appgrowing_source",
    scope_key: scopeKey,
    competitor,
    value: {
      preferred_source: preferredSource,
      materials_found: materialsFound,
    },
  });
}

function appGrowingMaterialIdentityKey(material) {
  return appGrowingMaterialDedupeKey(material)
    || String(material?.material_id || material?.title || JSON.stringify(material || {}));
}

function appGrowingMaterialDedupeKey(material) {
  const raw = String(material?.resource_url || material?.preview_url || material?.poster_url || material?.landing_url || "").trim();
  if (!raw) {
    return "";
  }
  let identity = raw;
  try {
    const url = new URL(raw);
    url.search = "";
    url.hash = "";
    url.protocol = url.protocol.toLowerCase();
    url.hostname = url.hostname.toLowerCase();
    identity = url.toString();
  } catch {
    // Keep non-URL resource identifiers stable as provided.
  }
  return `sha256:${crypto.createHash("sha256").update(identity).digest("hex")}`;
}

function appGrowingPlannedMaterialPages(competitors, priorityCompetitors, pageLimit, priorityPageLimit) {
  return competitors.reduce((total, competitor) => {
    const priority = priorityCompetitors.has(normalizeCompetitorKey(competitor));
    return total + (priority ? priorityPageLimit : pageLimit);
  }, 0);
}

function shouldUseAppGrowingBrowserFallback(params = {}, plannedPages = 1) {
  if (params.browser_capture_fallback === true) {
    return true;
  }
  if (params.browser_capture_fallback === false) {
    return false;
  }
  return plannedPages <= 1;
}

function appGrowingRemainingBudgetMS(deadline) {
  if (!Number.isFinite(deadline)) {
    return Number.POSITIVE_INFINITY;
  }
  return Math.max(0, deadline - Date.now());
}

function appGrowingHasBudget(deadline, minBudgetMS = 1) {
  return appGrowingRemainingBudgetMS(deadline) >= minBudgetMS;
}

function appGrowingRequestTimeoutMS(deadline, desiredTimeoutMS) {
  const desired = positiveIntegerParam(desiredTimeoutMS, appGrowingGraphQLTimeoutMS, 1_000, 60_000);
  const remaining = appGrowingRemainingBudgetMS(deadline);
  if (!Number.isFinite(remaining)) {
    return desired;
  }
  if (remaining <= 0) {
    return 1;
  }
  return Math.max(1, Math.min(desired, Math.max(1, remaining - 500)));
}

async function captureAppGrowingMaterialPageViaGraphQL(context, connector, options) {
  const responses = [];
  const operations = new Set();
  const brand = await resolveAppGrowingBrand(context, connector, options, responses, operations);
  if (!brand.id) {
    return {
      url: connector.graphQLURL,
      operations: Array.from(operations).sort(),
      responses,
      appBrandID: "",
      appBrandName: brand.name || "",
      appBrandSource: brand.source || "",
      total: null,
      limit: null,
      needsReauth: brand.needsReauth === true,
      error: brand.error || "app_brand_not_found",
      materials: [],
    };
  }

  let lastError = "";
  for (const order of appGrowingGraphQLMaterialOrders(options.params)) {
    const variables = appGrowingAppMaterialListVariables(options.params, brand.id, options.pageNumber, new Date(), order);
    const result = await appGrowingGraphQLRequest(
      options.page || context,
      connector,
      "appMaterialList",
      APPGROWING_APP_MATERIAL_LIST_QUERY,
      variables,
      options.graphQLTimeoutMS,
      options.deadline,
    ).catch((error) => ({ error: error instanceof Error ? error.message : String(error), status: null, body: null }));
    operations.add("appMaterialList");
    responses.push(appGrowingGraphQLResponseSummary("appMaterialList", result));
    if (result.error) {
      lastError = result.error;
      continue;
    }
    const error = appGrowingGraphQLErrorMessage(result.body);
    if (error) {
      if (appGrowingGraphQLNeedsReauth(result.body)) {
        return {
          url: connector.graphQLURL,
          operations: Array.from(operations).sort(),
          responses,
          appBrandID: brand.id,
          appBrandName: brand.name || "",
          appBrandSource: brand.source || "",
          total: null,
          limit: null,
          needsReauth: true,
          error,
          materials: [],
        };
      }
      lastError = error;
      continue;
    }
    const materials = extractAppGrowingMaterials(result.body);
    return {
      url: connector.graphQLURL,
      operations: Array.from(operations).sort(),
      responses,
      appBrandID: brand.id,
      appBrandName: brand.name || "",
      appBrandSource: brand.source || "",
      total: parseNumericValue(valueAtPath(result.body, ["data", "materialList", "total"])),
      limit: parseNumericValue(valueAtPath(result.body, ["data", "materialList", "limit"])),
      needsReauth: false,
      error: "",
      materials,
    };
  }

  return {
    url: connector.graphQLURL,
    operations: Array.from(operations).sort(),
    responses,
    appBrandID: brand.id,
    appBrandName: brand.name || "",
    appBrandSource: brand.source || "",
    total: null,
    limit: null,
    needsReauth: false,
    error: lastError || "app_material_list_failed",
    materials: [],
  };
}

async function resolveAppGrowingBrand(context, connector, options, responses, operations) {
  const competitor = options.competitor;
  const key = normalizeCompetitorKey(competitor);
  if (options.brandCache?.has(key)) {
    return options.brandCache.get(key);
  }
  const explicitID = appGrowingBrandIDFromParams(connector, competitor, options.params);
  if (explicitID) {
    const brand = { id: explicitID, name: competitor, source: "params" };
    options.brandCache?.set(key, brand);
    return brand;
  }

  const memoryBrand = appGrowingBrandFromStrategyMemory(options.params, competitor);
  if (memoryBrand) {
    options.brandCache?.set(key, memoryBrand);
    return memoryBrand;
  }

  let lastError = "";
  const keywords = uniqueNonEmptyValues([
    competitor,
    ...appGrowingAliasesFromStrategyMemory(options.params, competitor),
  ]);
  for (const keyword of keywords) {
    const variables = appGrowingSearchAppVariables(keyword, options.params);
    const result = await appGrowingGraphQLRequest(
      options.page || context,
      connector,
      "searchApp",
      APPGROWING_SEARCH_APP_QUERY,
      variables,
      options.graphQLTimeoutMS,
      options.deadline,
    ).catch((error) => ({ error: error instanceof Error ? error.message : String(error), status: null, body: null }));
    operations.add("searchApp");
    responses.push(appGrowingGraphQLResponseSummary("searchApp", result));
    if (result.error) {
      lastError = result.error;
      continue;
    }
    const error = appGrowingGraphQLErrorMessage(result.body);
    if (error) {
      if (appGrowingGraphQLNeedsReauth(result.body)) {
        const expired = {
          id: "",
          name: competitor,
          source: "searchApp",
          needsReauth: true,
          error,
        };
        options.brandCache?.set(key, expired);
        return expired;
      }
      lastError = error;
      continue;
    }
    const rows = valueAtPath(result.body, ["data", "searchAppBrand", "data"]);
    const brand = appGrowingBrandFromSearchRows(rows, keyword);
    if (brand?.id) {
      const resolved = {
        ...brand,
        name: brand.name || competitor,
        source: keyword === competitor ? "searchApp" : "alias",
        keyword,
      };
      options.brandCache?.set(key, resolved);
      return resolved;
    }
  }
  const brand = { id: "", name: competitor, source: "searchApp", error: lastError };
  options.brandCache?.set(key, brand);
  return brand;
}

function appGrowingGraphQLRequest(requestTarget, connector, operationName, query, variables, timeoutMS = appGrowingGraphQLTimeoutMS, deadline = NaN) {
  if (!connector.graphQLURL) {
    throw new Error("AppGrowing GraphQL request context is unavailable");
  }
  const timeout = appGrowingRequestTimeoutMS(deadline, timeoutMS);
  const payload = { operationName, query, variables };
  if (typeof requestTarget?.evaluate === "function") {
    const headers = connectorGraphQLHeaders(connector, operationName);
    delete headers.origin;
    delete headers.referer;
    delete headers["user-agent"];
    return requestTarget.evaluate(async ({ url, data, requestHeaders, requestTimeoutMS }) => {
      const controller = new AbortController();
      const timer = window.setTimeout(() => controller.abort(), requestTimeoutMS);
      try {
        const response = await fetch(url, {
          method: "POST",
          credentials: "include",
          headers: requestHeaders,
          body: JSON.stringify(data),
          signal: controller.signal,
        });
        return {
          status: response.status,
          text: await response.text(),
        };
      } finally {
        window.clearTimeout(timer);
      }
    }, {
      url: connector.graphQLURL,
      data: payload,
      requestHeaders: headers,
      requestTimeoutMS: timeout,
    }).then(({ status, text }) => appGrowingGraphQLResult(status, text));
  }
  if (!requestTarget?.request) {
    throw new Error("AppGrowing GraphQL request context is unavailable");
  }
  return requestTarget.request.post(connector.graphQLURL, {
    data: payload,
    headers: connectorGraphQLHeaders(connector, operationName),
    timeout,
  }).then(async (response) => {
    const status = response.status();
    let text = "";
    try {
      text = await response.text();
    } catch {
      text = "";
    }
    return appGrowingGraphQLResult(status, text);
  });
}

function appGrowingGraphQLResult(status, text) {
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = null;
  }
  const result = { status, body };
  if (status < 200 || status >= 300) {
    result.error = appGrowingGraphQLHTTPErrorMessage(status, body, text);
  }
  return result;
}

function appGrowingGraphQLHTTPErrorMessage(status, body, text) {
  const bodyError = appGrowingGraphQLErrorMessage(body) || appGrowingCompactResponseText(text);
  return [`appgrowing_graphql_http_${status}`, bodyError].filter(Boolean).join(": ");
}

function appGrowingCompactResponseText(text) {
  if (typeof text !== "string") {
    return "";
  }
  const compact = text.replace(/\s+/g, " ").trim();
  return compact.length > 180 ? `${compact.slice(0, 177)}...` : compact;
}

function appGrowingGraphQLResponseSummary(operationName, result) {
  return {
    status: result.status ?? null,
    operations: [operationName],
    needs_reauth: appGrowingGraphQLNeedsReauth(result.body) || undefined,
    error: result.error || appGrowingGraphQLErrorMessage(result.body) || undefined,
  };
}

function appGrowingGraphQLNeedsReauth(body) {
  const errors = Array.isArray(body?.errors) ? body.errors : [];
  return errors.some((error) => {
    const extensions = error?.extensions;
    const code = String(extensions && typeof extensions === "object" ? extensions.c || extensions.code || "" : "").trim();
    const message = String(extensions && typeof extensions === "object" ? extensions.m || error?.message || "" : error?.message || "").trim();
    return code === "05:403005"
      || /login has expired|please log in again|account was logged out|session (?:has )?expired/i.test(message);
  });
}

function appGrowingBusinessProbeResult(authCheck, result) {
  const graphQLError = appGrowingGraphQLErrorMessage(result?.body);
  const hasSearchData = result?.body?.data
    && Object.prototype.hasOwnProperty.call(result.body.data, "searchAppBrand");
  const responseError = result?.error
    || graphQLError
    || (!hasSearchData ? "searchApp verification returned no searchAppBrand data" : "");
  const needsReauth = appGrowingGraphQLNeedsReauth(result?.body);
  const businessAuthenticated = authCheck?.authenticated === true && !responseError;
  const method = [authCheck?.method, "searchApp"].filter(Boolean).join("+");
  const businessCheck = {
    authenticated: businessAuthenticated,
    operation: "searchApp",
    http_status: result?.status ?? null,
    needs_reauth: needsReauth,
    upstream_error: responseError || undefined,
    observed_at: new Date().toISOString(),
  };
  return {
    authCheck: {
      ...authCheck,
      authenticated: businessAuthenticated,
      method,
      upstream_error: responseError || undefined,
      business_check: businessCheck,
    },
    needsReauth,
    verificationError: responseError && !needsReauth ? responseError : "",
  };
}

async function verifyAppGrowingBusinessAccess(requestTarget, connector, authCheck) {
  if (connector.id !== "appgrowing" || authCheck?.authenticated !== true) {
    return {
      authCheck,
      needsReauth: false,
      verificationError: "",
    };
  }
  let result;
  try {
    result = await appGrowingGraphQLRequest(
      requestTarget,
      connector,
      "searchApp",
      APPGROWING_SEARCH_APP_QUERY,
      appGrowingSearchAppVariables("Easycash", { purpose: 2 }),
    );
  } catch (err) {
    result = {
      status: null,
      body: null,
      error: err instanceof Error ? err.message : String(err),
    };
  }
  return appGrowingBusinessProbeResult(authCheck, result);
}

function appGrowingGraphQLErrorMessage(body) {
  const errors = Array.isArray(body?.errors) ? body.errors : [];
  if (errors.length === 0) {
    return "";
  }
  return errors
    .slice(0, 3)
    .map((error) => {
      const extensions = error?.extensions;
      const code = extensions && typeof extensions === "object" ? extensions.c || extensions.code : "";
      const message = extensions && typeof extensions === "object" ? extensions.m || error?.message : error?.message;
      return [code, message].filter(Boolean).join(": ") || String(error);
    })
    .join(" | ");
}

function appGrowingBrandFromSearchRows(rows, competitor) {
  if (!Array.isArray(rows)) {
    return null;
  }
  const normalized = normalizeCompetitorKey(competitor);
  const candidates = rows
    .map((row) => row?.appBrand)
    .filter((brand) => brand && typeof brand === "object" && typeof brand.id === "string" && brand.id.trim() !== "");
  const exact = candidates.find((brand) => normalizeCompetitorKey(brand.name) === normalized);
  const chosen = exact || candidates[0];
  if (!chosen) {
    return null;
  }
  return {
    id: chosen.id.trim(),
    name: typeof chosen.name === "string" ? chosen.name.trim() : competitor,
    source: "searchApp",
  };
}

function appGrowingBrandIDFromParams(connector, competitor, params) {
  for (const map of [params.app_brand_ids, params.appBrandIds, params.app_brands]) {
    const value = lookupCompetitorParam(map, competitor);
    if (value) {
      return value;
    }
  }
  const competitorURL = appGrowingCompetitorURL(connector, competitor, params);
  const match = competitorURL?.pathname.match(/\/appBrand\/([^/]+)\/leaflet$/);
  return match ? decodeURIComponent(match[1]) : "";
}

function appGrowingSearchAppVariables(competitor, params) {
  const variables = {
    purpose: positiveIntegerParam(params.purpose, 2, 1, 20),
    keyword: competitor,
    accurateSearch: positiveIntegerParam(params.accurateSearch ?? params.accurate_search, 1, 0, 1),
    page: 1,
  };
  const hadAdvert = optionalIntegerParam(params.hadAdvert ?? params.had_advert, 0, 1);
  variables.hadAdvert = Number.isInteger(hadAdvert) ? hadAdvert : 1;
  return variables;
}

function appGrowingAppMaterialListVariables(params, appBrandID, pageNumber, now = new Date(), order = "") {
  const window = appGrowingGraphQLDateWindow(params, now);
  const variables = {
    purpose: positiveIntegerParam(params.purpose, 2, 1, 20),
    startDate: window.startDate,
    endDate: window.endDate,
    field: String(params.field || "all"),
    order: order || appGrowingGraphQLMaterialOrders(params)[0],
    page: pageNumber,
    accurateSearch: positiveIntegerParam(params.accurateSearch ?? params.accurate_search, 1, 0, 1),
    appBrand: appBrandID,
  };
  const isNew = optionalIntegerParam(params.isNew ?? params.is_new, 0, 1);
  if (Number.isInteger(isNew)) {
    variables.isNew = isNew;
  }
  return variables;
}

function appGrowingGraphQLMaterialOrders(params = {}) {
  const explicit = String(params.graphql_order || params.material_order || params.app_material_order || "").trim();
  const mapped = mapAppGrowingMaterialOrder(explicit);
  return uniqueNonEmptyValues([
    mapped,
    "impression_inc_2y_desc",
    "max_dt_desc",
    "cnt_ad_id_desc",
  ]);
}

function mapAppGrowingMaterialOrder(value) {
  const normalized = String(value || "").trim();
  if (!normalized || normalized === "_score_desc") {
    return "";
  }
  const map = {
    cnt_dt_desc: "impression_inc_2y_desc",
    impression_desc: "impression_inc_2y_desc",
    material_cnt_desc: "impression_inc_2y_desc",
    max_dt_desc: "max_dt_desc",
    cnt_ad_id_desc: "cnt_ad_id_desc",
  };
  return map[normalized] || normalized;
}

function appGrowingGraphQLDateWindow(params = {}, now = new Date()) {
  const explicitStart = stringParam(params.startDate ?? params.start_date);
  const explicitEnd = stringParam(params.endDate ?? params.end_date);
  if (explicitStart || explicitEnd) {
    return {
      startDate: explicitStart || null,
      endDate: explicitEnd || null,
    };
  }
  const range = stringParam(params.daterange ?? params.date_range);
  const match = range.match(/^(-?\d+)\s*,\s*(-?\d+)$/);
  if (!match) {
    return {
      startDate: null,
      endDate: null,
    };
  }
  const startOffset = Number.parseInt(match[1], 10);
  const endOffset = Number.parseInt(match[2], 10);
  const anchor = utcDateOnly(now);
  return {
    startDate: formatLocalDate(addDaysUTC(anchor, startOffset)),
    endDate: formatLocalDate(addDaysUTC(anchor, endOffset)),
  };
}

function stringParam(value) {
  return typeof value === "string" ? value.trim() : "";
}

function optionalIntegerParam(value, min, max) {
  const number = Number(value);
  if (!Number.isInteger(number)) {
    return null;
  }
  return Math.max(min, Math.min(max, number));
}

function utcDateOnly(value) {
  const date = value instanceof Date && !Number.isNaN(value.getTime()) ? value : new Date();
  return new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate()));
}

function addDaysUTC(date, days) {
  const next = new Date(date.getTime());
  next.setUTCDate(next.getUTCDate() + days);
  return next;
}

function formatLocalDate(date) {
  return date.toISOString().slice(0, 10);
}

function uniqueNonEmptyValues(values) {
  const seen = new Set();
  const out = [];
  for (const value of values) {
    const text = String(value || "").trim();
    if (!text || seen.has(text)) {
      continue;
    }
    seen.add(text);
    out.push(text);
  }
  return out;
}

async function captureAppGrowingMaterialPage(page, connector, options) {
  const url = appGrowingMaterialURL(connector, options);
  const captured = [];
  const operations = new Set();
  const responses = [];
  let captureError = "";
  let pageCrashed = false;

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
    const summary = {
      status: response.status(),
      operations: operationNames,
    };
    responses.push(summary);
    try {
      const body = await response.json();
      summary.needs_reauth = appGrowingGraphQLNeedsReauth(body) || undefined;
      summary.error = appGrowingGraphQLErrorMessage(body) || undefined;
      const materials = extractAppGrowingMaterials(body);
      if (materials.length > 0) {
        captured.push(...materials);
      }
    } catch {
      // Ignore non-JSON GraphQL responses and keep the crawl best-effort.
    }
  };

  const routeHandler = appGrowingCrawlRouteHandler();
  if (routeHandler) {
    await page.route("**/*", routeHandler).catch(() => {});
  }
  page.on("response", onResponse);
  try {
    await page.goto(url, { waitUntil: "domcontentloaded", timeout: positiveIntegerParam(options.pageTimeoutMS, 60_000, 1_000, 60_000) });
    await Promise.race([
      page.waitForLoadState("networkidle", { timeout: options.captureTimeoutMS }).catch(() => null),
      sleep(options.captureTimeoutMS),
    ]);
    await stimulateAppGrowingMaterialList(page, options.captureTimeoutMS);
  } catch (error) {
    captureError = errorMessage(error);
    pageCrashed = isBrowserPageCrashError(error);
  } finally {
    page.off("response", onResponse);
    if (routeHandler) {
      await page.unroute("**/*", routeHandler).catch(() => {});
    }
  }

  const snapshot = await appGrowingPageSnapshot(page).catch((error) => ({
    url: safePageURL(page),
    title: "",
    text: "",
    error: errorMessage(error),
  }));
  return {
    url,
    operations: Array.from(operations).sort(),
    responses,
    snapshot,
    needsReauth: pageSnapshotHasAnonymousText(snapshot, connector)
      || responses.some((response) => response.needs_reauth === true),
    error: captureError,
    pageCrashed,
    materials: captured,
  };
}

function appGrowingCrawlRouteHandler() {
  return async (route) => {
    const request = route.request();
    if (shouldBlockAppGrowingCrawlResource(request)) {
      await route.abort().catch(() => {});
      return;
    }
    await route.continue().catch(() => {});
  };
}

function shouldBlockAppGrowingCrawlResource(request) {
  const resourceType = request.resourceType?.() || "";
  if (["font", "image", "media"].includes(resourceType)) {
    return true;
  }
  const url = request.url?.() || "";
  return /(?:google-analytics|googletagmanager|doubleclick|hotjar|sentry|clarity|facebook|tiktok|analytics|collect|beacon)/i.test(url);
}

function isBrowserPageCrashError(error) {
  return /page crashed|target page, context or browser has been closed|browser has been closed/i.test(errorMessage(error));
}

function errorMessage(error) {
  return error instanceof Error ? error.message : String(error);
}

function safePageURL(page) {
  try {
    return page.url();
  } catch {
    return "";
  }
}

async function recreateAppGrowingCapturePage(context, page) {
  await page?.close?.().catch(() => {});
  return context.newPage();
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
  const allUnique = uniqueMaterials(materials);
  const excludedKeys = options.excludedKeys instanceof Set ? options.excludedKeys : new Set();
  const unique = allUnique.filter((material) => !excludedKeys.has(appGrowingMaterialDedupeKey(material)));
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
    excludedCount: allUnique.length - unique.length,
  };
}

function appGrowingNovelTargetMet(materials, rules, totalLimit, excludedKeys) {
  return selectAppGrowingMaterials(materials, rules, totalLimit, {
    fallbackToTopMaterials: false,
    excludedKeys,
  }).selected.length >= totalLimit;
}

function appGrowingSelectionMixSummary(selectedMaterials, rules, totalLimit) {
  const numericLimit = Number(totalLimit);
  const safeLimit = Math.max(0, Number.isFinite(numericLimit) ? Math.floor(numericLimit) : 0);
  const newRatio = Number(rules?.new_materials?.ratio);
  const targetNew = Math.min(
    safeLimit,
    Math.max(0, Math.round(safeLimit * (Number.isFinite(newRatio) ? newRatio : 0))),
  );
  const targetVolume = Math.max(0, safeLimit - targetNew);
  const actualNew = selectedMaterials.filter((material) => material?.bucket === "new").length;
  const actualVolume = selectedMaterials.filter((material) => material?.bucket === "volume").length;
  const actualOther = Math.max(0, selectedMaterials.length - actualNew - actualVolume);
  const selectedCount = selectedMaterials.length;
  const ratio = (count) => selectedCount > 0 ? Number((count / selectedCount).toFixed(4)) : 0;

  return {
    target: {
      new_materials: targetNew,
      volume_materials: targetVolume,
    },
    actual: {
      new_materials: actualNew,
      volume_materials: actualVolume,
      other_materials: actualOther,
    },
    actual_ratio: {
      new_materials: ratio(actualNew),
      volume_materials: ratio(actualVolume),
      other_materials: ratio(actualOther),
    },
    shortfall: {
      new_materials: Math.max(0, targetNew - actualNew),
      volume_materials: Math.max(0, targetVolume - actualVolume),
    },
    selected: selectedCount,
    requested_limit: safeLimit,
    ratio_target_met: selectedCount === safeLimit
      && actualOther === 0
      && actualNew === targetNew
      && actualVolume === targetVolume,
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
    const key = appGrowingMaterialDedupeKey(material)
      || material.material_id
      || `${material.competitor}:${material.title}:${material.duration_days}:${material.impression_estimate}`;
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
    const key = appGrowingMaterialDedupeKey(candidate) || candidate.material_id;
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
  const releaseBrowserSlot = acquireBrowserSlot();
  let browser;
  let crawlerBrowserTimeout;
  try {
    browser = await chromium.launch(chromiumLaunchOptions(crawlerHeadless(connector)));
    crawlerBrowserTimeout = setTimeout(() => {
      void browser.close().catch(() => {});
    }, crawlerBrowserTimeoutMS);
    const context = await browser.newContext({
      storageState,
      ...(connector.id === "appgrowing" ? { locale: "en" } : {}),
    });
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
      const pageCheck = await verifyConnectorPageAuth(page, connector).catch(() => authCheck);
      authCheck = pageCheck?.authenticated ? pageCheck : await verifyConnectorAuth(context, connector);
    }
    const loginDetected = isLoginURL(currentURL, connector);
    const profileVerify = capability === "profile_verify";
    const identityVerificationError = loginDetected ? "" : connectorAuthVerificationError(authCheck);
    let businessProbe = {
      authCheck,
      needsReauth: false,
      verificationError: identityVerificationError,
    };
    if (profileVerify && authCheck.authenticated && !loginDetected && !identityVerificationError) {
      businessProbe = await verifyAppGrowingBusinessAccess(page, connector, authCheck);
      authCheck = businessProbe.authCheck;
    }
    const needsReauth = loginDetected
      || (!authCheck.authenticated && !businessProbe.verificationError)
      || businessProbe.needsReauth;
    const verificationFailed = Boolean(businessProbe.verificationError);
    const authProbe = {
      url: currentURL,
      title,
      http_status: response ? response.status() : null,
      login_detected: loginDetected,
      auth_check: authCheck,
    };
    if (!needsReauth && !verificationFailed && capability === "material_search") {
      if (connector.id !== "appgrowing") {
        throw userError(`connector ${connector.id} has no material_search executor`, 400);
      }
      const materialResult = await runAppGrowingMaterialSearch(page, context, connector, {
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
    if (!needsReauth && !verificationFailed && capability === "page_extract") {
      await runDeclarativePageSteps(page, body.params?.steps);
      extracted = await extractDeclarativePageData(page, body.params);
    }
    await context.close();
    return {
      status: needsReauth ? "need_reauth" : verificationFailed ? "failed" : "completed",
      downloaded: 0,
      output_prefix: `local://credential-broker/${body.profile_id || "unknown"}`,
      message: needsReauth
        ? "stored browser state cannot access AppGrowing materials; re-authentication is required"
        : verificationFailed
          ? `AppGrowing credential verification failed: ${businessProbe.verificationError}`
        : profileVerify
          ? `stored browser state and material access verified for ${connector.id}`
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
    clearTimeout(crawlerBrowserTimeout);
    await browser?.close().catch(() => {});
    releaseBrowserSlot();
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
      const playwrightReady = fs.existsSync(chromium.executablePath());
      const isReadinessProbe = url.pathname === "/readyz";
      writeJSON(res, isReadinessProbe && !playwrightReady ? 503 : 200, {
        ok: !isReadinessProbe || playwrightReady,
        service: "crawler-worker",
        playwright: playwrightReady,
        remote_ui: remoteBrowserUI,
        active_browsers: browserCapacity.active(),
        max_open_browsers: browserCapacity.limit(),
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
      const expiresAt = Date.now() + loginSessionTTLMS;
      activeSessions.set(body.session_token, {
        profileID: body.profile_id,
        connectorID: body.connector_id,
        loginURL: connector.loginURL || body.login_url,
        status: "pending",
        createdAt: Date.now(),
        expiresAt,
        browser: null,
        releaseBrowserSlot: null,
        context: null,
        page: null,
        viewport: sessionViewport,
        error: "",
      });
      scheduleSessionExpiry(body.session_token, expiresAt);
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
        if (req.method === "GET" && sessionPath.action === "stream") {
          res.writeHead(200, {
            "content-type": "image/svg+xml; charset=utf-8",
            "cache-control": "no-store",
          });
          res.end(unavailableSessionImage());
          return;
        }
        if (req.method === "GET" && sessionPath.action === "") {
          res.writeHead(410, {
            "content-type": "text/html; charset=utf-8",
            "cache-control": "no-store",
          });
          res.end(unavailableSessionHTML());
          return;
        }
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
      if (req.method === "POST" && sessionPath.action === "close") {
        writeJSON(res, 200, await closeRemoteBrowser(session));
        return;
      }
      if (req.method === "POST" && sessionPath.action === "complete") {
        try {
          writeJSON(res, 200, await completeSession(session, sessionPath.token));
        } catch (err) {
          session.error = err instanceof Error ? err.message : String(err);
          throw err;
        }
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
  appGrowingAppMaterialListVariables,
  appGrowingBrandFromStrategyMemory,
  appGrowingBrowserFallbackCompetitors,
  appGrowingCompetitorDiagnostics,
  appGrowingGraphQLDateWindow,
  appGrowingBusinessProbeResult,
  appGrowingGraphQLNeedsReauth,
  appGrowingGraphQLRequest,
  appGrowingMaterialDedupeKey,
  appGrowingMaterialURL,
  appGrowingSearchAppVariables,
  appGrowingSelectionMixSummary,
  appGrowingShouldUseAdaptiveBrowserFallback,
  captureAppGrowingMaterialPage,
  authCheckFromBody,
  connectorGraphQLHeaders,
  connectorAuthVerificationError,
  connectorForID,
  connectorTargetURL,
  extractAppGrowingMaterials,
  isBrowserPageCrashError,
  normalizeDeclarativeConnector,
  normalizeAppGrowingMaterial,
  selectAppGrowingMaterials,
  shouldBlockAppGrowingCrawlResource,
  shouldUseAppGrowingBrowserFallback,
};

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  server.listen(port, host, () => {
    console.log(`crawler-worker listening on ${publicURL}`);
    console.log(`completion callback target: ${apiURL}/api/credential-login-sessions/complete`);
    console.log(`remote browser UI: ${remoteBrowserUI ? "enabled" : "disabled"}`);
    console.log(`browser proxy: ${browserProxyServer ? "configured" : "disabled"}`);
    if (databaseURL) {
      console.log("credential DB store enabled");
    } else {
      console.log("DATABASE_URL not set; crawl cannot read stored credential state");
    }
  });
}
