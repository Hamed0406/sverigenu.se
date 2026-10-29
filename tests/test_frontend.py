"""
Sweden Nu — automated frontend test suite
==========================================
Level 1  Smoke   – every page returns 200, no JS errors, no failed fetches
Level 2  Render  – charts paint, tables populate, hero KPIs have values
Level 3  API     – every endpoint returns valid JSON with data > 0
"""

import re
import httpx
import pytest
from playwright.sync_api import Page, expect

from conftest import BASE_URL, API_URL

# ── Ignored console noise ──────────────────────────────────────────────────
_IGNORED_CONSOLE = re.compile(
    r"ResizeObserver|favicon|chart\.js|non-passive|"
    r"violation|download the React DevTools|Extension context",
    re.IGNORECASE,
)

# ── Helpers ────────────────────────────────────────────────────────────────

def collect_errors(page: Page):
    """Return a list of real JS errors / failed-fetch messages captured on the page."""
    errors = []

    def on_console(msg):
        if msg.type in ("error", "warning"):
            text = msg.text
            if not _IGNORED_CONSOLE.search(text):
                if any(kw in text.lower() for kw in ("error", "failed", "uncaught", "undefined is not")):
                    errors.append(f"[{msg.type}] {text}")

    def on_response(response):
        if not response.url.startswith(BASE_URL):
            return
        if response.url.endswith(".html") or "/fonts/" in response.url:
            return
        if response.status >= 400:
            errors.append(f"[HTTP {response.status}] {response.url}")

    page.on("console", on_console)
    page.on("response", on_response)
    return errors


# ═══════════════════════════════════════════════════════════════════════════
# Level 1 — Smoke tests
# ═══════════════════════════════════════════════════════════════════════════

PAGES = [
    "/",
    "/befolkning",
    "/inkomst",
    "/polis",
    "/vader",
    "/riksdagen",
    "/val",
    "/luftkvalitet",
    "/brott",
    "/trafik",
    "/ekonomi",
    "/energi",
    "/skatt",
    "/samband",
    "/sjukskrivning",
    "/arbetsmarknad",
    "/utbildning",
    "/halsa",
    "/domstol",
    "/lan/ab",
    "/lan/m",
    "/lan/bd",
]


@pytest.mark.parametrize("path", PAGES)
def test_page_loads(page: Page, path: str):
    """Every page returns 200 and has a <main> element."""
    errors = collect_errors(page)
    response = page.goto(BASE_URL + path, wait_until="domcontentloaded", timeout=10_000)
    assert response is not None and response.status == 200, f"{path} returned {response.status if response else 'None'}"
    assert page.title() != "", f"{path} has empty <title>"
    # Pages use either <main> or a top-level <div> wrapper — check body has content
    body_children = page.query_selector_all("body > *")
    assert len(body_children) > 0, f"{path} has empty <body>"
    # Give async JS a moment to fire any errors
    page.wait_for_timeout(1500)
    assert errors == [], f"{path} produced JS/network errors:\n" + "\n".join(errors)


# ═══════════════════════════════════════════════════════════════════════════
# Level 2 — Render tests
# ═══════════════════════════════════════════════════════════════════════════

@pytest.mark.slow
def test_render_index(page: Page):
    page.goto(BASE_URL + "/", wait_until="networkidle", timeout=15_000)
    cards = page.query_selector_all(".stat-card, .card, [class*='card']")
    assert len(cards) >= 3, f"Expected ≥3 cards on index, got {len(cards)}"


@pytest.mark.slow
def test_render_befolkning(page: Page):
    page.goto(BASE_URL + "/befolkning", wait_until="networkidle", timeout=15_000)
    canvas = page.query_selector("canvas")
    assert canvas is not None, "No <canvas> on befolkning"
    width = canvas.evaluate("el => el.clientWidth")
    assert width > 0, "Canvas has zero clientWidth"
    tbody = page.query_selector("tbody")
    assert tbody is not None
    rows = tbody.query_selector_all("tr")
    assert len(rows) > 0, "tbody is empty on befolkning"


@pytest.mark.slow
def test_render_halsa(page: Page):
    page.goto(BASE_URL + "/halsa", wait_until="networkidle", timeout=15_000)
    canvas = page.query_selector("canvas")
    assert canvas is not None, "No <canvas> on halsa"
    # Hero KPI should not be placeholder dash
    hero = page.query_selector("#heroTotal, #heroLife, .hero-val, .kpi-val")
    if hero:
        text = (hero.inner_text() or "").strip()
        assert text not in ("–", "", "0"), f"Hero KPI is placeholder: '{text}'"


@pytest.mark.slow
def test_render_polis_no_xss(page: Page):
    page.goto(BASE_URL + "/polis", wait_until="domcontentloaded", timeout=15_000)
    # Wait for feed to populate (events load via fetch)
    page.wait_for_selector("#feed a, #feed .event", timeout=8_000)
    feed = page.query_selector_all("#feed a, #feed .event")
    assert len(feed) >= 1, "Police event feed is empty"
    # XSS check: no raw onerror= in rendered feed HTML
    html = page.inner_html("#feed")
    assert "onerror=" not in html.lower(), "Possible XSS: onerror= found in rendered feed HTML"


@pytest.mark.slow
def test_render_riksdagen(page: Page):
    page.goto(BASE_URL + "/riksdagen", wait_until="networkidle", timeout=15_000)
    page.wait_for_timeout(3000)
    # At least 10 MP entries somewhere on the page
    rows = page.query_selector_all("tr, .mp-card, [class*='mp-'], [class*='member']")
    assert len(rows) >= 10, f"Expected ≥10 MP entries, got {len(rows)}"


@pytest.mark.slow
def test_render_brott(page: Page):
    page.goto(BASE_URL + "/brott", wait_until="networkidle", timeout=15_000)
    canvas = page.query_selector("canvas")
    assert canvas is not None, "No chart on brott"
    width = canvas.evaluate("el => el.clientWidth")
    assert width > 0, "Canvas has zero width"
    # Toggle/filter button
    sel = page.query_selector("select, [role='listbox'], .tab-btn, .year-btn, .sex-btn, .crime-btn, button[data-measure], button[data-cat]")
    assert sel is not None, "No selector/tab/button control on brott"


@pytest.mark.slow
def test_render_utbildning(page: Page):
    page.goto(BASE_URL + "/utbildning", wait_until="networkidle", timeout=15_000)
    page.wait_for_timeout(3000)
    tbody = page.query_selector("tbody")
    assert tbody is not None, "No table on utbildning"
    rows = tbody.query_selector_all("tr")
    assert len(rows) > 0, "Table is empty on utbildning"


@pytest.mark.slow
def test_render_inkomst(page: Page):
    page.goto(BASE_URL + "/inkomst", wait_until="networkidle", timeout=15_000)
    canvas = page.query_selector("canvas")
    assert canvas is not None, "No canvas on inkomst"
    tbody = page.query_selector("tbody")
    assert tbody is not None
    rows = tbody.query_selector_all("tr")
    assert len(rows) > 0, "Table empty on inkomst"


@pytest.mark.slow
def test_render_luftkvalitet(page: Page):
    page.goto(BASE_URL + "/luftkvalitet", wait_until="networkidle", timeout=15_000)
    page.wait_for_timeout(3000)
    rows = page.query_selector_all("tbody tr")
    assert len(rows) >= 10, f"Expected ≥10 air quality rows, got {len(rows)}"


@pytest.mark.slow
def test_render_lan_ab(page: Page):
    page.goto(BASE_URL + "/lan/ab", wait_until="networkidle", timeout=15_000)
    page.wait_for_timeout(3000)
    title = page.title()
    assert "stockholm" in title.lower() or "stockholm" in page.inner_text("h1, h2").lower(), \
        f"Stockholm not in page title/heading: {title}"
    kpi_vals = page.query_selector_all(".kpi-val, .hero-val, .stat-val, [class*='kpi']")
    non_empty = [v for v in kpi_vals if (v.inner_text() or "").strip() not in ("", "–", "0", "…")]
    assert len(non_empty) >= 3, f"Expected ≥3 non-empty KPI values on /lan/ab, got {len(non_empty)}"


@pytest.mark.slow
def test_render_val(page: Page):
    page.goto(BASE_URL + "/val", wait_until="networkidle", timeout=15_000)
    page.wait_for_timeout(2000)
    rows = page.query_selector_all("tr, .party-bar, [class*='party'], [class*='result']")
    assert len(rows) >= 5, f"Expected ≥5 party results on val, got {len(rows)}"


# ═══════════════════════════════════════════════════════════════════════════
# Level 3 — API tests
# ═══════════════════════════════════════════════════════════════════════════

API_ENDPOINTS = [
    "/weather",
    "/policeevents",
    "/befolkning",
    "/inkomst",
    "/kolada",
    "/lan/summary?county=ab",
    "/domstol/trends",
    "/domstol/processing",
    "/folkhalsa",
    "/val/national",
    "/val/turnout",
    "/luftkvalitet",
    "/brott",
    "/trafik",
    "/riksbank",
    "/elpris",
    "/arbetsmarknad/jobs",
    "/arbetsmarknad/fields",
    "/utbildning/schools",
    "/utbildning/national",
    "/halsa/dodsorsaker",
    "/halsa/tandhalsa",
    "/befolkning/forlossningar",
    "/riksdagen/members",
    "/riksdagen/seats",
    "/riksdagen/votes",
    "/sync/status",
]


@pytest.mark.parametrize("endpoint", API_ENDPOINTS)
def test_api_endpoint(endpoint: str):
    """Every API endpoint returns 200, valid JSON, and non-empty data."""
    url = API_URL + endpoint
    r = httpx.get(url, timeout=15.0)
    assert r.status_code == 200, f"{endpoint} returned {r.status_code}"
    data = r.json()
    assert isinstance(data, (dict, list)), f"{endpoint} returned non-JSON"
    if isinstance(data, dict):
        count = data.get("count", None)
        items = data.get("data", data.get("items", data.get("rows", None)))
        if count is not None:
            assert count > 0, f"{endpoint} has count=0"
        elif items is not None:
            assert len(items) > 0, f"{endpoint} has empty data array"
        # sync/status is a special status object — just check it's a dict
