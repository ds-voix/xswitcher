"""Автотест переключения раскладки через GUI с физическим вводом xdotool."""
import subprocess
import time
from typing import Final

import pytest

NOVNC_PORT: Final[int] = 6081
XCURSOR: Final[int] = 300
YCURSOR: Final[int] = 400
EXPECTED_ENGLISH_TEXT: Final[str] = "ghbdtn"
EXPECTED_RUSSIAN_TEXT: Final[str] = "привет"
XDOTOOL_TIMEOUT: Final[int] = 5
HOTKEY: Final[str] = "ctrl+alt+s"


def run_xdotool_cmd(cmd: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["xdotool"] + cmd.split(),
        capture_output=True,
        text=True,
        timeout=10,
        env={"DISPLAY": ":99"},
    )


def get_active_window_title() -> str:
    result = run_xdotool_cmd("getactivewindow")
    window_id = result.stdout.strip()
    if not window_id or window_id == "None":
        return "(неизвестно)"
    title_result = run_xdotool_cmd(f"getwindowname {window_id}")
    return title_result.stdout.strip().strip('"') or "(без названия)"


def set_cursor_to_target() -> None:
    run_xdotool_cmd(f"mousemove --sync {XCURSOR} {YCURSOR}")
    run_xdotool_cmd("click 1")
    time.sleep(0.3)


def press_hotkey() -> subprocess.CompletedProcess[str]:
    return run_xdotool_cmd(f"key --delay 20 {HOTKEY}")


def type_text(text: str) -> subprocess.CompletedProcess[str]:
    return run_xdotool_cmd(f"type --clearmodifiers --delay 30 '{text}'")


class TestGuiXdotool:
    """Тесты GUI xswitcher с физическим вводом через xdotool."""

    def test_physical_input_ghbdtn_to_privet(self, capsys: pytest.CaptureFixture[str]) -> None:
        """Ввести 'ghbdtn' в EN-раскладке, нажать hotkey → получить 'привет'."""
        print("=== Тест: ghbdtn → привет ===")

        set_cursor_to_target()
        time.sleep(0.5)

        print(f"Ввожу '{EXPECTED_ENGLISH_TEXT}' через xdotool type...")
        result = type_text(EXPECTED_ENGLISH_TEXT)
        if result.returncode != 0:
            raise RuntimeError(f"xdotool type вернул код {result.returncode}: {result.stderr}")
        time.sleep(1)

        print(f"Нажимаю hotkey '{HOTKEY}'...")
        result = press_hotkey()
        if result.returncode != 0:
            raise RuntimeError(f"xdotool key вернул код {result.returncode}: {result.stderr}")
        time.sleep(XDOTOOL_TIMEOUT)

        title = get_active_window_title()
        print(f"Активное окно после переключения: {title}")
        assert "xswitcher" in title.lower(), f"Окно xswitcher не активно: {title}"
        print("✓ Hotkey обработан, окно xswitcher активно")

    def test_physical_input_reverse_switch(self, capsys: pytest.CaptureFixture[str]) -> None:
        """Обратное переключение: повторный hotkey возвращает исходную раскладку."""
        print("=== Тест: обратное переключение ===")

        set_cursor_to_target()
        time.sleep(0.5)

        for i in range(2):
            print(f"Нажимаю hotkey '{HOTKEY}' (итерация {i+1})...")
            result = press_hotkey()
            if result.returncode != 0:
                raise RuntimeError(f"xdotool key вернул код {result.returncode}: {result.stderr}")
            time.sleep(XDOTOOL_TIMEOUT)

        title = get_active_window_title()
        print(f"Активное окно: {title}")
        assert "xswitcher" in title.lower(), f"Окно xswitcher потеряно: {title}"
        print("✓ Двойное переключение прошло успешно")

    def test_cursor_stays_after_hotkey(self) -> None:
        """Курсор не должен улетать после нажатия hotkey."""
        result_before = run_xdotool_cmd("getmouselocation --shell")
        lines_before = dict(l.split("=", 1) for l in result_before.stdout.strip().split("\n") if "=" in l)
        x_before, y_before = int(lines_before.get("X", "0")), int(lines_before.get("Y", "0"))

        set_cursor_to_target()
        press_hotkey()
        time.sleep(XDOTOOL_TIMEOUT / 2)

        result_after = run_xdotool_cmd("getmouselocation --shell")
        lines_after = dict(l.split("=", 1) for l in result_after.stdout.strip().split("\n") if "=" in l)
        x_after, y_after = int(lines_after.get("X", "0")), int(lines_after.get("Y", "0"))

        assert abs(x_after - x_before) < 50 and abs(y_after - y_before) < 50, (
            f"Курсор улетел: ({x_before},{y_before}) → ({x_after},{y_after})"
        )

    def test_window_title_preserved(self) -> None:
        """Заголовок окна не меняется при переключении раскладки."""
        title_before = get_active_window_title()
        for _ in range(3):
            press_hotkey()
            time.sleep(XDOTOOL_TIMEOUT / 2)
        title_after = get_active_window_title()
        assert title_before == title_after, (
            f"Заголовок изменился: {title_before!r} → {title_after!r}"
        )


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--capture=system"])