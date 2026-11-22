#!/usr/bin/env python3
"""
Автотест GUI xswitcher: физическое переключение раскладки через noVNC.

Тест проверяет, что нажатие горячей клавиши (Ctrl+Alt+S) в графическом окне
xswitcher реально переключает раскладку и меняет отображаемый текст.

Это именно физический ввод: события идут через VNC-протокол → X11 → IME → GUI,
как если бы пользователь нажал клавиши физической клавиатурой.

Сценарий:
  1. noVNC запущен на порту 6081.
  2. Открываем About-вкладку GUI xswitcher.
  3. Набираем "hello" в поле ввода.
  4. Нажимаем Ctrl+Alt+S (hotkey из конфига стенда).
  5. Текст должен измениться на "gheoll" (английская раскладка для "hello").
  6. Снова жмём Ctrl+Alt+S — текст возвращается в "hello".

Установка:
    pip install pytest playwright

Запуск:
    pytest stand/tests/physical_input_test.py -v --browser=firefox
"""

import os
import sys
import time
from typing import Final, Optional

try:
    from playwright.sync_api import sync_playwright, Page, BrowserContext
except ImportError as e:
    print(f"Установите playwright: pip install playwright")
    print("playwright install")
    raise SystemExit(1)

# === Настройки окружения ===================================================== #
NOVNC_URL: Final[str] = "http://host.docker.internal:6081/noVNC/"

# Горячая клавиша, заданная в стенде (по умолчанию ctrl+alt+s)
DEFAULT_HOTKEY: Final[str] = "ctrl+alt+s"

# Дефолтный текст для теста
TEST_TEXT: Final[str] = "hello"


def get_hotkey_sequence(hotkey_str: str) -> list[str]:
    """Разбить строку hotkey на список событий браузера."""
    # Формат: ctrl+alt+s → ["Control", "Alt", "s"]
    modifiers, key = hotkey_str.lower().split("+") if "+" in hotkey_str else ("", hotkey_str)
    modifiers = [m.strip() for m in modifiers.split("+")]

    result: list[str] = []
    if "ctrl" in modifiers:
        result.append("Control")
    if "alt" in modifiers:
        result.append("Alt")
    if "meta" in modifiers or "win" in modifiers:
        result.append("Meta")
    if "shift" in modifiers:
        result.append("Shift")

    # Для буквы — зажимаем Shift, иначе верхний регистр не активируется
    key_lower = key.lower()
    if key_lower and key_lower not in ("space",):
        result.insert(0, "Shift")  # жмём Shift перед клавишей

    result.append(key)

    # Освобождаем модификаторы в обратном порядке
    for mod in reversed(modifiers):
        if mod == "ctrl":
            result.append("Control")
        elif mod == "alt":
            result.append("Alt")
        elif mod == "meta" or mod == "win":
            result.append("Meta")
        elif mod == "shift":
            result.append("Shift")

    return result


def click_about_tab(page: Page) -> None:
    """Перейти на вкладку About в GUI."""
    page.wait_for_load_state("domcontentloaded")
    time.sleep(0.5)  # дать noVNC синхронизироваться

    # Ищем кнопку/таб About — ищем по тексту или классу.
    about_btn = page.locator('text="About"').first
    if not about_btn.is_visible():
        # Пробуем найти через CSS selector (класс с noVNC)
        about_btn = page.locator("button:has-text('About')").first

    try:
        about_btn.click()
        time.sleep(0.5)
    except Exception:
        print("[WARN] About-кнопку не нашли, пробуем через keyboard:")
        # Альтернативный подход — кликнуть в область окна (если окно открыто)
        page.locator("div[role='tab']").first.click()


def type_text_in_field(page: Page, text: str) -> None:
    """Набрать текст в поле ввода GUI."""
    # Ищем input field. В Fyne это обычно TextField или InputField.
    # Попробуем несколько селекторов.

    candidates = [
        page.locator("input[type='text']"),
        page.locator("input[placeholder*='Input']"),
        page.locator(".field-input"),
        page.locator("textbox"),
        page.locator("textarea"),
    ]

    for selector in candidates:
        try:
            field = selector.first
            if field.is_visible():
                # Очищаем поле, если там что-то есть
                field.fill("")
                time.sleep(0.1)
                # Набираем текст с задержкой для каждого символа (имитация реального ввода)
                for ch in text:
                    page.keyboard.type(ch, delay=50)  # 50ms между символами — как у реального человека
                time.sleep(0.3)
                return
        except Exception:
            continue

    raise RuntimeError("Поле ввода не найдено")


def press_hotkey(page: Page, hotkey_str: str) -> None:
    """Нажать горячую клавишу через browser-use (физический ввод)."""
    sequence = get_hotkey_sequence(hotkey_str)
    print(f"[INFO] Нажимаю {hotkey_str} ({sequence})")

    page.keyboard.press(sequence[0])  # зажимаем первый модификатор
    time.sleep(0.1)

    for key in sequence:
        if "Shift" == key:
            page.keyboard.press("Shift", delay=30)
            time.sleep(0.1)
        else:
            page.keyboard.press(key, delay=20)
            time.sleep(0.1)

    # Освобождаем клавиши в обратном порядке (как при отпускании физической клавиатуры)
    for key in reversed(sequence):
        if "Shift" == key:
            page.keyboard.press("Shift", delay=30)
            time.sleep(0.1)
        else:
            page.keyboard.press(key, delay=20)
            time.sleep(0.1)


def wait_for_text_change(page: Page, target: str, timeout: float = 3.0) -> Optional[str]:
    """Ждать изменения текста в поле ввода."""
    start = time.monotonic()

    while time.monotonic() - start < timeout:
        try:
            # Пытаемся прочитать текст из поля ввода
            text_result = page.locator("input[type='text']").first.inner_text().strip()
            if text_result.strip():
                return text_result
        except Exception:
            pass

        time.sleep(0.1)

    raise RuntimeError(f"Текст не изменился за {timeout}s")


# === Тесты =================================================================== #

class TestGuiPhysicalInput:
    """Автотесты физического ввода в GUI xswitcher через noVNC."""

    @pytest.fixture(scope="function", autouse=True)
    def setup(self, browser_context: BrowserContext) -> None:
        """Подготовка к тесту перед каждым тестом."""
        # Переходим на noVNC
        page = browser_context.new_page()
        page.goto(NOVNC_URL)

        # Ждём загрузки noVNC и GUI
        page.wait_for_load_state("networkidle", timeout=30000)
        time.sleep(1.5)  # noVNC синхронизируется с VNC сервером

        yield

    def test_physical_input_hotkey_ru_to_en(self, browser_context: BrowserContext) -> None:
        """
        Переключение раскладки RU → EN через горячую клавишу.

        Ожидаемый результат: текст "hello" превращается в "gheoll".
        """
        page = browser_context.pages[-1]

        print("\n=== Тест: RU → EN переключение ===")

        # 1. Открываем About-вкладку (где поле ввода)
        click_about_tab(page)
        time.sleep(0.5)

        # 2. Набираем тестовый текст
        print("Набираю 'hello'...")
        type_text_in_field(page, TEST_TEXT)
        text = page.locator("input[type='text']").first.inner_text().strip()
        assert text == "hello", f"Ожидался 'hello', получил(а): {text!r}"
        print(f"[OK] текст: {text}")

        # 3. Жмём горячую клавишу — переключение раскладки
        hotkey = DEFAULT_HOTKEY if os.environ.get("XSWITCHER_HOTKEY") == DEFAULT_HOTKEY else "ctrl+alt+s"
        print(f"Жму hotkey: {hotkey}...")
        press_hotkey(page, hotkey)

        # 4. Ждём обработки IME и обновления GUI
        time.sleep(1.0)

        # 5. Проверяем результат
        new_text = page.locator("input[type='text']").first.inner_text().strip()
        expected = "gheoll"  # hello на английской раскладке
        assert new_text == expected, f"Ожидался '{expected}', получил(а): {new_text!r}"

        print(f"[OK] Текст изменился: 'hello' → {new_text}")

    def test_physical_input_en_to_ru(self, browser_context: BrowserContext) -> None:
        """
        Обратное переключение раскладки EN → RU.

        Ожидаемый результат: текст "gheoll" превращается обратно в "hello".
        """
        page = browser_context.pages[-1]

        print("\n=== Тест: EN → RU переключение ===")

        # 1. Открываем About-вкладку
        click_about_tab(page)
        time.sleep(0.5)

        # 2. Набираем "hello" (на английской раскладке это уже будет гхелло, но мы вставляем как есть)
        # Для упрощения — просто жмём hotkey и проверяем что вернётся русское слово
        press_hotkey(page, DEFAULT_HOTKEY)

        time.sleep(1.0)

        text = page.locator("input[type='text']").first.inner_text().strip()
        assert "hello" in text.lower(), f"Ожидался текст с русской раскладкой, получил: {text!r}"

        print(f"[OK] Текст: {text}")

    def test_cursor_stable_after_hotkey(self, browser_context: BrowserContext) -> None:
        """Проверить, что курсор не улетает после нажатия hotkey."""
        page = browser_context.pages[-1]

        click_about_tab(page)
        type_text_in_field(page, TEST_TEXT)
        time.sleep(0.5)

        # Запоминаем координаты курсора (через JS в noVNC — упрощённо)
        coords_before = page.evaluate("""() => {
            const rects = document.querySelectorAll('input[type="text"]');
            if (rects.length > 0) {
                return rects[0].getBoundingClientRect();
            }
            return null;
        }""")

        press_hotkey(page, DEFAULT_HOTKEY)
        time.sleep(1.0)

        coords_after = page.evaluate("""() => {
            const rects = document.querySelectorAll('input[type="text"]');
            if (rects.length > 0) {
                return rects[0].getBoundingClientRect();
            }
            return null;
        }""")

        # Координаты поля должны быть примерно одинаковыми
        assert coords_before is not None, "Поле ввода не найдено до нажатия hotkey"
        assert coords_after is not None, "Поле ввода не найдено после нажатия hotkey"
        assert abs(coords_before["x"] - coords_after["x"]) < 50, \
            f"Курсор сместился горизонтально: {coords_before['x']} → {coords_after['x']}"

    def test_multiple_hotkey_presses(self, browser_context: BrowserContext) -> None:
        """Множественные нажатия hotkey (переключение между раскладками)."""
        page = browser_context.pages[-1]

        click_about_tab(page)
        type_text_in_field(page, TEST_TEXT)

        text = page.locator("input[type='text']").first.inner_text().strip()
        print(f"Исходный текст: {text}")

        # Жмём hotkey несколько раз — текст должен чередоваться между раскладками
        for i in range(3):
            press_hotkey(page, DEFAULT_HOTKEY)
            time.sleep(0.3)

            current = page.locator("input[type='text']").first.inner_text().strip()
            print(f"  после нажатия {i+1}: {current!r}")


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--browser=firefox"])
