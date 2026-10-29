import pytest
from playwright.sync_api import sync_playwright

BASE_URL = "http://localhost:8091"
API_URL  = "http://localhost:8091/api/v1"


@pytest.fixture(scope="session")
def browser():
    with sync_playwright() as p:
        b = p.chromium.launch(headless=True)
        yield b
        b.close()


@pytest.fixture
def page(browser):
    ctx = browser.new_context()
    pg  = ctx.new_page()
    yield pg
    ctx.close()
